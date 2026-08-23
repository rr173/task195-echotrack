package service

import (
	"context"
	"math"
	"testing"

	"task195-echotrack/internal/store"
)

func TestTask195Bug10_ElementDelayUpdateIsPersisted(t *testing.T) {
	s, err := store.Open(t.TempDir() + "/db.sqlite")
	if err != nil { t.Fatal(err) }
	defer s.Close()
	ctx := context.Background()
	arr := NewArrayService(s)
	if _, err := arr.Register(ctx, "arr-b10", "array", 48000, 1); err != nil { t.Fatal(err) }
	updated, err := arr.UpdateElementDelay(ctx, "arr-b10", 1, 42.5)
	if err != nil { t.Fatal(err) }
	if math.Abs(updated.DelayUs-42.5) > 1e-9 { t.Fatalf("response delay=%v", updated.DelayUs) }
	reloaded, err := arr.Get(ctx, "arr-b10")
	if err != nil { t.Fatal(err) }
	if math.Abs(reloaded.Elements[0].DelayUs-42.5) > 1e-9 { t.Fatalf("stored delay=%v", reloaded.Elements[0].DelayUs) }
}
