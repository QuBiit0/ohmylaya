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

func TestRemoveAllRetryGivesUpAfterBudgetWithoutTrailingWait(t *testing.T) {
	calls, waits := 0, 0
	origRemove, origWait := removeAll, removeRetryWait
	removeAll = func(string) error { calls++; return errors.New("locked") }
	removeRetryWait = func() <-chan time.Time {
		waits++
		c := make(chan time.Time, 1)
		c <- time.Time{}
		return c
	}
	defer func() { removeAll, removeRetryWait = origRemove, origWait }()
	_ = removeAllRetry(context.Background(), "anything")
	if calls != removeAttempts {
		t.Errorf("removal attempts = %d, want %d", calls, removeAttempts)
	}
	// A wait only separates attempts; none follows the final failure.
	if waits != removeAttempts-1 {
		t.Errorf("retry waits = %d, want %d", waits, removeAttempts-1)
	}
}

func TestRemoveAllRetrySucceedsOnceUnlocked(t *testing.T) {
	calls := 0
	origRemove, origWait := removeAll, removeRetryWait
	removeAll = func(string) error {
		calls++
		if calls < 3 {
			return errors.New("locked")
		}
		return nil
	}
	removeRetryWait = func() <-chan time.Time { return time.After(time.Millisecond) }
	defer func() { removeAll, removeRetryWait = origRemove, origWait }()
	if err := removeAllRetry(context.Background(), "anything"); err != nil {
		t.Fatalf("err = %v, want nil once the lock clears", err)
	}
	if calls != 3 {
		t.Errorf("removal attempts = %d, want 3", calls)
	}
}
