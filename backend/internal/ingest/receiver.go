package ingest

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

// ReceiverConfig captures the four load-bearing knobs for the OTLP receiver
// embedded inside the ingester binary: the gRPC bind addr (:4317 by default)
// and the HTTP bind addr (:4318 by default, /v1/traces URL path).
type ReceiverConfig struct {
	GRPCAddr string
	HTTPAddr string
}

// NewOTLPReceiver returns a configured otlpreceiver wired to `c`.
//
// API NOTE (collector v0.153.0): the receiver's Config exposes both
// protocols through a single `Protocols` struct, and each protocol slot is a
// `configoptional.Optional[T]` wrapper rather than a pointer. We use
// `configoptional.Some(...)` to enable both gRPC and HTTP. Both server
// configs put the listen address under `NetAddr` (confignet.AddrConfig),
// not a top-level Endpoint field.
func NewOTLPReceiver(cfg ReceiverConfig, c *Consumer) (receiver.Traces, error) {
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
		TracesURLPath: "/v1/traces",
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
	return factory.CreateTraces(context.Background(), set, rcvrCfg, c)
}
