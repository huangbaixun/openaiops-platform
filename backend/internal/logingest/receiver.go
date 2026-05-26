package logingest

import (
	"context"

	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/config/configgrpc"
	"go.opentelemetry.io/collector/config/confighttp"
	"go.opentelemetry.io/collector/config/confignet"
	"go.opentelemetry.io/collector/config/configoptional"
	"go.opentelemetry.io/collector/receiver"
	"go.opentelemetry.io/collector/receiver/otlpreceiver"
	noopMeter "go.opentelemetry.io/otel/metric/noop"
	"go.opentelemetry.io/otel/trace/noop"
	"go.uber.org/zap"
)

// ReceiverConfig captures the load-bearing knobs for the OTLP log receiver.
type ReceiverConfig struct {
	GRPCAddr string
	HTTPAddr string
}

// NewOTLPLogReceiver returns a configured otlpreceiver wired to `c`.
//
// IncludeMetadata: true is required on BOTH protocols from day 1. SLICE-1
// T10/T13 caught the same flag missing on the trace receiver — Authorization
// headers were silently lost on the affected transport. Sub-assertion 7 of
// backend/internal/logingest/cross_tenant_test.go (T11) locks this for logs.
func NewOTLPLogReceiver(cfg ReceiverConfig, c *LogConsumer) (receiver.Logs, error) {
	factory := otlpreceiver.NewFactory()
	rcvrCfg := factory.CreateDefaultConfig().(*otlpreceiver.Config)

	rcvrCfg.Protocols.GRPC = configoptional.Some(configgrpc.ServerConfig{
		NetAddr: confignet.AddrConfig{
			Endpoint:  cfg.GRPCAddr,
			Transport: confignet.TransportTypeTCP,
		},
		// Same reason as HTTPConfig.IncludeMetadata below: without this
		// flag the gRPC interceptor (configgrpc.enhanceWithClientInformation)
		// skips copying incoming metadata into client.Info.Metadata, so
		// extractBearer() in consume.go cannot see the Authorization
		// header and every OTLP/gRPC request 401s with "missing bearer".
		// Discovered by SLICE-1 T13 (AC #8 cross-tenant E2E). The hot-rod
		// demo masks this because it only emits OTLP/HTTP.
		IncludeMetadata: true,
	})

	rcvrCfg.Protocols.HTTP = configoptional.Some(otlpreceiver.HTTPConfig{
		ServerConfig: confighttp.ServerConfig{
			NetAddr: confignet.AddrConfig{
				Endpoint:  cfg.HTTPAddr,
				Transport: confignet.TransportTypeTCP,
			},
			// REQUIRED for HTTP auth path. Without IncludeMetadata=true the
			// confighttp wrapper does NOT copy incoming HTTP headers into
			// client.Info.Metadata, so extractBearer() in consume.go can't
			// see the Authorization header and every OTLP/HTTP request 401s
			// — even when the Bearer is present on the wire. gRPC works
			// without this flag because gRPC always exposes metadata.
			// Discovered while wiring the hot-rod demo (T10 review).
			IncludeMetadata: true,
		},
		LogsURLPath: "/v1/logs",
	})

	telSet := component.TelemetrySettings{
		Logger:         zap.NewNop(),
		TracerProvider: noop.NewTracerProvider(),
		MeterProvider:  noopMeter.NewMeterProvider(),
	}
	set := receiver.Settings{
		ID:                component.NewID(component.MustNewType("otlp")),
		TelemetrySettings: telSet,
		BuildInfo:         component.NewDefaultBuildInfo(),
	}
	return factory.CreateLogs(context.Background(), set, rcvrCfg, c)
}
