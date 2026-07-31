package paymentworker

import (
	"context"
	"errors"
	"time"

	"lapangango-api/internal/paymentoutbox"

	"github.com/google/uuid"
)

var (
	ErrInvalidWorkerTiming = errors.New("payment worker lease must cover provider call timeout and margin")
)

// CommandClaimer is the minimal outbox seam needed by the worker loop.
type CommandClaimer interface {
	ClaimNextForTypes(context.Context, string, time.Duration, []paymentoutbox.CommandType) (paymentoutbox.Command, error)
}

// CommandProcessor is deliberately smaller than Processor so worker panic
// and lease tests can use deterministic fakes.
type CommandProcessor interface {
	Process(context.Context, paymentoutbox.Command) error
	CallTimeout() time.Duration
}

type WorkerEvent struct {
	Code             string
	CommandID        string
	CommandType      paymentoutbox.CommandType
	PaymentAttemptID string
	AttemptCount     int
}

type WorkerObserver interface {
	Record(WorkerEvent)
}

type noopWorkerObserver struct{}

func (noopWorkerObserver) Record(WorkerEvent) {}

// Worker polls only the command types explicitly enabled by its constructor.
// It has no provider-specific dependency; tests inject a payments.FakeAdapter.
type Worker struct {
	outbox       CommandClaimer
	processor    CommandProcessor
	commandTypes []paymentoutbox.CommandType
	owner        string
	lease        time.Duration
	poll         time.Duration
	observer     WorkerObserver
}

type WorkerOptions struct {
	CommandTypes []paymentoutbox.CommandType
	Owner        string
	Lease        time.Duration
	Poll         time.Duration
	LeaseMargin  time.Duration
	Observer     WorkerObserver
}

func NewWorker(outbox CommandClaimer, processor CommandProcessor, options WorkerOptions) (*Worker, error) {
	if outbox == nil || processor == nil {
		return nil, errors.New("payment worker dependencies are required")
	}
	if options.Owner == "" {
		options.Owner = "worker:" + uuid.NewString()
	}
	if !paymentoutbox.ValidateLeaseOwner(options.Owner) {
		return nil, paymentoutbox.ErrInvalidCommand
	}
	if options.Lease == 0 {
		options.Lease = 30 * time.Second
	}
	if !paymentoutbox.ValidateLeaseDuration(options.Lease) {
		return nil, paymentoutbox.ErrInvalidCommand
	}
	if options.Poll <= 0 {
		options.Poll = time.Second
	}
	if options.LeaseMargin <= 0 {
		options.LeaseMargin = 5 * time.Second
	}
	callTimeout := processor.CallTimeout()
	if callTimeout <= 0 ||
		options.LeaseMargin >= options.Lease ||
		callTimeout > options.Lease-options.LeaseMargin {
		return nil, ErrInvalidWorkerTiming
	}
	if len(options.CommandTypes) == 0 {
		options.CommandTypes = []paymentoutbox.CommandType{
			paymentoutbox.CommandPaymentCreate,
			paymentoutbox.CommandPaymentInquiry,
		}
	}
	for _, commandType := range options.CommandTypes {
		if commandType != paymentoutbox.CommandPaymentCreate && commandType != paymentoutbox.CommandPaymentInquiry {
			return nil, paymentoutbox.ErrInvalidCommand
		}
	}
	if options.Observer == nil {
		options.Observer = noopWorkerObserver{}
	}
	return &Worker{
		outbox:       outbox,
		processor:    processor,
		commandTypes: append([]paymentoutbox.CommandType(nil), options.CommandTypes...),
		owner:        options.Owner,
		lease:        options.Lease,
		poll:         options.Poll,
		observer:     options.Observer,
	}, nil
}

func (w *Worker) Start(ctx context.Context) {
	ticker := time.NewTicker(w.poll)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			w.emit(WorkerEvent{Code: "WORKER_STOPPED"})
			return
		case <-ticker.C:
			w.processOne(ctx)
		}
	}
}

func (w *Worker) processOne(ctx context.Context) {
	var command paymentoutbox.Command
	claimed := false
	defer func() {
		if recovered := recover(); recovered != nil {
			if claimed {
				w.emit(commandEvent("COMMAND_PANIC_RECOVERED", command))
			} else {
				w.emit(WorkerEvent{Code: "CLAIM_FAILED"})
			}
		}
	}()

	var err error
	command, err = w.outbox.ClaimNextForTypes(ctx, w.owner, w.lease, w.commandTypes)
	if errors.Is(err, paymentoutbox.ErrNoCommandAvailable) {
		return
	}
	if err != nil {
		w.emit(WorkerEvent{Code: "CLAIM_FAILED"})
		return
	}
	claimed = true
	if err := w.processor.Process(ctx, command); err != nil {
		if errors.Is(err, paymentoutbox.ErrLeaseConflict) {
			w.emit(commandEvent("LEASE_CONFLICT", command))
			return
		}
		w.emit(commandEvent("COMMAND_FAILED", command))
	}
}

func commandEvent(code string, command paymentoutbox.Command) WorkerEvent {
	event := WorkerEvent{Code: code, CommandID: command.ID, CommandType: command.CommandType, AttemptCount: command.AttemptCount}
	if command.PaymentAttemptID != nil {
		event.PaymentAttemptID = *command.PaymentAttemptID
	}
	return event
}

func (w *Worker) emit(event WorkerEvent) {
	defer func() { _ = recover() }()
	if w.observer != nil {
		w.observer.Record(event)
	}
}

var _ CommandClaimer = (*paymentoutbox.Repository)(nil)
var _ CommandProcessor = (*Processor)(nil)
