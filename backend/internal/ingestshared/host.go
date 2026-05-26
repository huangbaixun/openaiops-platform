package ingestshared

import "go.opentelemetry.io/collector/component"

// nopHost implements component.Host with an empty extensions map.
// Replaces componenttest.NewNopHost which is test-scoped per upstream docs.
type nopHost struct{}

func (nopHost) GetExtensions() map[component.ID]component.Component { return nil }

// NewHost returns a minimal component.Host for embedding otlpreceiver in
// a non-collector binary (test-package componenttest.NewNopHost is not for runtime use).
func NewHost() component.Host { return nopHost{} }
