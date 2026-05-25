package ingest

import (
	"context"
	"errors"
	"strings"

	"go.opentelemetry.io/collector/client"
)

var ErrMissingBearer = errors.New("missing bearer")

// extractBearer pulls the Bearer token out of the OTLP request's authorization
// header. Works for both gRPC metadata and HTTP headers because the OTel
// otlpreceiver normalizes both into client.Info.Metadata.
func extractBearer(ctx context.Context) (string, error) {
	info := client.FromContext(ctx)
	vals := info.Metadata.Get("authorization")
	if len(vals) == 0 {
		return "", ErrMissingBearer
	}
	h := vals[0]
	if !strings.HasPrefix(h, "Bearer ") {
		return "", ErrMissingBearer
	}
	return strings.TrimPrefix(h, "Bearer "), nil
}
