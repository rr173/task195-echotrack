package fitting

import (
	"math"
	"testing"
)

func TestDetectJumps(t *testing.T) {
	phase := []float64{0, 0.1, 2.5, 2.6, 2.7}
	brokenAt, jumps := DetectJumps(phase, 1.0)
	if jumps != 1 {
		t.Errorf("jumps=%d want 1", jumps)
	}
	if len(brokenAt) != 1 || brokenAt[0] != 2 {
		t.Errorf("brokenAt=%v want [2]", brokenAt)
	}
}

func TestSplitSegments(t *testing.T) {
	// 总长 20，断裂点 [5, 15]，minLen=3：应得 [0..4][5..14][15..19] 三段。
	segs := SplitSegments(20, []int{5, 15}, 3)
	if len(segs) != 3 {
		t.Fatalf("len=%d want 3: %+v", len(segs), segs)
	}
	if segs[0].Start != 0 || segs[0].End != 4 {
		t.Errorf("seg0=%+v", segs[0])
	}
	if segs[1].Start != 5 || segs[1].End != 14 {
		t.Errorf("seg1=%+v", segs[1])
	}
	if segs[2].Start != 15 || segs[2].End != 19 {
		t.Errorf("seg2=%+v", segs[2])
	}
}

func TestSplitSegmentsMergeShort(t *testing.T) {
	// 断裂点 [4, 5]：段 [4,4] 长度 1 < minLen 3，并入前段 → [0..4] 与 [5..9]。
	segs := SplitSegments(10, []int{4, 5}, 3)
	if len(segs) != 2 {
		t.Fatalf("len=%d want 2: %+v", len(segs), segs)
	}
	if segs[0].End != 4 {
		t.Errorf("short segment not merged: %+v", segs[0])
	}
}

func TestFitTrackContinuous(t *testing.T) {
	phase := make([]float64, 100)
	for k := range phase {
		phase[k] = float64(k) * 0.01 // 缓慢连续
	}
	res, err := FitTrack(phase, DefaultOptions())
	if err != nil {
		t.Fatalf("err=%v", err)
	}
	if !res.Continuous {
		t.Errorf("expected continuous, jumps=%d", res.JumpCount)
	}
	if len(res.Segments) != 1 {
		t.Errorf("expected 1 segment, got %d", len(res.Segments))
	}
}

func TestFitTrackBroken(t *testing.T) {
	phase := make([]float64, 100)
	for k := range phase {
		phase[k] = float64(k) * 0.01
	}
	phase[50] += 3.0 // 大跳变（前后两处邻差均超阈值）
	res, err := FitTrack(phase, DefaultOptions())
	if err != nil {
		t.Fatalf("err=%v", err)
	}
	if res.Continuous {
		t.Errorf("expected needs segmentation")
	}
	if res.JumpCount < 1 {
		t.Errorf("jumps=%d want >=1", res.JumpCount)
	}
}

func TestConcatTracksMedian(t *testing.T) {
	a := []float64{1, 2, 100, 4}
	b := []float64{1, 2, 3, 4}
	c := []float64{1, 2, 5, 4}
	got, err := ConcatTracks([][]float64{a, b, c})
	if err != nil {
		t.Fatalf("err=%v", err)
	}
	if len(got) != 4 {
		t.Fatalf("len=%d want 4", len(got))
	}
	if math.Abs(got[2]-5) > 1e-9 {
		t.Errorf("median at idx2=%v want 5", got[2])
	}
}

func TestConcatTracksLengthMismatch(t *testing.T) {
	_, err := ConcatTracks([][]float64{{1, 2}, {1, 2, 3}})
	if err == nil {
		t.Fatal("expected length mismatch error")
	}
}
