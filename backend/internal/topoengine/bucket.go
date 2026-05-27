package topoengine

import "time"

// ClosedBucketAt returns the start of the latest 1-minute bucket that is
// strictly before t. Aggregation processes [bucket, bucket+1min); any tick at t
// will safely aggregate a bucket no in-flight ingest is still writing to.
//
//	ClosedBucketAt(2026-05-27T12:03:42Z) == 2026-05-27T12:02:00Z
//	                                       (bucket [12:02:00, 12:03:00))
func ClosedBucketAt(t time.Time) time.Time {
	return t.UTC().Truncate(time.Minute).Add(-time.Minute)
}
