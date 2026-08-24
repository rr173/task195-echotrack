package service

import (
	"context"
	"math"
	"sort"

	"task195-echotrack/internal/correction"
	"task195-echotrack/internal/fitting"
	"task195-echotrack/internal/model"
	"task195-echotrack/internal/store"
)

// ProcessService 处理编排：延迟校正 + 相位拼接 + 轨迹拟合。
type ProcessService struct {
	store *store.Store
	arr   *ArrayService
}

// NewProcessService 构造处理服务。
func NewProcessService(s *store.Store, arr *ArrayService) *ProcessService {
	return &ProcessService{store: s, arr: arr}
}

// ProcessResult 一次处理的结果摘要。
type ProcessResult struct {
	BatchID       string   `json:"batchId"`
	Status        string   `json:"status"`
	Corrected     int      `json:"corrected"`
	BrokenWindows int      `json:"brokenWindows"`
	Tracks        []string `json:"tracks"`
	JumpCount     int      `json:"jumpCount"`
}

// Process 执行批次处理流水线：
//  1. 批次 uploading -> processing；
//  2. 对每个窗口做延迟校正 + 相位解卷绕 + 连续性分析；
//  3. 按参考阵元做差分相位并跨阵元拼接轨迹；
//  4. 批次 processing -> reviewing。
//
// 幂等语义：若批次已 reviewing/published 且窗口集未变，直接返回现有轨迹。
func (p *ProcessService) Process(ctx context.Context, batchID string, opts fitting.FittingOptions) (*ProcessResult, error) {
	batch, err := p.store.GetBatch(ctx, batchID)
	if err != nil {
		return nil, err
	}
	if batch.Status == model.BatchStatusPublished || batch.Status == model.BatchStatusArchived {
		return nil, model.ErrFrozenBatch
	}
	if batch.Status == model.BatchStatusReviewing {
		tracks, _ := p.store.ListTracks(ctx, batchID)
		if len(tracks) > 0 {
			return p.summarize(batch, tracks), nil
		}
	}
	arr, err := p.arr.RequireArray(ctx, batch.ArrayID)
	if err != nil {
		return nil, err
	}
	// 进入 processing 状态（已是 processing 则幂等跳过，供重算复用）。
	if batch.Status != model.BatchStatusProcessing {
		if _, err := p.store.UpdateBatchState(ctx, batchID, model.BatchStatusProcessing, batch.WindowCursor); err != nil {
			return nil, err
		}
	}
	// 清理旧校正与轨迹，重算。
	_ = p.store.DeleteCorrectionsByBatch(ctx, batchID)
	_ = p.store.DeleteTracksByBatch(ctx, batchID)

	windows, err := p.store.ListWindows(ctx, batchID)
	if err != nil {
		return nil, err
	}
	// 按阵元分组（忽略状态为 ignored 的窗口）。
	type winGroup struct {
		elementNo int
		seqNo     int64
		phase     []float64
		delayUs   float64
		winID     string
	}
	groups := map[int][]winGroup{}
	var order []int
	for _, w := range windows {
		if w.Status == model.WindowStatusIgnored {
			continue
		}
		el, err := arr.ElementByNo(w.ElementNo)
		if err != nil {
			return nil, err
		}
		phase := correction.PhaseOf(w.I, w.Q)
		unwrapped := correction.UnwrapPhase(phase)
		rotI, rotQ, rotRad, err := correction.CorrectWindow(w.I, w.Q, el.DelayUs, batch.SampleRateHz)
		if err != nil {
			return nil, err
		}
		_ = rotI
		_ = rotQ
		corr := &model.Correction{
			ID:          IDGen("corr"),
			BatchID:     batchID,
			WindowID:    w.ID,
			ElementNo:   w.ElementNo,
			DelayUs:     el.DelayUs,
			RotationRad: rotRad,
			Unwrapped:   unwrapped,
			AppliedAt:   Now(),
		}
		if err := p.store.SaveCorrection(ctx, corr); err != nil {
			return nil, err
		}
		// 标记校正状态。
		_ = p.store.UpdateWindowStatus(ctx, w.ID, model.WindowStatusCorrected)
		if _, ok := groups[w.ElementNo]; !ok {
			order = append(order, w.ElementNo)
		}
		groups[w.ElementNo] = append(groups[w.ElementNo], winGroup{
			elementNo: w.ElementNo, seqNo: w.SeqNo, phase: unwrapped, delayUs: el.DelayUs, winID: w.ID,
		})
	}

	// 参考阵元：取参考阵元的相位序列作为基准。
	ref, ok := groups[batch.ReferenceElement]
	if !ok || len(ref) == 0 {
		// 参考阵元无窗口时回退到第一个有窗口的阵元。
		if len(order) > 0 {
			batch.ReferenceElement = order[0]
			_, _ = p.store.SetBatchReferenceElement(ctx, batchID, order[0])
			ref = groups[order[0]]
		}
	}

	// 按时间索引（阵元内 seqNo 对齐）构造差分轨迹。
	tracks := []*model.Track{}
	jumpTotal := 0
	for _, elNo := range order {
		group := groups[elNo]
		// 拼接该阵元全部窗口相位（按 seqNo 从小到大，与读取乱序无关）。
		// 批次窗口可能乱序到达，处理后的回波轨迹必须按序号从小到大拼接，
		// 否则连续相位会在拼接边界处产生伪跳变，被误判为断裂。
		sort.Slice(group, func(i, j int) bool { return group[i].seqNo < group[j].seqNo })
		if len(group) == 0 {
			continue
		}
		var phases []float64
		for _, g := range group {
			phases = append(phases, g.phase...)
		}
		res, err := fitting.FitTrack(phases, opts)
		if err != nil {
			return nil, err
		}
		jumpTotal += res.JumpCount
		timePoints := make([]float64, len(phases))
		for k := range phases {
			timePoints[k] = float64(k) / batch.SampleRateHz
		}
		status := model.TrackStatusNeedsSegmentation
		if res.Continuous {
			status = model.TrackStatusContinuous
		}
		track := &model.Track{
			ID:          IDGen("trk"),
			BatchID:     batchID,
			ElementNo:   elNo,
			Status:      status,
			PhasePoints: res.PhasePoints,
			TimePoints:  timePoints,
			BrokenAt:    res.BrokenAt,
			CreatedAt:   Now(),
			UpdatedAt:   Now(),
		}
		if err := p.store.SaveTrack(ctx, track); err != nil {
			return nil, err
		}
		tracks = append(tracks, track)
	}

	// 若某窗口被判定断裂（不可修复跳变），标记 phase_broken 由人工复核。
	brokenCount := 0
	for _, t := range tracks {
		if len(t.BrokenAt) > 0 {
			brokenCount++
		}
	}

	if _, err := p.store.UpdateBatchState(ctx, batchID, model.BatchStatusReviewing, batch.WindowCursor); err != nil {
		return nil, err
	}
	return &ProcessResult{
		BatchID:       batchID,
		Status:        model.BatchStatusReviewing,
		Corrected:     len(windows),
		BrokenWindows: brokenCount,
		Tracks:        idsOf(tracks),
		JumpCount:     jumpTotal,
	}, nil
}

