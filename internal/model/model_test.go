package model

import (
	"errors"
	"testing"
)

func TestNewArrayValidation(t *testing.T) {
	if _, err := NewArray("", "n", 48000, 4); err == nil {
		t.Error("empty id should fail")
	}
	if _, err := NewArray("a", "", 48000, 4); err == nil {
		t.Error("empty name should fail")
	}
	if _, err := NewArray("a", "n", 0, 4); err == nil {
		t.Error("zero sample rate should fail")
	}
	if _, err := NewArray("a", "n", 48000, 0); err == nil {
		t.Error("zero element count should fail")
	}
	if _, err := NewArray("a", "n", 48000, 65); err == nil {
		t.Error("too many elements should fail")
	}
	arr, err := NewArray("a", "n", 48000, 4)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(arr.Elements) != 4 {
		t.Errorf("elements=%d want 4", len(arr.Elements))
	}
}

func TestElementByNo(t *testing.T) {
	arr, _ := NewArray("a", "n", 48000, 3)
	if _, err := arr.ElementByNo(2); err != nil {
		t.Errorf("element 2: %v", err)
	}
	if _, err := arr.ElementByNo(9); !errors.Is(err, ErrUnknownElement) {
		t.Errorf("element 9: got %v want ErrUnknownElement", err)
	}
}

func TestBatchStateTransitions(t *testing.T) {
	cases := []struct {
		from, to string
		ok       bool
	}{
		{BatchStatusUploading, BatchStatusProcessing, true},
		{BatchStatusProcessing, BatchStatusReviewing, true},
		{BatchStatusReviewing, BatchStatusPublished, true},
		{BatchStatusPublished, BatchStatusArchived, true},
		{BatchStatusUploading, BatchStatusPublished, false},
		{BatchStatusArchived, BatchStatusPublished, false},
		{BatchStatusReviewing, BatchStatusUploading, false},
	}
	for _, c := range cases {
		err := ValidateTransition(c.from, c.to)
		if c.ok && err != nil {
			t.Errorf("%s->%s: unexpected error %v", c.from, c.to, err)
		}
		if !c.ok && err == nil {
			t.Errorf("%s->%s: expected error", c.from, c.to)
		}
	}
}

func TestWindowStateTransitions(t *testing.T) {
	if err := ValidateWindowTransition(WindowStatusNew, WindowStatusCorrected); err != nil {
		t.Errorf("new->corrected: %v", err)
	}
	if err := ValidateWindowTransition(WindowStatusNew, WindowStatusIgnored); err != nil {
		t.Errorf("new->ignored: %v", err)
	}
	if err := ValidateWindowTransition(WindowStatusCorrected, WindowStatusNew); err == nil {
		t.Error("corrected->new should fail")
	}
}

func TestTrackStateTransitions(t *testing.T) {
	if err := ValidateTrackTransition(TrackStatusNeedsSegmentation, TrackStatusContinuous); err != nil {
		t.Errorf("seg->cont: %v", err)
	}
	if err := ValidateTrackTransition(TrackStatusContinuous, TrackStatusConfirmed); err != nil {
		t.Errorf("cont->confirmed: %v", err)
	}
	if err := ValidateTrackTransition(TrackStatusConfirmed, TrackStatusContinuous); err == nil {
		t.Error("confirmed->continuous should fail")
	}
}

func TestInterpretationTransitions(t *testing.T) {
	if err := ValidateInterpretationTransition(InterpretationStatusDraft, InterpretationStatusPublished); err != nil {
		t.Errorf("draft->published: %v", err)
	}
	if err := ValidateInterpretationTransition(InterpretationStatusPublished, InterpretationStatusSuperseded); err != nil {
		t.Errorf("published->superseded: %v", err)
	}
	if err := ValidateInterpretationTransition(InterpretationStatusSuperseded, InterpretationStatusDraft); err == nil {
		t.Error("superseded->draft should fail")
	}
}

func TestComputeContinuity(t *testing.T) {
	// 15 个断裂点 → 连续间隔 84/99 ≈ 0.848 < 0.9，需人工复核。
	tr := &Track{
		ID:          "t1",
		ElementNo:   1,
		PhasePoints: make([]float64, 100),
		BrokenAt:    []int{5, 10, 15, 20, 25, 30, 35, 40, 45, 50, 55, 60, 65, 70, 75},
	}
	c := ComputeContinuity(tr)
	if c == nil {
		t.Fatal("nil continuity")
	}
	if c.BrokenPoints != 15 {
		t.Errorf("broken=%d want 15", c.BrokenPoints)
	}
	if !c.NeedsReview {
		t.Error("15/99 broken should need review")
	}
	// 全连续轨迹。
	tr2 := &Track{ID: "t2", PhasePoints: make([]float64, 100)}
	if c2 := ComputeContinuity(tr2); c2 == nil || c2.NeedsReview {
		t.Errorf("fully continuous should not need review: %+v", c2)
	}
}

func TestComputeWindowStats(t *testing.T) {
	windows := []*Window{
		{Status: WindowStatusCorrected},
		{Status: WindowStatusCorrected},
		{Status: WindowStatusPhaseBroken},
		{Status: WindowStatusIgnored},
	}
	st := ComputeWindowStats(windows)
	if st.Total != 4 || st.Corrected != 2 || st.PhaseBroken != 1 || st.Ignored != 1 {
		t.Errorf("unexpected stats: %+v", st)
	}
}

func TestAdvanceWindowCursorExclusiveUpperBound(t *testing.T) {
	// 上传序号 3 的第一个窗口：游标应表示下一个待处理序号 4。
	if got := AdvanceWindowCursor(0, 3); got != 4 {
		t.Errorf("first window seqNo=3: cursor=%d want 4 (next to process)", got)
	}
	// 当前游标已在前方时不回退。
	if got := AdvanceWindowCursor(7, 3); got != 7 {
		t.Errorf("late seqNo=3 vs cursor=7: cursor=%d want 7 (monotonic)", got)
	}
	// 高序号窗口推进到 seqNo+1。
	if got := AdvanceWindowCursor(7, 9); got != 10 {
		t.Errorf("seqNo=9 vs cursor=7: cursor=%d want 10", got)
	}
	// 重复上传同一序号不回退。
	if got := AdvanceWindowCursor(4, 3); got != 4 {
		t.Errorf("duplicate seqNo=3 vs cursor=4: cursor=%d want 4", got)
	}
}
