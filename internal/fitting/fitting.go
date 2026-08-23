// Package fitting 实现回波轨迹拟合：跨窗口拼接相位、自动分段与连续性裁决。
// 输入为已校正（解卷绕）的相位序列，输出连续轨迹与断裂分段建议。
package fitting

import (
	"fmt"
	"math"
)

// FittingOptions 拟合参数。
type FittingOptions struct {
	// JumpThresholdRad 相邻点相位差超过该值判定为跳变/断裂。
	JumpThresholdRad float64
	// MinSegmentLen 自动分段的最小点数，小于该值的断裂段被合并。
	MinSegmentLen int
	// MinContinuityRatio 连续点数占总点数比例低于该值时判定为需人工分段。
	MinContinuityRatio float64
}

// DefaultOptions 返回默认拟合参数。
func DefaultOptions() FittingOptions {
	return FittingOptions{
		JumpThresholdRad:   math.Pi / 2,
		MinSegmentLen:      8,
		MinContinuityRatio: 0.9,
	}
}

// Segment 拟合输出的连续区段。
type Segment struct {
	Start int `json:"start"`
	End   int `json:"end"` // 包含端
}

// Result 拟合结果。
type Result struct {
	PhasePoints []float64 `json:"phasePoints"`
	BrokenAt    []int     `json:"brokenAt"`
	Segments    []Segment `json:"segments"`
	Continuous  bool      `json:"continuous"`
	JumpCount   int       `json:"jumpCount"`
}

// FitTrack 对单个相位序列拟合：检测断裂、自动分段、裁决连续性。
func FitTrack(phase []float64, opts FittingOptions) (*Result, error) {
	if len(phase) == 0 {
		return nil, fmt.Errorf("empty phase sequence")
	}
	brokenAt, jumps := DetectJumps(phase, opts.JumpThresholdRad)
	segments := SplitSegments(len(phase), brokenAt, opts.MinSegmentLen)
	continuous := jumps == 0 && len(phase) >= 2 && phase[0] <= phase[len(phase)-1]
	// 若断裂被合并后无残余断裂，视为可接受连续。
	if jumps > 0 {
		contiguous := 0
		for _, s := range segments {
			contiguous += s.End - s.Start + 1
		}
		ratio := float64(contiguous) / float64(len(phase))
		continuous = ratio >= opts.MinContinuityRatio && len(brokenAt) == 0
	}
	return &Result{
		PhasePoints: phase,
		BrokenAt:    brokenAt,
		Segments:    segments,
		Continuous:  continuous,
		JumpCount:   jumps,
	}, nil
}

// DetectJumps 返回跳变点索引（index > 0 且与前一索引的相位差超过阈值）。
func DetectJumps(phase []float64, thresholdRad float64) ([]int, int) {
	var brokenAt []int
	jumps := 0
	for k := 1; k < len(phase); k++ {
		delta := math.Abs(phase[k] - phase[k-1])
		if delta > thresholdRad {
			brokenAt = append(brokenAt, k)
			jumps++
		}
	}
	return brokenAt, jumps
}

// SplitSegments 按断裂点划分连续段，短段（点数 < minLen）并入相邻段。
func SplitSegments(total int, brokenAt []int, minLen int) []Segment {
	if total <= 0 {
		return nil
	}
	if len(brokenAt) == 0 {
		return []Segment{{Start: 0, End: total - 1}}
	}
	// 由断裂点构造原始分段边界。
	boundaries := []int{0}
	boundaries = append(boundaries, brokenAt...)
	boundaries = append(boundaries, total)
	var raw []Segment
	for i := 0; i+1 < len(boundaries); i++ {
		raw = append(raw, Segment{Start: boundaries[i], End: boundaries[i+1] - 1})
	}
	// 合并过短段。
	var merged []Segment
	for _, s := range raw {
		if len(merged) == 0 {
			merged = append(merged, s)
			continue
		}
		if s.End-s.Start+1 < minLen {
			// 并入前一段。
			merged[len(merged)-1].End = s.End
			continue
		}
		merged = append(merged, s)
	}
	return merged
}

// ConcatTracks 把多阵元的相位轨迹按时间索引拼接为单条主导轨迹：
// 每个时间索引取中位数相位（抗离群）。
func ConcatTracks(tracks [][]float64) ([]float64, error) {
	if len(tracks) == 0 {
		return nil, fmt.Errorf("no tracks to concat")
	}
	n := len(tracks[0])
	for _, t := range tracks {
		if len(t) != n {
			return nil, fmt.Errorf("track length mismatch")
		}
	}
	out := make([]float64, n)
	for k := 0; k < n; k++ {
		col := make([]float64, len(tracks))
		for j := 0; j < len(tracks); j++ {
			col[j] = tracks[j][k]
		}
		out[k] = median(col)
	}
	return out, nil
}

// median 返回中位数。
func median(vals []float64) float64 {
	if len(vals) == 0 {
		return 0
	}
	// 简易插入排序（数据量小）。
	for i := 1; i < len(vals); i++ {
		for j := i; j > 0 && vals[j] < vals[j-1]; j-- {
			vals[j], vals[j-1] = vals[j-1], vals[j]
		}
	}
	m := len(vals) / 2
	if len(vals)%2 == 0 {
		return (vals[m-1] + vals[m]) / 2
	}
	return vals[m]
}
