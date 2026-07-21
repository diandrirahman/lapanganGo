package main

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

type fakeHTTPServer struct {
	started       chan struct{}
	finished      chan struct{}
	release       chan struct{}
	startedOnce   sync.Once
	releaseOnce   sync.Once
	shutdownCalls atomic.Int32
	closeCalls    atomic.Int32
	listenErr     error
	shutdownErr   error
}

func newFakeHTTPServer(listenErr, shutdownErr error) *fakeHTTPServer {
	return &fakeHTTPServer{
		started:     make(chan struct{}),
		finished:    make(chan struct{}),
		release:     make(chan struct{}),
		listenErr:   listenErr,
		shutdownErr: shutdownErr,
	}
}

func (s *fakeHTTPServer) ListenAndServe() error {
	s.startedOnce.Do(func() { close(s.started) })
	defer close(s.finished)
	if s.listenErr != nil {
		return s.listenErr
	}
	<-s.release
	return http.ErrServerClosed
}

func (s *fakeHTTPServer) Shutdown(context.Context) error {
	s.shutdownCalls.Add(1)
	s.releaseOnce.Do(func() { close(s.release) })
	return s.shutdownErr
}

func (s *fakeHTTPServer) Close() error {
	s.closeCalls.Add(1)
	s.releaseOnce.Do(func() { close(s.release) })
	return nil
}

func waitForServerTestSignal(t *testing.T, signal <-chan struct{}) {
	t.Helper()
	select {
	case <-signal:
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for fake server signal")
	}
}

func TestRunHTTPServer_ListenFailureReturnsToCaller(t *testing.T) {
	server := newFakeHTTPServer(errors.New("listener provider secret"), nil)

	err := runHTTPServer(context.Background(), server)

	if err == nil || !strings.Contains(err.Error(), "listener provider secret") {
		t.Fatalf("expected listener error, got %v", err)
	}
	if got := server.shutdownCalls.Load(); got != 0 {
		t.Fatalf("expected no shutdown for pre-serve failure, got %d calls", got)
	}
	waitForServerTestSignal(t, server.finished)
}

func TestRunHTTPServer_EarlyServerClosedIsFailure(t *testing.T) {
	server := newFakeHTTPServer(http.ErrServerClosed, nil)

	err := runHTTPServer(context.Background(), server)

	if err == nil || !strings.Contains(err.Error(), "server_closed_unexpectedly") {
		t.Fatalf("expected unexpected early close error, got %v", err)
	}
	waitForServerTestSignal(t, server.finished)
}

func TestRunHTTPServer_ContextCancellationShutsDownExactlyOnce(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	server := newFakeHTTPServer(nil, nil)
	result := make(chan error, 1)
	go func() { result <- runHTTPServer(ctx, server) }()

	waitForServerTestSignal(t, server.started)
	cancel()

	select {
	case err := <-result:
		if err != nil {
			t.Fatalf("expected clean shutdown, got %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("runHTTPServer did not return after context cancellation")
	}
	if got := server.shutdownCalls.Load(); got != 1 {
		t.Fatalf("expected exactly one shutdown call, got %d", got)
	}
	if got := server.closeCalls.Load(); got != 0 {
		t.Fatalf("expected no forced close after clean shutdown, got %d calls", got)
	}
	waitForServerTestSignal(t, server.finished)
}

func TestRunHTTPServer_ShutdownFailureReturnsToCaller(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	server := newFakeHTTPServer(nil, errors.New("shutdown provider secret"))
	result := make(chan error, 1)
	go func() { result <- runHTTPServer(ctx, server) }()

	waitForServerTestSignal(t, server.started)
	cancel()

	select {
	case err := <-result:
		if err == nil || !strings.Contains(err.Error(), "shutdown provider secret") {
			t.Fatalf("expected shutdown error, got %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("runHTTPServer did not return after shutdown failure")
	}
	if got := server.shutdownCalls.Load(); got != 1 {
		t.Fatalf("expected exactly one shutdown call, got %d", got)
	}
	if got := server.closeCalls.Load(); got != 1 {
		t.Fatalf("expected forced close after shutdown failure, got %d calls", got)
	}
	waitForServerTestSignal(t, server.finished)
}
