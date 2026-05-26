//go:build integration

// TestSlice2_CrossTenantLogs is the SLICE-2 AC #8 security gate:
// proves end-to-end that tenant A's log records never bleed into tenant B's
// query results, across BOTH OTLP transports (gRPC + HTTP).
//
// Eight sub-assertions per AC #8:
//  1. A ingests 3 logs via OTLP/gRPC; CH count for tenant A == 3
//  2. B queries /v1/logs — empty items, has_more=false
//  3. B queries /v1/logs?trace_id=<A's> — empty items
//  4. A queries /v1/logs?trace_id=<A's> — 3 rows
//  5. A queries /v1/logs?trace_id=<A's>&span_id=<A's> — expected subset (>=1)
//  6. OTLP/gRPC with no Authorization metadata → gRPC codes.Unauthenticated
//  7. OTLP/HTTP :4328 with no Authorization header → HTTP 401
//     (SLICE-1 T13 regression — IncludeMetadata=true bug catch for logs)
//  8. /v1/logs with garbage Bearer → HTTP 401
//
// Sub-7 is LOAD-BEARING: it locks in that IncludeMetadata=true is set on the
// HTTP transport in logingest.NewOTLPLogReceiver. SLICE-1 T13 caught that flag
// missing on the trace receiver; we assert it from day 1 for logs.
package logingest_test

import (
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/plog"
	"go.opentelemetry.io/collector/pdata/plog/plogotlp"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
	grpcstatus "google.golang.org/grpc/status"

	"github.com/huangbaixun/openaiops-platform/backend/internal/auth"
	"github.com/huangbaixun/openaiops-platform/backend/internal/chquery"
	"github.com/huangbaixun/openaiops-platform/backend/internal/ingestshared"
	"github.com/huangbaixun/openaiops-platform/backend/internal/logingest"
	"github.com/huangbaixun/openaiops-platform/backend/internal/query"
)

// logEnv bundles the runtime for a single test: OTLP addresses, query base URL,
// direct CH + PG handles. Shutdown tears down the receiver, query server, metering,
// and CH connection.
type logEnv struct {
	IngestGRPCAddr string
	IngestHTTPAddr string
	QueryBaseURL   string
	CHConn         *chquery.Conn
	Shutdown       func()
}

// pickLogPort returns "127.0.0.1:N" with a free ephemeral port.
// The listener is closed immediately so the port is available for the receiver.
func pickLogPort(t *testing.T) string {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	addr := l.Addr().String()
	require.NoError(t, l.Close())
	return addr
}

// bringUpLogIngestAndQuery wires the in-process system under test:
// real PG resolver + real CH conn + real LogConsumer + real OTLPLogReceiver
// (gRPC + HTTP) on random ports; plus a real query.NewRouter behind an
// httptest.Server. Caller must defer env.Shutdown().
func bringUpLogIngestAndQuery(t *testing.T) *logEnv {
	t.Helper()

	chConn, err := chquery.Connect(context.Background(), meteringCHFix.DSN)
	require.NoError(t, err)

	resolver := auth.NewPGResolver(meteringPGPool)
	reg := prometheus.NewRegistry()
	base := ingestshared.NewBaseMetrics(reg, "log")
	metering := ingestshared.NewMetering(meteringPGPool, base, "log")

	consumer := logingest.NewLogConsumer(resolver, chConn, metering, base)

	grpcAddr := pickLogPort(t)
	httpAddr := pickLogPort(t)

	rcvr, err := logingest.NewOTLPLogReceiver(logingest.ReceiverConfig{
		GRPCAddr: grpcAddr,
		HTTPAddr: httpAddr,
	}, consumer)
	require.NoError(t, err)
	require.NoError(t, rcvr.Start(context.Background(), ingestshared.NewHost()))

	qrouter := query.NewRouter(resolver, chConn)
	qsrv := httptest.NewServer(qrouter)

	return &logEnv{
		IngestGRPCAddr: grpcAddr,
		IngestHTTPAddr: httpAddr,
		QueryBaseURL:   qsrv.URL,
		CHConn:         chConn,
		Shutdown: func() {
			qsrv.Close()
			shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			_ = rcvr.Shutdown(shutdownCtx)
			metering.Drain(shutdownCtx)
			metering.Close()
			_ = chConn.Close()
		},
	}
}

