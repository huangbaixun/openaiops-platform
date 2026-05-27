package topoengine

import (
	"testing"
	"time"
)

func mustParse(t *testing.T, s string) time.Time {
	t.Helper()
	v, err := time.Parse(time.RFC3339Nano, s)
	if err != nil {
		t.Fatalf("parse %q: %v", s, err)
	}
	return v
}

// closedBucketAt(T) returns the start of the bucket strictly before T's
// containing minute. e.g. T=12:03:42 → 12:02:00.
func TestClosedBucketAt(t *testing.T) {
	cases := []struct{ tick, want string }{
		{"2026-05-27T12:03:42.123Z", "2026-05-27T12:02:00Z"},
		{"2026-05-27T12:00:00.000Z", "2026-05-27T11:59:00Z"},
		{"2026-05-27T12:01:00.000Z", "2026-05-27T12:00:00Z"},
		{"2026-05-27T12:00:00.001Z", "2026-05-27T11:59:00Z"},
	}
	for _, c := range cases {
		t.Run(c.tick, func(t *testing.T) {
			got := ClosedBucketAt(mustParse(t, c.tick))
			want := mustParse(t, c.want)
			if !got.Equal(want) {
				t.Fatalf("ClosedBucketAt(%s) = %s, want %s", c.tick, got, want)
			}
		})
	}
}
