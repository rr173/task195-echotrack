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
