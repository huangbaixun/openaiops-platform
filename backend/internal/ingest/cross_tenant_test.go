//go:build integration

// TestSlice1_CrossTenantIsolation is the SLICE-1 AC #8 security gate:
// proves end-to-end that tenant A's spans never bleed into tenant B's
// query results, across BOTH OTLP transports (gRPC + HTTP).
//
// Six core sub-assertions per AC #8:
//  1. acme sees its own traces (list)
//  2. acme sees its own span count (detail)
//  3. beta sees zero traces (list)
//  4. beta gets 404 on acme's trace_id (detail)
//  5. no-bearer → 401
//  6. garbage-bearer → 401
//
// Plus two HTTP-transport regression sub-assertions guarding the T10
// receiver bug (IncludeMetadata=false silently dropped Authorization
// on the HTTP path):
//  7. OTLP/HTTP with valid bearer ingests spans (CH count increments)
//  8. OTLP/HTTP without bearer → 401 (not 200)
package ingest_test

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/ptrace"
	"go.opentelemetry.io/collector/pdata/ptrace/ptraceotlp"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
)

// fixtureSpans returns a deterministic ptrace.Traces carrying n spans
// under a single trace. salt varies the first 4 trace_id bytes so the
// caller can build distinct traces across batches (e.g., the 5-span
// gRPC batch and the 3-span HTTP regression batch don't collide).
// Returns the hex-encoded trace_id for cross-checks.
func fixtureSpans(n int, salt byte) (ptrace.Traces, string) {
	td := ptrace.NewTraces()
	rs := td.ResourceSpans().AppendEmpty()
	rs.Resource().Attributes().PutStr("service.name", "frontend")
	ss := rs.ScopeSpans().AppendEmpty()
	var trID [16]byte
	trID[0], trID[1], trID[2], trID[3] = 0xab, 0xcd, 0xef, salt
	now := time.Now()
	for i := 0; i < n; i++ {
		s := ss.Spans().AppendEmpty()
		s.SetName(fmt.Sprintf("op-%d", i))
		s.SetTraceID(pcommon.TraceID(trID))
		// span_id needs to be non-zero and unique within the trace; salt
		// the second byte too so spans across batches don't collide.
		s.SetSpanID(pcommon.SpanID([8]byte{byte(i + 1), salt}))
		s.SetStartTimestamp(pcommon.NewTimestampFromTime(now.Add(time.Duration(i) * time.Microsecond)))
		s.SetEndTimestamp(pcommon.NewTimestampFromTime(now.Add(time.Duration(i)*time.Microsecond + time.Millisecond)))
	}
	return td, hex.EncodeToString(trID[:])
}

