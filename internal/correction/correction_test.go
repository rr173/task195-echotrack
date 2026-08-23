package correction

import (
	"math"
	"testing"
)

func TestPhase(t *testing.T) {
	cases := []struct {
		i, q float64
		want float64
	}{
		{1, 0, 0},
		{0, 1, math.Pi / 2},
		{-1, 0, math.Pi},
		{0, -1, -math.Pi / 2},
	}
	for _, c := range cases {
		got := Phase(c.i, c.q)
		if math.Abs(got-c.want) > 1e-9 {
			t.Errorf("Phase(%v,%v)=%v want %v", c.i, c.q, got, c.want)
		}
	}
}

func TestUnwrapPhase(t *testing.T) {
	// 构造跨 ±pi 边界的相位：-2.9, 2.9, -2.9, 2.9 应解卷绕为连续序列。
	raw := []float64{-2.9, 2.9, -2.9, 2.9}
	got := UnwrapPhase(raw)
	if len(got) != len(raw) {
		t.Fatalf("len=%d want %d", len(got), len(raw))
	}
	for k := 1; k < len(got); k++ {
		// 相邻差应落在 (-pi, pi] 内（无 ±2π 折叠）。
		d := got[k] - got[k-1]
		if d <= -math.Pi || d > math.Pi {
			t.Errorf("unwrapped delta %v out of range at %d", d, k)
		}
	}
}

func TestCorrectWindowRotation(t *testing.T) {
	// 单位复数 (1,0) 延迟 1/4 采样周期 → 旋转 -pi/2。
	i := []float64{1}
	q := []float64{0}
	rotI, rotQ, rotRad, err := CorrectWindow(i, q, 250, 1000)
	if err != nil {
		t.Fatalf("err=%v", err)
	}
	// delayUs=250us, fs=1000Hz → 0.25 周期 → 相位 2*pi*0.25 = pi/2。
	wantRad := math.Pi / 2
	if math.Abs(rotRad-wantRad) > 1e-9 {
		t.Errorf("rotationRad=%v want %v", rotRad, wantRad)
	}
	// (1,0) 旋转 +pi/2 → (0,1)。
	if math.Abs(rotI[0]) > 1e-9 || math.Abs(rotQ[0]-1) > 1e-9 {
		t.Errorf("rotated=(%v,%v) want (0,1)", rotI[0], rotQ[0])
	}
}

func TestCorrectWindowLengthMismatch(t *testing.T) {
	_, _, _, err := CorrectWindow([]float64{1}, []float64{}, 0, 1000)
	if err == nil {
		t.Fatal("expected length mismatch error")
	}
}

func TestAnalyzeContinuity(t *testing.T) {
	// 相位序列有一个 3 弧度跳变。
	phase := []float64{0, 0.1, 0.2, 0.1, 3.3, 3.4, 3.5}
	brokenAt, jumps := AnalyzeContinuity(phase, 1.0)
	if jumps != 1 {
		t.Errorf("jumps=%d want 1", jumps)
	}
	if len(brokenAt) != 1 || brokenAt[0] != 4 {
		t.Errorf("brokenAt=%v want [4]", brokenAt)
	}
}

func TestEstimateDelay(t *testing.T) {
	// 相位差 pi/2、采样率 1000Hz：0.5 rad / (2*pi*1000) 秒 = 250us。
	got := EstimateDelay(math.Pi/2, 1000)
	if math.Abs(got-250) > 1e-6 {
		t.Errorf("delay=%v want 250", got)
	}
}
