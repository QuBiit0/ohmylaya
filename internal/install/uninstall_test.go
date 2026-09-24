package install

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestRemoveAllRetryStopsWhenContextEnds(t *testing.T) {
	calls := 0
	orig := removeAll
	removeAll = func(string) error { calls++; return errors.New("locked") }
	defer func() { removeAll = orig }()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	start := time.Now()
	if err := removeAllRetry(ctx, "anything"); err == nil {
		t.Fatal("want the removal error")
	}
	if calls != 1 {
		t.Errorf("removal attempts = %d, want 1 once ctx is done", calls)
	}
	if time.Since(start) > 200*time.Millisecond {
		t.Errorf("took %v, want no retry wait after ctx is done", time.Since(start))
	}
}

func TestRemoveAllRetryDoesNotSleepAfterLastAttempt(t *testing.T) {
	calls := 0
	orig := removeAll
	removeAll = func(string) error { calls++; return errors.New("locked") }
	defer func() { removeAll = orig }()
	start := time.Now()
	_ = removeAllRetry(context.Background(), "anything")
	if calls != removeAttempts {
		t.Errorf("removal attempts = %d, want %d", calls, removeAttempts)
	}
	if max := time.Duration(removeAttempts-1)*removeRetryWait + 400*time.Millisecond; time.Since(start) > max {
		t.Errorf("took %v, want under %v", time.Since(start), max)
	}
}
