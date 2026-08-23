package service

import (
	"context"
	"fmt"

	"task195-echotrack/internal/model"
	"task195-echotrack/internal/review"
	"task195-echotrack/internal/store"
)

// ReviewService 复核与解释包管理用例。
type ReviewService struct {
	store *store.Store
}

// NewReviewService 构造复核服务。
func NewReviewService(s *store.Store) *ReviewService {
	return &ReviewService{store: s}
}

// AddAnnotation 添加标注。
func (r *ReviewService) AddAnnotation(ctx context.Context, in review.AnnotationInput) (*model.Annotation, error) {
	if err := in.Validate(); err != nil {
		return nil, err
	}
	if _, err := r.store.GetBatch(ctx, in.BatchID); err != nil {
		return nil, err
	}
	a := &model.Annotation{
		ID:         IDGen("ann"),
		BatchID:    in.BatchID,
		TargetType: in.TargetType,
		TargetID:   in.TargetID,
		Note:       in.Note,
		Author:     in.Author,
		CreatedAt:  Now(),
	}
	if err := r.store.AddAnnotation(ctx, a); err != nil {
		return nil, err
	}
	return a, nil
}

// ListAnnotations 列出批次标注。
func (r *ReviewService) ListAnnotations(ctx context.Context, batchID string) ([]*model.Annotation, error) {
	if _, err := r.store.GetBatch(ctx, batchID); err != nil {
		return nil, err
	}
	return r.store.ListAnnotations(ctx, batchID)
}

// AddSegment 人工分段：将轨迹段记录并（可选）确认轨迹。
func (r *ReviewService) AddSegment(ctx context.Context, d review.SegmentDecision) (*model.Segment, error) {
	if err := d.Validate(); err != nil {
		return nil, err
	}
	track, err := r.store.GetTrack(ctx, d.TrackID)
	if err != nil {
		return nil, err
	}
	if d.EndIdx >= len(track.PhasePoints) {
		return nil, fmt.Errorf("segment end %d out of range (len %d)", d.EndIdx, len(track.PhasePoints))
	}
	seg := &model.Segment{
		ID:        IDGen("seg"),
		TrackID:   d.TrackID,
		StartIdx:  d.StartIdx,
		EndIdx:    d.EndIdx,
		Label:     d.Label,
		Author:    d.Author,
		CreatedAt: Now(),
	}
	if err := r.store.AddSegment(ctx, seg); err != nil {
		return nil, err
	}
	if d.Confirmed {
		if err := r.store.UpdateTrackStatus(ctx, d.TrackID, model.TrackStatusConfirmed); err != nil {
			return nil, err
		}
	}
	return seg, nil
}

// ListSegments 列出轨迹段。
func (r *ReviewService) ListSegments(ctx context.Context, trackID string) ([]*model.Segment, error) {
	if _, err := r.store.GetTrack(ctx, trackID); err != nil {
		return nil, err
	}
	return r.store.ListSegments(ctx, trackID)
}

// ConfirmTrack 确认轨迹（needs_segmentation/continuous -> confirmed）。
func (r *ReviewService) ConfirmTrack(ctx context.Context, trackID string) error {
	t, err := r.store.GetTrack(ctx, trackID)
	if err != nil {
		return err
	}
	if t.Status == model.TrackStatusConfirmed {
		return nil
	}
	return r.store.UpdateTrackStatus(ctx, trackID, model.TrackStatusConfirmed)
}

