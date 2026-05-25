package ingest

import (
	"context"

	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/component/componenttest"
	"go.opentelemetry.io/collector/config/configgrpc"
	"go.opentelemetry.io/collector/config/confighttp"
	"go.opentelemetry.io/collector/config/confignet"
	"go.opentelemetry.io/collector/config/configoptional"
	"go.opentelemetry.io/collector/receiver"
	"go.opentelemetry.io/collector/receiver/otlpreceiver"
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
	})

	rcvrCfg.Protocols.HTTP = configoptional.Some(otlpreceiver.HTTPConfig{
		ServerConfig: confighttp.ServerConfig{
			NetAddr: confignet.AddrConfig{
				Endpoint:  cfg.HTTPAddr,
				Transport: confignet.TransportTypeTCP,
			},
		},
		TracesURLPath: "/v1/traces",
	})

	set := receiver.Settings{
		ID:                component.NewID(component.MustNewType("otlp")),
		TelemetrySettings: componenttest.NewNopTelemetrySettings(),
		BuildInfo:         component.NewDefaultBuildInfo(),
	}
	return factory.CreateTraces(context.Background(), set, rcvrCfg, c)
}
