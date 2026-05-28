package persistence

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestOpContextUsesConfiguredTimeout(t *testing.T) {
	oldCfg := cfg
	cfg.opTimeout = 25 * time.Millisecond
	t.Cleanup(func() {
		cfg = oldCfg
	})

	ctx, cancel := opContext(context.Background())
	defer cancel()

	deadline, ok := ctx.Deadline()
	if !ok {
		t.Fatal("expected opContext to set a deadline")
	}

	remaining := time.Until(deadline)
	if remaining <= 0 || remaining > 25*time.Millisecond+20*time.Millisecond {
		t.Fatalf("unexpected remaining timeout: %v", remaining)
	}
}

func TestOpContextKeepsEarlierParentDeadline(t *testing.T) {
	oldCfg := cfg
	cfg.opTimeout = 200 * time.Millisecond
	t.Cleanup(func() {
		cfg = oldCfg
	})

	parent, parentCancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer parentCancel()

	ctx, cancel := opContext(parent)
	defer cancel()

	parentDeadline, _ := parent.Deadline()
	deadline, ok := ctx.Deadline()
	if !ok {
		t.Fatal("expected opContext to keep a deadline")
	}
	if !deadline.Equal(parentDeadline) {
		t.Fatalf("expected parent deadline %v, got %v", parentDeadline, deadline)
	}
}

func TestGetPlayerByUIDContextHonorsCanceledContext(t *testing.T) {
	resetStoreForTest(t)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, _, err := GetPlayerByUIDContext(ctx, 1)
	if err == nil {
		t.Fatal("expected canceled context error")
	}
	if !errors.Is(err, context.Canceled) && !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("expected context cancellation error, got %v", err)
	}
}