// sendOTLPGRPC fires one ExportTraces call over gRPC with the bearer
// stuffed into the authorization metadata header — the path the OTel
// SDK uses in production.
func sendOTLPGRPC(t *testing.T, grpcAddr, bearer string, td ptrace.Traces) {
	t.Helper()
	cc, err := grpc.NewClient(grpcAddr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	require.NoError(t, err)
	defer cc.Close()
	client := ptraceotlp.NewGRPCClient(cc)
	ctx := metadata.AppendToOutgoingContext(context.Background(), "authorization", "Bearer "+bearer)
	_, err = client.Export(ctx, ptraceotlp.NewExportRequestFromTraces(td))
	require.NoError(t, err)
}

// sendOTLPHTTP POSTs ExportTraces as protobuf to the receiver's HTTP
// path. Returns the HTTP status code so the caller can assert 200 on
// the happy path and 401 on the no-bearer regression path.
func sendOTLPHTTP(t *testing.T, httpAddr, bearer string, td ptrace.Traces) (statusCode int) {
	t.Helper()
	body, err := ptraceotlp.NewExportRequestFromTraces(td).MarshalProto()
	require.NoError(t, err)
	req, err := http.NewRequest(http.MethodPost, "http://"+httpAddr+"/v1/traces", bytesReader(body))
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/x-protobuf")
	if bearer != "" {
		req.Header.Set("Authorization", "Bearer "+bearer)
	}
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	return resp.StatusCode
}

func TestSlice1_CrossTenantIsolation(t *testing.T) {
	env := bringUpIngestAndQuery(t)
	defer env.Shutdown()

	tidAcme, keyAcme := env.SeedTenant(t, "acme")
	_, keyBeta := env.SeedTenant(t, "beta")

	// Acme writes 5 spans over gRPC; salt=0x01 distinguishes this trace.
	td, traceID := fixtureSpans(5, 0x01)
	sendOTLPGRPC(t, env.IngestGRPCAddr, keyAcme, td)
	pollCH(t, env.CHConn, tidAcme, 5, 10*time.Second)

	// List queries use an explicit ts window pinned around the test run
	// (ts_from = now-1h, ts_to = now+1h). Default parseListParams uses
	// (now-1h, now) with strict ts < now — and clickhouse-go can round
	// the parameter so spans landed within ~1s of now get excluded.
	// Widening ts_to is the right E2E shape (matches what the frontend
	// sends anyway) and keeps THIS test focused on tenancy, not on
	// CH timestamp-parameter precision.
	now := time.Now().UTC()
	listPath := fmt.Sprintf("/v1/traces?ts_from=%s&ts_to=%s",
		now.Add(-time.Hour).Format(time.RFC3339Nano),
		now.Add(time.Hour).Format(time.RFC3339Nano))

	t.Run("acme sees own traces (list)", func(t *testing.T) {
		code, body, err := callList(env.QueryBaseURL, listPath, keyAcme)
		require.NoError(t, err)
		require.Equal(t, http.StatusOK, code)
		var resp struct {
			Items []struct {
				TraceID   string `json:"trace_id"`
				SpanCount uint64 `json:"span_count"`
			} `json:"items"`
		}
		require.NoError(t, json.Unmarshal(body, &resp))
		require.Len(t, resp.Items, 1)
		require.Equal(t, uint64(5), resp.Items[0].SpanCount)
		require.Equal(t, traceID, resp.Items[0].TraceID)
	})

	t.Run("acme sees own span count (detail)", func(t *testing.T) {
		req, _ := http.NewRequest(http.MethodGet, env.QueryBaseURL+"/v1/traces/"+traceID, nil)
		req.Header.Set("Authorization", "Bearer "+keyAcme)
		resp, err := http.DefaultClient.Do(req)
		require.NoError(t, err)
		defer resp.Body.Close()
		require.Equal(t, http.StatusOK, resp.StatusCode)
		var d struct {
			Spans []json.RawMessage `json:"spans"`
		}
		require.NoError(t, json.NewDecoder(resp.Body).Decode(&d))
		require.Len(t, d.Spans, 5)
	})

	t.Run("beta sees zero traces (list)", func(t *testing.T) {
		// Use the same widened window so we know any zero-result is
		// tenancy isolation, not a ts-filter accident.
		code, body, err := callList(env.QueryBaseURL, listPath, keyBeta)
		require.NoError(t, err)
		require.Equal(t, http.StatusOK, code)
		var resp struct {
			Items []json.RawMessage `json:"items"`
		}
		require.NoError(t, json.Unmarshal(body, &resp))
		require.Empty(t, resp.Items, "tenant beta must not see tenant acme's trace")
	})

	t.Run("beta gets 404 on acme trace_id (detail)", func(t *testing.T) {
		require.Equal(t, http.StatusNotFound,
			callDetail(env.QueryBaseURL, keyBeta, traceID))
	})

	t.Run("no bearer -> 401", func(t *testing.T) {
		code, _, _ := callList(env.QueryBaseURL, "", "")
		require.Equal(t, http.StatusUnauthorized, code)
	})

	t.Run("garbage bearer -> 401", func(t *testing.T) {
		code, _, _ := callList(env.QueryBaseURL, "", "deadbeef")
		require.Equal(t, http.StatusUnauthorized, code)
	})

	// ---- HTTP-transport regression (T10 IncludeMetadata bug) ----
	//
	// The receiver bug shipped IncludeMetadata=false by default. With
	// that, confighttp didn't copy the Authorization header into
	// client.Info.Metadata, extractBearer() saw nothing, and every
	// OTLP/HTTP call 401'd — silently passing the gRPC path. The two
	// sub-tests below lock both halves of the fix into a regression.

	t.Run("OTLP/HTTP delivers spans with valid bearer (T10 regression)", func(t *testing.T) {
		// Read acme's row count BEFORE the HTTP batch — using the same
		// poller path so MustTenantScope is satisfied.
		ctxA := authCtxFor(tidAcme.String())
		var before uint64
		require.NoError(t, env.CHConn.QueryRow(ctxA,
			`SELECT count() FROM traces_v1 WHERE tenant_id = ?`).Scan(&before))

		td2, _ := fixtureSpans(3, 0x02) // distinct trace_id from the gRPC batch
		code := sendOTLPHTTP(t, env.IngestHTTPAddr, keyAcme, td2)
		require.Equal(t, http.StatusOK, code,
			"OTLP/HTTP must return 200 when bearer is valid; T10 bug returned 401")
		pollCH(t, env.CHConn, tidAcme, before+3, 10*time.Second)
	})

	t.Run("OTLP/HTTP missing bearer -> 401 (must not silently 200)", func(t *testing.T) {
		td3, _ := fixtureSpans(1, 0x03)
		code := sendOTLPHTTP(t, env.IngestHTTPAddr, "", td3)
		require.Equal(t, http.StatusUnauthorized, code,
			"OTLP/HTTP without bearer must reject with 401, not silently accept")
	})
}