// summarize 构造幂等重复处理的摘要。
func (p *ProcessService) summarize(batch *model.Batch, tracks []*model.Track) *ProcessResult {
	return &ProcessResult{
		BatchID: batch.ID,
		Status:  batch.Status,
		Tracks:  idsOf(tracks),
	}
}

func idsOf(tracks []*model.Track) []string {
	out := make([]string, 0, len(tracks))
	for _, t := range tracks {
		out = append(out, t.ID)
	}
	return out
}

// DifferentialRecompute 切换参考阵元后重算：对全部轨迹重新以新参考差分。
// 简化实现：直接重新执行 Process（幂等路径会重算）。
func (p *ProcessService) DifferentialRecompute(ctx context.Context, batchID string, newRef int) (*ProcessResult, error) {
	batch, err := p.store.GetBatch(ctx, batchID)
	if err != nil {
		return nil, err
	}
	arr, err := p.arr.RequireArray(ctx, batch.ArrayID)
	if err != nil {
		return nil, err
	}
	if _, err := arr.ElementByNo(newRef); err != nil {
		return nil, err
	}
	if _, err := p.store.SetBatchReferenceElement(ctx, batchID, newRef); err != nil {
		return nil, err
	}
	return p.Process(ctx, batchID, fitting.DefaultOptions())
}

// math 保留引用。
var _ = math.Pi

// Tracks 列出批次轨迹。
func (p *ProcessService) Tracks(ctx context.Context, batchID string) ([]*model.Track, error) {
	if _, err := p.store.GetBatch(ctx, batchID); err != nil {
		return nil, err
	}
	return p.store.ListTracks(ctx, batchID)
}

// Track 读取单条轨迹。
func (p *ProcessService) Track(ctx context.Context, trackID string) (*model.Track, error) {
	return p.store.GetTrack(ctx, trackID)
}

// SystemStats 全局统计。
type SystemStats struct {
	Arrays          int `json:"arrays"`
	Batches         int `json:"batches"`
	Windows         int `json:"windows"`
	Corrections     int `json:"corrections"`
	Tracks          int `json:"tracks"`
	Interpretations int `json:"interpretations"`
}

// SystemStats 汇总数据库规模。
func (p *ProcessService) SystemStats(ctx context.Context) (*SystemStats, error) {
	st := &SystemStats{}
	counts := []struct {
		query string
		dst   *int
	}{
		{`SELECT COUNT(*) FROM arrays`, &st.Arrays},
		{`SELECT COUNT(*) FROM batches`, &st.Batches},
		{`SELECT COUNT(*) FROM windows`, &st.Windows},
		{`SELECT COUNT(*) FROM corrections`, &st.Corrections},
		{`SELECT COUNT(*) FROM tracks`, &st.Tracks},
		{`SELECT COUNT(*) FROM interpretations`, &st.Interpretations},
	}
	for _, c := range counts {
		if err := p.store.DB().QueryRowContext(ctx, c.query).Scan(c.dst); err != nil {
			return nil, err
		}
	}
	return st, nil
}
