package ingest

import "go.opentelemetry.io/collector/component"

// nopHost implements component.Host with an empty extensions map.
// Replaces componenttest.NewNopHost which is test-scoped per upstream docs.
type nopHost struct{}

func (nopHost) GetExtensions() map[component.ID]component.Component { return nil }

// NewHost returns a minimal component.Host suitable for embedding the
// otlpreceiver in a non-collector binary.
func NewHost() component.Host { return nopHost{} }
