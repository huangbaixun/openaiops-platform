package ingest

import "github.com/google/uuid"

// Metering is the per-batch usage event recorder. T6 lands the real impl.
// Stub interface here so Consumer compiles and the T6 swap is drop-in.
type Metering struct{}

func (m *Metering) Enqueue(tid uuid.UUID, count int) { _ = tid; _ = count }
