package ingestshared

import (
	"context"
	"errors"
	"strings"

	"go.opentelemetry.io/collector/client"
)

// ExtractBearer reads the Authorization metadata from the OTLP receiver's
// client.Info (gRPC metadata or HTTP header). Returns the bare api-key with
// the "Bearer " prefix stripped. Error if header is missing or malformed.
//
// Both OTLP transports (gRPC + HTTP) MUST be configured with
// IncludeMetadata: true at receiver build time; otherwise client.Info has no
// metadata and this always errors. SLICE-1 T10/T13 lesson; do not regress.
func ExtractBearer(ctx context.Context) (string, error) {
	info := client.FromContext(ctx)
	vals := info.Metadata.Get("authorization")
	if len(vals) == 0 || vals[0] == "" {
		return "", errors.New("missing authorization metadata")
	}
	v := vals[0]
	const prefix = "Bearer "
	if !strings.HasPrefix(v, prefix) {
		return "", errors.New("authorization must use Bearer scheme")
	}
	return strings.TrimSpace(v[len(prefix):]), nil
}
