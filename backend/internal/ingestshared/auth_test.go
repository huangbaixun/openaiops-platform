package ingestshared

import (
	"context"
	"testing"

	"go.opentelemetry.io/collector/client"
)

func TestExtractBearer_FromMetadata(t *testing.T) {
	md := client.NewMetadata(map[string][]string{"authorization": {"Bearer test-key-acme"}})
	ctx := client.NewContext(context.Background(), client.Info{Metadata: md})
	got, err := ExtractBearer(ctx)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if got != "test-key-acme" {
		t.Fatalf("want test-key-acme, got %q", got)
	}
}

func TestExtractBearer_Missing(t *testing.T) {
	ctx := client.NewContext(context.Background(), client.Info{})
	if _, err := ExtractBearer(ctx); err == nil {
		t.Fatal("expected error")
	}
}

func TestExtractBearer_NotBearerPrefix(t *testing.T) {
	md := client.NewMetadata(map[string][]string{"authorization": {"Basic abc"}})
	ctx := client.NewContext(context.Background(), client.Info{Metadata: md})
	if _, err := ExtractBearer(ctx); err == nil {
		t.Fatal("expected error")
	}
}
