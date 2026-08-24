package store

import (
	"context"
	"testing"

	"task195-echotrack/internal/model"
)

func TestOpenPersistsArrayAcrossRestart(t *testing.T) {
	dbPath := t.TempDir() + "/echotrack.db"
	ctx := context.Background()

	s, err := Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	arr, err := model.NewArray("arr-store-test", "store test", 48000, 2)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.CreateArray(ctx, arr); err != nil {
		t.Fatal(err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}

	s, err = Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	recovered, err := s.GetArray(ctx, arr.ID)
	if err != nil {
		t.Fatal(err)
	}
	if recovered.Name != arr.Name || len(recovered.Elements) != 2 {
		t.Fatalf("recovered array mismatch: %+v", recovered)
	}
}

func TestUpdateElementDelayPersistsValue(t *testing.T) {
	dbPath := t.TempDir() + "/echotrack.db"
	ctx := context.Background()

	s, err := Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	arr, err := model.NewArray("arr-delay-test", "delay test", 48000, 2)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.CreateArray(ctx, arr); err != nil {
		t.Fatal(err)
	}

	const want = 12.5
	if err := s.UpdateElementDelay(ctx, arr.ID, 2, want); err != nil {
		t.Fatalf("update delay: %v", err)
	}

	// 重新读取（新连接），断言持久化的是新延迟而非旧值 0。
	s2, err := Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer s2.Close()
	recovered, err := s2.GetArray(ctx, arr.ID)
	if err != nil {
		t.Fatal(err)
	}
	for _, el := range recovered.Elements {
		got := el.DelayUs
		expect := 0.0
		if el.ElementNo == 2 {
			expect = want
		}
		if got != expect {
			t.Errorf("element %d delay: got %v want %v", el.ElementNo, got, expect)
		}
	}
}
