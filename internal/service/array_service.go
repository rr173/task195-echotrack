package service

import (
	"context"
	"fmt"

	"task195-echotrack/internal/model"
	"task195-echotrack/internal/store"
)

// ArrayService 阵列管理用例。
type ArrayService struct {
	store *store.Store
}

// NewArrayService 构造阵列服务。
func NewArrayService(s *store.Store) *ArrayService {
	return &ArrayService{store: s}
}

// Register 注册阵列（含默认阵元）。
func (a *ArrayService) Register(ctx context.Context, id, name string, sampleRateHz float64, elementCount int) (*model.Array, error) {
	arr, err := model.NewArray(id, name, sampleRateHz, elementCount)
	if err != nil {
		return nil, err
	}
	if err := a.store.CreateArray(ctx, arr); err != nil {
		return nil, err
	}
	return arr, nil
}

// Get 查询阵列（含阵元）。
func (a *ArrayService) Get(ctx context.Context, id string) (*model.Array, error) {
	return a.store.GetArray(ctx, id)
}

// List 列出全部阵列。
func (a *ArrayService) List(ctx context.Context) ([]*model.Array, error) {
	return a.store.ListArrays(ctx)
}

// UpdateElementDelay 更新阵元延迟校正量。
func (a *ArrayService) UpdateElementDelay(ctx context.Context, arrayID string, elementNo int, delayUs float64) (*model.Element, error) {
	arr, err := a.store.GetArray(ctx, arrayID)
	if err != nil {
		return nil, err
	}
	el, err := arr.ElementByNo(elementNo)
	if err != nil {
		return nil, err
	}
	if err := a.store.UpdateElementDelay(ctx, arrayID, elementNo, delayUs); err != nil {
		return nil, err
	}
	el = el.WithDelay(delayUs)
	return &el, nil
}

// Stats 返回阵列统计。
func (a *ArrayService) Stats(ctx context.Context, arrayID string) (*model.ArrayStats, error) {
	if _, err := a.store.GetArray(ctx, arrayID); err != nil {
		return nil, err
	}
	n, err := a.store.CountBatchesByArray(ctx, arrayID)
	if err != nil {
		return nil, err
	}
	return &model.ArrayStats{ID: arrayID, BatchCount: n}, nil
}

// RequireArray 校验阵列存在并返回。
func (a *ArrayService) RequireArray(ctx context.Context, arrayID string) (*model.Array, error) {
	arr, err := a.store.GetArray(ctx, arrayID)
	if err != nil {
		return nil, fmt.Errorf("array %s: %w", arrayID, err)
	}
	return arr, nil
}
