package paymentworker

import (
	"context"
	"errors"
	"testing"
	"time"

	"lapangango-api/internal/paymentoutbox"
)

type workerClaimerFake struct {
	commands []paymentoutbox.Command
	claimErr error
	claims   int
	onClaim  func()
}

func (f *workerClaimerFake) ClaimNextForTypes(context.Context, string, time.Duration, []paymentoutbox.CommandType) (paymentoutbox.Command, error) {
	f.claims++
	if f.onClaim != nil {
		f.onClaim()
	}
	if f.claimErr != nil {
		return paymentoutbox.Command{}, f.claimErr
	}
	if len(f.commands) == 0 {
		return paymentoutbox.Command{}, paymentoutbox.ErrNoCommandAvailable
	}
	command := f.commands[0]
	f.commands = f.commands[1:]
	return command, nil
}

type workerProcessorFake struct {
	timeout time.Duration
	panic   bool
	calls   int
	process func(context.Context, paymentoutbox.Command) error
}

func (f *workerProcessorFake) CallTimeout() time.Duration { return f.timeout }

func (f *workerProcessorFake) Process(ctx context.Context, command paymentoutbox.Command) error {
	f.calls++
	if f.panic {
		panic("provider response must never kill worker")
	}
	if f.process != nil {
		return f.process(ctx, command)
	}
	return nil
}

type workerObserverFake struct{ events []WorkerEvent }

func (f *workerObserverFake) Record(event WorkerEvent) { f.events = append(f.events, event) }

func TestNewWorkerGeneratesUniqueOwnerAndValidatesLeaseMargin(t *testing.T) {
	claimer := &workerClaimerFake{}
	processor := &workerProcessorFake{timeout: 10 * time.Second}
	first, err := NewWorker(claimer, processor, WorkerOptions{Lease: 20 * time.Second})
	if err != nil {
		t.Fatal(err)
	}
	second, err := NewWorker(claimer, processor, WorkerOptions{Lease: 20 * time.Second})
	if err != nil {
		t.Fatal(err)
	}
	if first.owner == second.owner || !paymentoutbox.ValidateLeaseOwner(first.owner) {
		t.Fatalf("owners are not unique worker identities: %q %q", first.owner, second.owner)
	}
	if _, err := NewWorker(claimer, processor, WorkerOptions{Lease: 14 * time.Second}); !errors.Is(err, ErrInvalidWorkerTiming) {
		t.Fatalf("short lease error = %v", err)
	}
	for _, lease := range []time.Duration{
		-time.Microsecond,
		20*time.Second + time.Nanosecond,
		24*time.Hour + time.Microsecond,
	} {
		if _, err := NewWorker(claimer, processor, WorkerOptions{Lease: lease}); !errors.Is(err, paymentoutbox.ErrInvalidCommand) {
			t.Fatalf("outbox-incompatible lease %s error = %v; want ErrInvalidCommand", lease, err)
		}
	}
	if _, err := NewWorker(claimer, processor, WorkerOptions{Lease: 24 * time.Hour}); err != nil {
		t.Fatalf("maximum outbox-compatible lease rejected: %v", err)
	}
	for _, tc := range []struct {
		name      string
		timeout   time.Duration
		margin    time.Duration
		lease     time.Duration
		wantError bool
	}{
		{name: "zero processor timeout", timeout: 0, lease: 20 * time.Second, wantError: true},
		{name: "processor timeout overflow", timeout: time.Duration(1<<63 - 1), lease: 20 * time.Second, wantError: true},
		{name: "lease margin overflow", timeout: 10 * time.Second, margin: time.Duration(1<<63 - 1), lease: 20 * time.Second, wantError: true},
		{name: "maximum safe boundary", timeout: 24*time.Hour - time.Microsecond, margin: time.Microsecond, lease: 24 * time.Hour},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := NewWorker(claimer, &workerProcessorFake{timeout: tc.timeout}, WorkerOptions{Lease: tc.lease, LeaseMargin: tc.margin})
			if tc.wantError && !errors.Is(err, ErrInvalidWorkerTiming) {
				t.Fatalf("timing timeout=%s margin=%s lease=%s error=%v; want ErrInvalidWorkerTiming", tc.timeout, tc.margin, tc.lease, err)
			}
			if !tc.wantError && err != nil {
				t.Fatalf("safe timing boundary rejected: %v", err)
			}
		})
	}
	for _, owner := range []string{
		"worker:00000000-0000-0000-0000-000000000001",
		"worker:00000000-0000-6000-8000-000000000001",
		"worker:00000000-0000-4000-0000-000000000001",
	} {
		if _, err := NewWorker(claimer, processor, WorkerOptions{Owner: owner, Lease: 20 * time.Second}); !errors.Is(err, paymentoutbox.ErrInvalidCommand) {
			t.Fatalf("outbox-incompatible owner %q error = %v", owner, err)
		}
	}
}