// fixtureLogs builds a plog.Logs with n records sharing the given traceID
// and spanID (both as 16/8-byte raw hex). salt varies a byte in the IDs so
// batches from different calls don't collide.
// Returns the logs, a hex trace_id string, and a hex span_id string.
func fixtureLogs_ct(n int, salt byte) (plog.Logs, string, string) {
	ld := plog.NewLogs()
	rl := ld.ResourceLogs().AppendEmpty()
	rl.Resource().Attributes().PutStr("service.name", "svc-cross-tenant")
	sl := rl.ScopeLogs().AppendEmpty()

	var trID [16]byte
	trID[0], trID[1], trID[2], trID[3] = 0xab, 0xcd, 0xef, salt

	var spID [8]byte
	spID[0], spID[1] = 0x11, salt

	traceIDHex := hex.EncodeToString(trID[:])
	spanIDHex := hex.EncodeToString(spID[:])

	now := time.Now()
	for i := 0; i < n; i++ {
		rec := sl.LogRecords().AppendEmpty()
		rec.SetTimestamp(pcommon.NewTimestampFromTime(now.Add(time.Duration(i) * time.Millisecond)))
		rec.SetObservedTimestamp(pcommon.NewTimestampFromTime(now.Add(time.Duration(i)*time.Millisecond + time.Microsecond)))
		rec.SetSeverityText("INFO")
		rec.SetSeverityNumber(plog.SeverityNumberInfo)
		rec.Body().SetStr(fmt.Sprintf("cross-tenant log line %d salt=%d", i, salt))
		rec.SetTraceID(pcommon.TraceID(trID))
		rec.SetSpanID(pcommon.SpanID(spID))
	}
	return ld, traceIDHex, spanIDHex
}

// sendLogGRPC fires one ExportLogs call over gRPC with bearer in Authorization metadata.
// Returns the error from Export (nil on success, non-nil on server rejection).
func sendLogGRPC(t *testing.T, grpcAddr, bearer string, ld plog.Logs) error {
	t.Helper()
	cc, err := grpc.NewClient(grpcAddr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	require.NoError(t, err)
	defer cc.Close()
	client := plogotlp.NewGRPCClient(cc)
	ctx := context.Background()
	if bearer != "" {
		ctx = metadata.AppendToOutgoingContext(ctx, "authorization", "Bearer "+bearer)
	}
	_, exportErr := client.Export(ctx, plogotlp.NewExportRequestFromLogs(ld))
	return exportErr
}

// sendLogHTTP POSTs ExportLogs as protobuf to the receiver's HTTP path.
// Returns the HTTP status code (200 on success, 401 on auth failure).
func sendLogHTTP(t *testing.T, httpAddr, bearer string, ld plog.Logs) (statusCode int) {
	t.Helper()
	body, err := plogotlp.NewExportRequestFromLogs(ld).MarshalProto()
	require.NoError(t, err)
	req, err := http.NewRequest(http.MethodPost, "http://"+httpAddr+"/v1/logs", io.NopCloser(bytes.NewReader(body)))
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/x-protobuf")
	if bearer != "" {
		req.Header.Set("Authorization", "Bearer "+bearer)
	}
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, resp.Body)
	return resp.StatusCode
}

