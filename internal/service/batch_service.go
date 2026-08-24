package service

import (
	"context"
	"fmt"

	"task195-echotrack/internal/ingest"
	"task195-echotrack/internal/model"
	"task195-echotrack/internal/store"
)

// BatchService 批次与窗口管理用例。
type BatchService struct {
	store *store.Store
	arr   *ArrayService
}

// NewBatchService 构造批次服务。
func NewBatchService(s *store.Store, arr *ArrayService) *BatchService {
	return &BatchService{store: s, arr: arr}
}

// Create 创建批次（绑定阵列采样率，初始状态 uploading）。
func (b *BatchService) Create(ctx context.Context, id, arrayID string, referenceElement int) (*model.Batch, error) {
	arr, err := b.arr.RequireArray(ctx, arrayID)
	if err != nil {
		return nil, err
	}
	if referenceElement == 0 {
		referenceElement = 1
	}
	if _, err := arr.ElementByNo(referenceElement); err != nil {
		return nil, err
	}
	now := Now()
	batch := &model.Batch{
		ID:               id,
		ArrayID:          arrayID,
		Status:           model.BatchStatusUploading,
		ReferenceElement: referenceElement,
		WindowCursor:     0,
		SampleRateHz:     arr.SampleRateHz,
		CreatedAt:        now,
		UpdatedAt:        now,
	}
	if err := b.store.CreateBatch(ctx, batch); err != nil {
		return nil, err
	}
	return batch, nil
}

// Get 读取批次。
func (b *BatchService) Get(ctx context.Context, id string) (*model.Batch, error) {
	return b.store.GetBatch(ctx, id)
}

// List 列出批次（可按阵列过滤）。
func (b *BatchService) List(ctx context.Context, arrayID string) ([]*model.Batch, error) {
	return b.store.ListBatches(ctx, arrayID)
}

// IngestWindow 接收窗口（幂等）：
//  1. 批次存在且可接收；2. 阵元合法；3. 采样率一致；4. 内容校验和去重。
//
// 取消语义：整个链路透传请求 ctx；客户端在写入提交前取消请求时，
// store.UpsertWindowAndAdvanceCursor 的事务回滚——既不写窗口也不推进游标，
// 本函数返回 model.ErrRequestCancelled，调用方据此返回可识别的取消结果，
// 后续查询看不到该窗口。提交后取消（响应尚未写完）则按成功处理，不回退已提交数据。
func (b *BatchService) IngestWindow(ctx context.Context, w ingest.WindowInput) (*model.Batch, *ingest.IngestReceipt, error) {
	if err := ctx.Err(); err != nil {
		return nil, nil, model.ErrRequestCancelled
	}
	if err := w.Validate(); err != nil {
		return nil, nil, fmt.Errorf("invalid window: %w", err)
	}
	batch, err := b.store.GetBatch(ctx, w.BatchID)
	if err != nil {
		return nil, nil, err
	}
	if !batch.CanReceive() {
		return nil, nil, fmt.Errorf("%w: batch %s in state %s", model.ErrFrozenBatch, batch.ID, batch.Status)
	}
	arr, err := b.arr.RequireArray(ctx, batch.ArrayID)
	if err != nil {
		return nil, nil, err
	}
	if _, err := arr.ElementByNo(w.ElementNo); err != nil {
		return nil, nil, err
	}
	if err := ingest.ValidateSampleRate(w.SampleRate, batch.SampleRateHz); err != nil {
		return nil, nil, err
	}
	// 序号倒退防护：窗口序号必须不小于当前游标（按阵元维度在 store 层校验唯一键）。
	checksum := ingest.Checksum(w.I, w.Q)
	win := &model.Window{
		ID:         IDGen("win"),
		BatchID:    w.BatchID,
		ElementNo:  w.ElementNo,
		SeqNo:      w.SeqNo,
		I:          w.I,
		Q:          w.Q,
		SampleRate: w.SampleRate,
		Status:     model.WindowStatusNew,
		Checksum:   checksum,
		CreatedAt:  Now(),
	}
	// 原子地写入窗口并推进游标：提交前取消则事务回滚。
	receipt, inserted, batchOut, err := b.store.UpsertWindowAndAdvanceCursor(ctx, win, batch.WindowCursor)
	if err != nil {
		return nil, nil, err
	}
	_ = inserted
	if err := ctx.Err(); err != nil {
		// 提交已完成但客户端在响应前取消：数据已落库且游标已推进，
		// 无法回退；返回可识别的取消结果，让客户端感知并按需重试幂等上传。
		return batchOut, &receipt, model.ErrRequestCancelled
	}
	return batchOut, &receipt, nil
}

// ListWindows 列出批次窗口。
func (b *BatchService) ListWindows(ctx context.Context, batchID string) ([]*model.Window, error) {
	if _, err := b.store.GetBatch(ctx, batchID); err != nil {
		return nil, err
	}
	return b.store.ListWindows(ctx, batchID)
}

// GetWindow 读取窗口。
func (b *BatchService) GetWindow(ctx context.Context, windowID string) (*model.Window, error) {
	return b.store.GetWindow(ctx, windowID)
}

// IgnoreWindow 忽略某阵元窗口。
func (b *BatchService) IgnoreWindow(ctx context.Context, batchID string, elementNo int, seqNo int64) error {
	batch, err := b.store.GetBatch(ctx, batchID)
	if err != nil {
		return err
	}
	if !batch.CanReceive() && batch.Status != model.BatchStatusReviewing {
		return model.ErrFrozenBatch
	}
	return b.store.IgnoreWindow(ctx, batchID, elementNo, seqNo)
}

// ChangeReferenceElement 切换参考阵元并更新批次（不自动重算，由 Process 触发）。
func (b *BatchService) ChangeReferenceElement(ctx context.Context, batchID string, elementNo int) (*model.Batch, error) {
	batch, err := b.store.GetBatch(ctx, batchID)
	if err != nil {
		return nil, err
	}
	arr, err := b.arr.RequireArray(ctx, batch.ArrayID)
	if err != nil {
		return nil, err
	}
	if _, err := arr.ElementByNo(elementNo); err != nil {
		return nil, err
	}
	return b.store.SetBatchReferenceElement(ctx, batchID, elementNo)
}