func TestWorkerRecoversProcessorPanicAndProcessesNextCommand(t *testing.T) {
	attemptID := "00000000-0000-4000-8000-000000000001"
	claimer := &workerClaimerFake{commands: []paymentoutbox.Command{
		{ID: "command-1", CommandType: paymentoutbox.CommandPaymentInquiry, PaymentAttemptID: &attemptID, AttemptCount: 1},
		{ID: "command-2", CommandType: paymentoutbox.CommandPaymentInquiry, PaymentAttemptID: &attemptID, AttemptCount: 2},
	}}
	processor := &workerProcessorFake{timeout: time.Second, panic: true}
	observer := &workerObserverFake{}
	worker, err := NewWorker(claimer, processor, WorkerOptions{Lease: 10 * time.Second, Observer: observer})
	if err != nil {
		t.Fatal(err)
	}
	worker.processOne(context.Background())
	processor.panic = false
	worker.processOne(context.Background())
	if processor.calls != 2 {
		t.Fatalf("processor calls = %d; want 2", processor.calls)
	}
	if len(observer.events) != 1 || observer.events[0].Code != "COMMAND_PANIC_RECOVERED" {
		t.Fatalf("panic observer events = %#v", observer.events)
	}
}

func TestWorkerDoesNotExposeClaimErrorsToObserver(t *testing.T) {
	observer := &workerObserverFake{}
	worker, err := NewWorker(&workerClaimerFake{claimErr: errors.New("secret database detail")}, &workerProcessorFake{timeout: time.Second}, WorkerOptions{Lease: 10 * time.Second, Observer: observer})
	if err != nil {
		t.Fatal(err)
	}
	worker.processOne(context.Background())
	if len(observer.events) != 1 || observer.events[0].Code != "CLAIM_FAILED" {
		t.Fatalf("claim observer events = %#v", observer.events)
	}
}

func TestWorkerStartStopsCleanlyOnCancelledContext(t *testing.T) {
	observer := &workerObserverFake{}
	worker, err := NewWorker(
		&workerClaimerFake{},
		&workerProcessorFake{timeout: time.Second},
		WorkerOptions{Lease: 10 * time.Second, Poll: time.Millisecond, Observer: observer},
	)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	worker.Start(ctx)
	if len(observer.events) != 1 || observer.events[0].Code != "WORKER_STOPPED" {
		t.Fatalf("stop observer events = %#v", observer.events)
	}
	noopWorkerObserver{}.Record(WorkerEvent{Code: "IGNORED"})
}

func TestWorkerCancellationDoesNotClaimOrProcessNewCommand(t *testing.T) {
	claimer := &workerClaimerFake{commands: []paymentoutbox.Command{{ID: "command-cancelled"}}}
	processor := &workerProcessorFake{timeout: time.Second}
	worker, err := NewWorker(claimer, processor, WorkerOptions{Lease: 10 * time.Second})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	worker.processOne(ctx)
	if claimer.claims != 0 || processor.calls != 0 {
		t.Fatalf("cancelled worker claim/process counts = %d/%d; want 0/0", claimer.claims, processor.calls)
	}
}

func TestWorkerCancellationAfterClaimDefersCommandWithoutProcessing(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	claimer := &workerClaimerFake{
		commands: []paymentoutbox.Command{{ID: "command-deferred"}},
		onClaim:  cancel,
	}
	processor := &workerProcessorFake{timeout: time.Second}
	worker, err := NewWorker(claimer, processor, WorkerOptions{Lease: 10 * time.Second})
	if err != nil {
		t.Fatal(err)
	}
	worker.processOne(ctx)
	if claimer.claims != 1 || processor.calls != 0 {
		t.Fatalf("post-claim cancellation counts = %d/%d; want 1/0", claimer.claims, processor.calls)
	}
}

func TestWorkerCancellationAllowsInFlightProcessorToReturnWithoutFailureEvent(t *testing.T) {
	started := make(chan struct{})
	completed := make(chan struct{})
	claimer := &workerClaimerFake{commands: []paymentoutbox.Command{{ID: "command-in-flight"}}}
	processor := &workerProcessorFake{
		timeout: time.Second,
		process: func(ctx context.Context, _ paymentoutbox.Command) error {
			close(started)
			<-ctx.Done()
			close(completed)
			return ctx.Err()
		},
	}
	observer := &workerObserverFake{}
	worker, err := NewWorker(claimer, processor, WorkerOptions{Lease: 10 * time.Second, Poll: time.Millisecond, Observer: observer})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan struct{})
	go func() {
		worker.Start(ctx)
		close(done)
	}()
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("worker did not begin processing")
	}
	cancel()
	select {
	case <-completed:
	case <-time.After(time.Second):
		t.Fatal("in-flight processor did not receive cancellation")
	}
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("worker did not stop after cancellation")
	}
	if claimer.claims != 1 || processor.calls != 1 {
		t.Fatalf("in-flight cancellation claim/process counts = %d/%d; want 1/1", claimer.claims, processor.calls)
	}
	if len(observer.events) != 1 || observer.events[0].Code != "WORKER_STOPPED" {
		t.Fatalf("in-flight cancellation events = %#v; want only WORKER_STOPPED", observer.events)
	}
}