// CreateInterpretation 创建解释包草稿。
func (r *ReviewService) CreateInterpretation(ctx context.Context, batchID, title string) (*model.Interpretation, error) {
	batch, err := r.store.GetBatch(ctx, batchID)
	if err != nil {
		return nil, err
	}
	if batch.Status != model.BatchStatusReviewing && batch.Status != model.BatchStatusPublished {
		return nil, fmt.Errorf("batch %s cannot create interpretation in state %s", batchID, batch.Status)
	}
	trackCount, err := r.store.CountTracksByBatch(ctx, batchID)
	if err != nil {
		return nil, err
	}
	version, err := r.store.NextInterpretationVersion(ctx, batchID)
	if err != nil {
		return nil, err
	}
	in := &model.Interpretation{
		ID:          IDGen("intp"),
		BatchID:     batchID,
		Version:     version,
		Status:      model.InterpretationStatusDraft,
		Title:       title,
		SnapshotRef: fmt.Sprintf("%s@v%d", batchID, version),
		TrackCount:  trackCount,
		CreatedAt:   Now(),
	}
	if err := r.store.CreateInterpretation(ctx, in); err != nil {
		return nil, err
	}
	return in, nil
}

// ListInterpretations 列出批次解释包。
func (r *ReviewService) ListInterpretations(ctx context.Context, batchID string) ([]*model.Interpretation, error) {
	if _, err := r.store.GetBatch(ctx, batchID); err != nil {
		return nil, err
	}
	return r.store.ListInterpretations(ctx, batchID)
}

// PublishInterpretation 发布解释包（draft -> published），并冻结批次。
// 发布后批次进入 published，迟到窗口只能创建新复核版本（新批次派生）。
func (r *ReviewService) PublishInterpretation(ctx context.Context, interpretationID string) (*model.Interpretation, error) {
	in, err := r.store.GetInterpretation(ctx, interpretationID)
	if err != nil {
		return nil, err
	}
	batch, err := r.store.GetBatch(ctx, in.BatchID)
	if err != nil {
		return nil, err
	}
	if batch.Status == model.BatchStatusArchived {
		return nil, model.ErrFrozenBatch
	}
	published, err := r.store.UpdateInterpretationState(ctx, interpretationID, model.InterpretationStatusPublished)
	if err != nil {
		return nil, err
	}
	// 冻结批次。
	if batch.Status != model.BatchStatusPublished {
		if _, err := r.store.UpdateBatchState(ctx, batch.ID, model.BatchStatusPublished, batch.WindowCursor); err != nil {
			return nil, err
		}
	}
	return published, nil
}

// SupersedeInterpretation 替代旧解释包：旧包 published -> superseded，
// 并以新版本草稿发布（由调用方再次 publish）。
func (r *ReviewService) SupersedeInterpretation(ctx context.Context, req review.SupersedeRequest) (*model.Interpretation, error) {
	if err := req.Validate(); err != nil {
		return nil, err
	}
	old, err := r.store.FindInterpretationByVersion(ctx, req.BatchID, req.OldVersion)
	if err != nil {
		return nil, err
	}
	if old.Status != model.InterpretationStatusPublished {
		return nil, fmt.Errorf("only published interpretation can be superseded")
	}
	if _, err := r.store.UpdateInterpretationState(ctx, old.ID, model.InterpretationStatusSuperseded); err != nil {
		return nil, err
	}
	// 创建新版本草稿。
	version, err := r.store.NextInterpretationVersion(ctx, old.BatchID)
	if err != nil {
		return nil, err
	}
	trackCount, err := r.store.CountTracksByBatch(ctx, old.BatchID)
	if err != nil {
		return nil, err
	}
	now := Now()
	next := &model.Interpretation{
		ID:          IDGen("intp"),
		BatchID:     old.BatchID,
		Version:     version,
		Status:      model.InterpretationStatusDraft,
		Title:       req.NewTitle,
		SnapshotRef: fmt.Sprintf("%s@v%d", old.BatchID, version),
		TrackCount:  trackCount,
		CreatedAt:   now,
	}
	if err := r.store.CreateInterpretation(ctx, next); err != nil {
		return nil, err
	}
	// 批次回到 reviewing 以接收迟到窗口。
	batch, err := r.store.GetBatch(ctx, old.BatchID)
	if err != nil {
		return nil, err
	}
	if batch.Status == model.BatchStatusPublished {
		if _, err := r.store.UpdateBatchState(ctx, batch.ID, model.BatchStatusReviewing, batch.WindowCursor); err != nil {
			return nil, err
		}
	}
	return next, nil
}
