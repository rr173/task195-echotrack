package model

// 统计与聚合辅助：供 API 展示与复核决策使用。

// WindowStats 窗口统计。
type WindowStats struct {
	Total       int `json:"total"`
	Corrected   int `json:"corrected"`
	PhaseBroken int `json:"phaseBroken"`
	Ignored     int `json:"ignored"`
}

// TrackContinuity 轨迹连续性统计。
type TrackContinuity struct {
	TrackID       string  `json:"trackId"`
	ElementNo     int     `json:"elementNo"`
	Points        int     `json:"points"`
	BrokenPoints  int     `json:"brokenPoints"`
	Continuity    float64 `json:"continuity"` // 0..1，连续点数占比
	NeedsReview   bool    `json:"needsReview"`
}

// ComputeWindowStats 聚合窗口状态分布。
func ComputeWindowStats(windows []*Window) *WindowStats {
	st := &WindowStats{Total: len(windows)}
	for _, w := range windows {
		switch w.Status {
		case WindowStatusCorrected:
			st.Corrected++
		case WindowStatusPhaseBroken:
			st.PhaseBroken++
		case WindowStatusIgnored:
			st.Ignored++
		}
	}
	return st
}

// ComputeContinuity 计算轨迹连续性：连续点数占比低于 0.9 时建议人工复核。
func ComputeContinuity(t *Track) *TrackContinuity {
	if t == nil || len(t.PhasePoints) == 0 {
		return nil
	}
	n := len(t.PhasePoints)
	brokenSet := make(map[int]struct{}, len(t.BrokenAt))
	for _, b := range t.BrokenAt {
		brokenSet[b] = struct{}{}
	}
	brokenCount := 0
	for k := 1; k < n; k++ {
		if _, ok := brokenSet[k]; ok {
			brokenCount++
		}
	}
	cont := 1.0
	if n > 1 {
		cont = float64(n-1-brokenCount) / float64(n-1)
	}
	if cont < 0 {
		cont = 0
	}
	return &TrackContinuity{
		TrackID:      t.ID,
		ElementNo:    t.ElementNo,
		Points:       n,
		BrokenPoints: brokenCount,
		Continuity:   cont,
		NeedsReview:  cont < 0.9,
	}
}
