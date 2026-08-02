package paymentwebhookprocessor

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"
)

type fakeRunner struct {
	calls  atomic.Int32
	cancel context.CancelFunc
}

func (f *fakeRunner) ProcessOne(context.Context) (bool, error) {
	f.calls.Add(1)
	if f.cancel != nil {
		f.cancel()
	}
	return true, nil
}

func TestWorkerCancellationPreventsNewClaims(t *testing.T) {
	runner := &fakeRunner{}
	worker, err := NewWorker(runner, time.Millisecond)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	worker.Start(ctx)
	if calls := runner.calls.Load(); calls != 0 {
		t.Fatalf("processor calls after cancellation = %d; want 0", calls)
	}
}

func TestWorkerStopsAfterInFlightClaimCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	runner := &fakeRunner{cancel: cancel}
	worker, err := NewWorker(runner, time.Millisecond)
	if err != nil {
		t.Fatal(err)
	}
	worker.Start(ctx)
	if calls := runner.calls.Load(); calls != 1 {
		t.Fatalf("processor calls = %d; want 1", calls)
	}
}

type claimedErrorRunner struct {
	calls  atomic.Int32
	first  chan struct{}
	second chan struct{}
}

func (r *claimedErrorRunner) ProcessOne(context.Context) (bool, error) {
	if r.calls.Add(1) == 1 {
		close(r.first)
	} else {
		select {
		case <-r.second:
		default:
			close(r.second)
		}
	}
	return true, errors.New("durable processing failure")
}

func TestWorkerWaitsAfterClaimedProcessingError(t *testing.T) {
	runner := &claimedErrorRunner{first: make(chan struct{}), second: make(chan struct{})}
	worker, err := NewWorker(runner, 100*time.Millisecond)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		worker.Start(ctx)
		close(done)
	}()
	select {
	case <-runner.first:
	case <-time.After(time.Second):
		t.Fatal("worker did not make the first processing attempt")
	}
	select {
	case <-runner.second:
		t.Fatal("worker retried a claimed processing error before its poll interval")
	case <-time.After(20 * time.Millisecond):
	}
	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("worker did not stop while waiting after an error")
	}
	if calls := runner.calls.Load(); calls != 1 {
		t.Fatalf("processor calls after cancellation = %d; want 1", calls)
	}
}

type slowClaimedErrorRunner struct {
	calls   atomic.Int32
	started chan struct{}
	release chan struct{}
	retried chan struct{}
}

func (r *slowClaimedErrorRunner) ProcessOne(context.Context) (bool, error) {
	if r.calls.Add(1) == 1 {
		close(r.started)
		<-r.release
	} else {
		select {
		case <-r.retried:
		default:
			close(r.retried)
		}
	}
	return true, errors.New("slow durable processing failure")
}

func TestWorkerStartsFreshWaitAfterSlowClaimedError(t *testing.T) {
	const pollInterval = 60 * time.Millisecond
	runner := &slowClaimedErrorRunner{
		started: make(chan struct{}), release: make(chan struct{}), retried: make(chan struct{}),
	}
	worker, err := NewWorker(runner, pollInterval)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		worker.Start(ctx)
		close(done)
	}()
	select {
	case <-runner.started:
	case <-time.After(time.Second):
		t.Fatal("worker did not start slow processing")
	}

	// Keep ProcessOne in flight longer than the poll interval. A long-lived
	// ticker would already have a pending tick when the error is released.
	time.Sleep(2 * pollInterval)
	close(runner.release)
	select {
	case <-runner.retried:
		t.Fatal("worker retried before a fresh post-error poll interval elapsed")
	case <-time.After(pollInterval / 3):
	}

	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("worker did not cancel during the fresh post-error wait")
	}
	if calls := runner.calls.Load(); calls != 1 {
		t.Fatalf("slow-error processor calls after cancellation = %d; want 1", calls)
	}
}

type drainingRunner struct {
	calls  atomic.Int32
	cancel context.CancelFunc
}

func (r *drainingRunner) ProcessOne(context.Context) (bool, error) {
	if r.calls.Add(1) == 2 {
		r.cancel()
	}
	return true, nil
}

func TestWorkerDrainsSuccessfulClaimsWithoutWaiting(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	runner := &drainingRunner{cancel: cancel}
	worker, err := NewWorker(runner, 200*time.Millisecond)
	if err != nil {
		t.Fatal(err)
	}
	started := time.Now()
	worker.Start(ctx)
	if calls := runner.calls.Load(); calls != 2 {
		t.Fatalf("successful processor calls = %d; want 2", calls)
	}
	if elapsed := time.Since(started); elapsed >= 100*time.Millisecond {
		t.Fatalf("successful claims waited for poll interval: %s", elapsed)
	}
}
