package ingest

// Metering is the per-batch usage event recorder. T6 lands the real impl.
// For T5 we declare the type and an Enqueue stub so Consumer compiles.
type Metering struct{}

func (m *Metering) Enqueue(tid any, count int) { _ = tid; _ = count }
