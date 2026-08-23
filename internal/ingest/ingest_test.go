package ingest

import (
	"testing"
)

func TestValidate(t *testing.T) {
	ok := WindowInput{BatchID: "b", ElementNo: 1, SeqNo: 0, I: []float64{1}, Q: []float64{1}, SampleRate: 48000}
	if err := ok.Validate(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	bad := []WindowInput{
		{BatchID: "", ElementNo: 1, SeqNo: 0, I: []float64{1}, Q: []float64{1}, SampleRate: 1},
		{BatchID: "b", ElementNo: 0, SeqNo: 0, I: []float64{1}, Q: []float64{1}, SampleRate: 1},
		{BatchID: "b", ElementNo: 1, SeqNo: -1, I: []float64{1}, Q: []float64{1}, SampleRate: 1},
		{BatchID: "b", ElementNo: 1, SeqNo: 0, I: []float64{1, 2}, Q: []float64{1}, SampleRate: 1},
		{BatchID: "b", ElementNo: 1, SeqNo: 0, I: nil, Q: nil, SampleRate: 1},
		{BatchID: "b", ElementNo: 1, SeqNo: 0, I: []float64{1}, Q: []float64{1}, SampleRate: 0},
	}
	for i, in := range bad {
		if err := in.Validate(); err == nil {
			t.Errorf("case %d: expected error for %+v", i, in)
		}
	}
}

func TestValidateSampleRate(t *testing.T) {
	if err := ValidateSampleRate(48000, 48000); err != nil {
		t.Errorf("same rate: %v", err)
	}
	if err := ValidateSampleRate(47999.99, 48000); err != nil {
		t.Errorf("tiny diff: %v", err)
	}
	if err := ValidateSampleRate(44000, 48000); err == nil {
		t.Error("mismatch should error")
	}
}

func TestChecksumDeterministic(t *testing.T) {
	i := []float64{0.1, 0.2, -0.3}
	q := []float64{1, 2, 3}
	c1 := Checksum(i, q)
	c2 := Checksum(i, q)
	if c1 != c2 {
		t.Errorf("checksum not deterministic: %s vs %s", c1, c2)
	}
	// 内容变化应改变校验和。
	if c1 == Checksum([]float64{0.1, 0.2, -0.4}, q) {
		t.Error("checksum should differ when content changes")
	}
	if c1 == Checksum(i, []float64{1, 2, 4}) {
		t.Error("checksum should differ when q changes")
	}
}

func TestNewReceipt(t *testing.T) {
	r := NewReceipt("w1", "abc", true, false)
	if !r.Inserted || r.Duplicated || r.WindowID != "w1" || r.Checksum != "abc" {
		t.Errorf("unexpected receipt: %+v", r)
	}
}
