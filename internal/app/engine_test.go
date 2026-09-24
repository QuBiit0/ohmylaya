package app

import (
	"context"
	"testing"
	"time"

	"github.com/QuBiit0/ohmylaya/internal/config"
)

func newTestRuntime(t *testing.T, provider string) *Runtime {
	t.Helper()
	cfg := config.Default()
	cfg.Provider = provider
	return &Runtime{Layout: config.NewLayout(t.TempDir()), Config: cfg}
}

func TestRunReaperIsNoOpForHostedProvider(t *testing.T) {
	e := NewEngine(newTestRuntime(t, "typesafe"))
	done := make(chan struct{})
	go func() {
		e.RunReaper(context.Background())
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("RunReaper kept running for a hosted provider")
	}
}

func TestRunReaperReturnsWhenContextEnds(t *testing.T) {
	e := NewEngine(newTestRuntime(t, "local"))
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		e.RunReaper(ctx)
		close(done)
	}()
	// For the local provider it must keep running until ctx ends.
	select {
	case <-done:
		t.Fatal("RunReaper returned before ctx ended")
	case <-time.After(200 * time.Millisecond):
	}
	cancel()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("RunReaper did not return after ctx ended")
	}
}