// pollLogsCH polls CH until tenant `tid` has at least `want` rows in logs_v1,
// or until `timeout`. Uses authCtxFor so chquery's MustTenantScope is satisfied.
func pollLogsCH(t *testing.T, ch *chquery.Conn, tid uuid.UUID, want uint64, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	ctx := auth.WithTenant(context.Background(), tid, "poll")
	var last uint64
	for time.Now().Before(deadline) {
		var n uint64
		if err := ch.QueryRow(ctx,
			`SELECT count() FROM logs_v1 WHERE tenant_id = ?`).Scan(&n); err == nil {
			last = n
			if n >= want {
				return
			}
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatalf("sub-assertion BLOCKED: CH did not show >=%d rows for tenant %s within %s (last=%d)",
		want, tid, timeout, last)
}

// callLogsList GETs /v1/logs with optional bearer + query string suffix.
// baseURL is the httptest.Server URL — no /api prefix.
func callLogsList(baseURL, qs, bearer string) (statusCode int, body []byte, err error) {
	path := "/v1/logs"
	if qs != "" {
		path += "?" + qs
	}
	req, err := http.NewRequest(http.MethodGet, baseURL+path, nil)
	if err != nil {
		return 0, nil, err
	}
	if bearer != "" {
		req.Header.Set("Authorization", "Bearer "+bearer)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return 0, nil, err
	}
	defer resp.Body.Close()
	b, err := io.ReadAll(resp.Body)
	return resp.StatusCode, b, err
}

func TestSlice2_CrossTenantLogs(t *testing.T) {
	env := bringUpLogIngestAndQuery(t)
	defer env.Shutdown()

	tidA, keyA := seedTenantWithKey(t, meteringPGPool, "ct-acme")
	_, keyB := seedTenantWithKey(t, meteringPGPool, "ct-beta")

	// Tenant A ingests 3 logs via OTLP/gRPC (salt=0xA1 — distinct from T6 logs).
	logs, traceIDHex, spanIDHex := fixtureLogs_ct(3, 0xA1)

	// Wide time window (now±1h) so logs land inside the query filter regardless
	// of CH timestamp rounding. Same idiom as SLICE-1 cross_tenant_test.go.
	now := time.Now().UTC()
	qs := fmt.Sprintf("ts_from=%s&ts_to=%s",
		now.Add(-time.Hour).Format(time.RFC3339Nano),
		now.Add(time.Hour).Format(time.RFC3339Nano))

	// ---------- Sub-assertion 1: A ingests via gRPC (gates all later sub-tests) ----------
	// Run OUTSIDE t.Run so that ingest failures cascade — otherwise sub-2/sub-3 would
	// pass vacuously (empty list trivially satisfies require.Empty). Matches the
	// SLICE-1 trace cross_tenant_test.go pattern at internal/ingest/cross_tenant_test.go.
	require.NoError(t,
		sendLogGRPC(t, env.IngestGRPCAddr, keyA, logs),
		"sub1: OTLP/gRPC ingest must succeed for tenant A with valid bearer")
	pollLogsCH(t, env.CHConn, tidA, 3, 10*time.Second)
	t.Run("sub1_A_ingest_grpc_3_logs_recorded", func(t *testing.T) {
		// Marker sub-test — the actual assertion ran above so the failure surfaces
		// before sibling sub-tests can produce false-greens.
	})

	// ---------- Sub-assertion 2: B sees no logs ----------
	t.Run("sub2_B_sees_empty_list", func(t *testing.T) {
		code, body, err := callLogsList(env.QueryBaseURL, qs, keyB)
		require.NoError(t, err)
		require.Equal(t, http.StatusOK, code, "sub2: /v1/logs must return 200 for tenant B")
		var resp struct {
			Items   []json.RawMessage `json:"items"`
			HasMore bool              `json:"has_more"`
		}
		require.NoError(t, json.Unmarshal(body, &resp), "sub2: response must be valid JSON")
		require.Empty(t, resp.Items, "sub2: tenant B must not see tenant A's logs")
		require.False(t, resp.HasMore, "sub2: has_more must be false when items is empty")
	})

	// ---------- Sub-assertion 3: B queries with A's trace_id → empty ----------
	t.Run("sub3_B_trace_id_filter_empty", func(t *testing.T) {
		code, body, err := callLogsList(env.QueryBaseURL, qs+"&trace_id="+traceIDHex, keyB)
		require.NoError(t, err)
		require.Equal(t, http.StatusOK, code, "sub3: trace_id filter must return 200 for tenant B")
		var resp struct {
			Items []json.RawMessage `json:"items"`
		}
		require.NoError(t, json.Unmarshal(body, &resp), "sub3: response must be valid JSON")
		require.Empty(t, resp.Items,
			"sub3: tenant B must not see tenant A's logs when filtering on A's trace_id")
	})

	// ---------- Sub-assertion 4: A queries with its own trace_id → 3 rows ----------
	t.Run("sub4_A_trace_id_filter_3_rows", func(t *testing.T) {
		code, body, err := callLogsList(env.QueryBaseURL, qs+"&trace_id="+traceIDHex, keyA)
		require.NoError(t, err)
		require.Equal(t, http.StatusOK, code, "sub4: trace_id filter must return 200 for tenant A")
		var resp struct {
			Items []json.RawMessage `json:"items"`
		}
		require.NoError(t, json.Unmarshal(body, &resp), "sub4: response must be valid JSON")
		require.Len(t, resp.Items, 3,
			"sub4: tenant A must see exactly 3 logs when filtering on its own trace_id")
	})

	// ---------- Sub-assertion 5: A queries trace_id+span_id → subset >=1 ----------
	t.Run("sub5_A_trace_and_span_id_filter", func(t *testing.T) {
		code, body, err := callLogsList(env.QueryBaseURL,
			qs+"&trace_id="+traceIDHex+"&span_id="+spanIDHex, keyA)
		require.NoError(t, err)
		require.Equal(t, http.StatusOK, code, "sub5: trace+span filter must return 200 for tenant A")
		var resp struct {
			Items []json.RawMessage `json:"items"`
		}
		require.NoError(t, json.Unmarshal(body, &resp), "sub5: response must be valid JSON")
		require.NotEmpty(t, resp.Items,
			"sub5: tenant A must see at least 1 log when filtering on trace_id+span_id")
	})

	// ---------- Sub-assertion 6: gRPC with no Bearer → codes.Unauthenticated ----------
	t.Run("sub6_grpc_no_bearer_unauthenticated", func(t *testing.T) {
		noAuthLogs, _, _ := fixtureLogs_ct(1, 0xA6)
		err := sendLogGRPC(t, env.IngestGRPCAddr, "", noAuthLogs)
		require.Error(t, err, "sub6: OTLP/gRPC without bearer must return an error")
		st, ok := grpcstatus.FromError(err)
		require.True(t, ok, "sub6: error must be a gRPC status error")
		require.Equal(t, codes.Unauthenticated, st.Code(),
			"sub6: gRPC status code must be Unauthenticated when Authorization metadata is absent; "+
				"counter log_ingester_auth_missing_total{signal=\"log\"} must have incremented")
	})

	// ---------- Sub-assertion 7: OTLP/HTTP without Bearer → 401 ----------
	// LOAD-BEARING: this is the SLICE-1 T13 regression check for logs.
	// logingest.NewOTLPLogReceiver MUST set IncludeMetadata=true on the HTTP
	// transport; without it the Authorization header is silently lost and the
	// receiver returns 200 instead of 401.
	t.Run("sub7_http_no_bearer_401_regression", func(t *testing.T) {
		noAuthLogs, _, _ := fixtureLogs_ct(1, 0xA7)
		code := sendLogHTTP(t, env.IngestHTTPAddr, "", noAuthLogs)
		require.Equal(t, http.StatusUnauthorized, code,
			"sub7: OTLP/HTTP without bearer must return 401, not 200; "+
				"IncludeMetadata=true is required on HTTP transport in logingest.NewOTLPLogReceiver "+
				"(SLICE-1 T13 regression — if this is 200, the flag is missing)")
	})

	// ---------- Sub-assertion 8: /v1/logs with garbage Bearer → 401 ----------
	t.Run("sub8_garbage_bearer_401", func(t *testing.T) {
		code, _, err := callLogsList(env.QueryBaseURL, qs, "deadbeefdeadbeef")
		require.NoError(t, err)
		require.Equal(t, http.StatusUnauthorized, code,
			"sub8: /v1/logs with an invalid Bearer must return 401")
	})
}
