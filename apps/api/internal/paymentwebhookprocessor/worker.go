package paymentwebhookprocessor

import (
	"context"
	"time"
)

// Worker owns no provider client. Cancelling its context is the processor kill
// switch and prevents any additional inbox claim.
type Worker struct {
	processor processorRunner
	poll      time.Duration
}

type processorRunner interface {
	ProcessOne(context.Context) (bool, error)
}

func NewWorker(processor processorRunner, poll time.Duration) (*Worker, error) {
	if processor == nil {
		return nil, ErrProcessorUnavailable
	}
	if poll <= 0 {
		poll = time.Second
	}
	return &Worker{processor: processor, poll: poll}, nil
}

func (w *Worker) Start(ctx context.Context) {
	if w == nil || w.processor == nil {
		return
	}
	for {
		if ctx.Err() != nil {
			return
		}
		claimed, err := w.processor.ProcessOne(ctx)
		if err != nil {
			// A claimed event can still fail while its transaction is rolled back.
			// Wait before retrying so a persistent database or audit failure cannot
			// spin on the same durable event.
			if !waitForNextPoll(ctx, w.poll) {
				return
			}
			continue
		}
		if claimed {
			continue
		}
		if !waitForNextPoll(ctx, w.poll) {
			return
		}
	}
}

func waitForNextPoll(ctx context.Context, delay time.Duration) bool {
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-timer.C:
		return true
	}
}
