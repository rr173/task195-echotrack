package service

import (
	"context"
	"errors"
	"testing"

	"task195-echotrack/internal/model"
	"task195-echotrack/internal/store"
)

// TestUpdateElementDelayReturnsNewValueAndPersists 验证更新阵元延迟后：
//   - 返回值携带新延迟（接口响应即新值）；
//   - 重新读取阵列得到的持久化值也是新延迟，而非旧值。
func TestUpdateElementDelayReturnsNewValueAndPersists(t *testing.T) {
	ctx := context.Background()
	s, err := store.Open(t.TempDir() + "/echotrack.db")
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	arrSvc := NewArrayService(s)
	arr, err := arrSvc.Register(ctx, "arr-svc-delay", "svc delay", 48000, 4)
	if err != nil {
		t.Fatal(err)
	}

	const want = 37.0
	updated, err := arrSvc.UpdateElementDelay(ctx, arr.ID, 3, want)
	if err != nil {
		t.Fatalf("update delay: %v", err)
	}
	// 接口返回值必须是新延迟。
	if updated.DelayUs != want {
		t.Errorf("response delay: got %v want %v", updated.DelayUs, want)
	}

	// 重新读取阵列，持久化值也必须是新延迟。
	reloaded, err := arrSvc.Get(ctx, arr.ID)
	if err != nil {
		t.Fatalf("reload array: %v", err)
	}
	got, err := reloaded.ElementByNo(3)
	if err != nil {
		t.Fatalf("find element: %v", err)
	}
	if got.DelayUs != want {
		t.Errorf("persisted delay: got %v want %v", got.DelayUs, want)
	}
	// 其余阵元延迟保持默认 0，未被误改。
	for _, el := range reloaded.Elements {
		if el.ElementNo == 3 {
			continue
		}
		if el.DelayUs != 0 {
			t.Errorf("element %d delay should remain 0, got %v", el.ElementNo, el.DelayUs)
		}
	}

	// 更新不存在的阵元返回 ErrUnknownElement。
	if _, err := arrSvc.UpdateElementDelay(ctx, arr.ID, 99, want); !errors.Is(err, model.ErrUnknownElement) {
		t.Errorf("update missing element: got %v want ErrUnknownElement", err)
	}
}
