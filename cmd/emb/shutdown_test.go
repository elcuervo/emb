package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"sync/atomic"
	"syscall"
	"testing"
	"time"

	"github.com/elcuervo/emb/internal/config"
	"github.com/elcuervo/emb/internal/server"
)

func TestWatchSignalsMapsSignalsToRequests(t *testing.T) {
	signals := make(chan os.Signal, 2)
	requests := make(chan server.ShutdownRequest, 2)
	var pending atomic.Bool
	exited := make(chan int, 1)
	go watchSignals(signals, &pending, requests, func(code int) { exited <- code })

	signals <- syscall.SIGTERM
	select {
	case req := <-requests:
		if req.Source != "SIGTERM" {
			t.Fatalf("SIGTERM request = %+v", req)
		}
	case <-time.After(time.Second):
		t.Fatal("SIGTERM did not produce a shutdown request")
	}
	if !pending.Load() {
		t.Fatal("a forwarded signal must mark the shutdown pending")
	}
}

func TestWatchSignalsSecondSIGTERMIsIgnored(t *testing.T) {
	signals := make(chan os.Signal, 2)
	requests := make(chan server.ShutdownRequest, 2)
	var pending atomic.Bool
	exited := make(chan int, 1)
	go watchSignals(signals, &pending, requests, func(code int) { exited <- code })

	signals <- syscall.SIGTERM
	<-requests
	signals <- syscall.SIGTERM
	select {
	case req := <-requests:
		t.Fatalf("second SIGTERM produced another request: %+v", req)
	case <-time.After(100 * time.Millisecond):
	}
	select {
	case code := <-exited:
		t.Fatalf("second SIGTERM exited with %d", code)
	default:
	}
}

func TestWatchSignalsSecondSIGINTExits(t *testing.T) {
	signals := make(chan os.Signal, 2)
	requests := make(chan server.ShutdownRequest, 2)
	var pending atomic.Bool
	exited := make(chan int, 1)
	go watchSignals(signals, &pending, requests, func(code int) { exited <- code })

	signals <- syscall.SIGINT
	<-requests
	signals <- syscall.SIGINT
	select {
	case code := <-exited:
		if code != 1 {
			t.Fatalf("second SIGINT exit code = %d, want 1", code)
		}
	case <-time.After(time.Second):
		t.Fatal("second SIGINT did not exit")
	}
}

func TestAwaitShutdownRunsTheRequestedSequence(t *testing.T) {
	requests := make(chan server.ShutdownRequest, 1)
	serveDone := make(chan error, 1)
	var pending atomic.Bool
	var gotFlags config.ShutdownFlags
	shutdown := func(_ context.Context, flags config.ShutdownFlags) error {
		gotFlags = flags
		serveDone <- nil
		return nil
	}
	requests <- server.ShutdownRequest{Source: "SIGTERM"}

	outcome, err := awaitShutdown(requests, serveDone, &pending,
		func(string) config.ShutdownFlags { return config.ShutdownFlags{Now: true} },
		shutdown, func() bool { return false })
	if err != nil {
		t.Fatalf("awaitShutdown: %v", err)
	}
	if outcome != outcomeShutdown {
		t.Fatalf("outcome = %v, want outcomeShutdown", outcome)
	}
	if !gotFlags.Now {
		t.Fatalf("signal shutdown flags = %+v, want the configured ones", gotFlags)
	}
}

func TestAwaitShutdownReturnsToWaitingAfterAbort(t *testing.T) {
	requests := make(chan server.ShutdownRequest, 2)
	serveDone := make(chan error, 1)
	var pending atomic.Bool
	aborted := make(chan struct{})
	calls := 0
	shutdown := func(_ context.Context, _ config.ShutdownFlags) error {
		calls++
		if calls == 1 {
			close(aborted)
			return server.ErrShutdownAborted
		}
		serveDone <- nil
		return nil
	}

	requests <- server.ShutdownRequest{Source: "SIGINT"}
	go func() {
		// The later SIGTERM arrives only after the abort returned to waiting.
		<-aborted
		requests <- server.ShutdownRequest{Source: "SIGTERM"}
	}()

	outcome, err := awaitShutdown(requests, serveDone, &pending,
		func(string) config.ShutdownFlags { return config.ShutdownFlags{} },
		shutdown, func() bool { return false })
	if err != nil {
		t.Fatalf("awaitShutdown: %v", err)
	}
	if outcome != outcomeShutdown {
		t.Fatalf("outcome = %v, want outcomeShutdown", outcome)
	}
	if calls != 2 {
		t.Fatalf("shutdown ran %d times, want 2 (abort then a later signal)", calls)
	}
}

func TestAwaitShutdownSkipsAnAbortedPresetRequest(t *testing.T) {
	requests := make(chan server.ShutdownRequest, 2)
	serveDone := make(chan error, 1)
	var pending atomic.Bool
	shutdown := func(_ context.Context, _ config.ShutdownFlags) error {
		serveDone <- nil
		return nil
	}
	// A command request whose ABORT resolved it before the loop started.
	requests <- server.ShutdownRequest{Source: "SHUTDOWN", Preset: true}
	requests <- server.ShutdownRequest{Source: "SIGTERM"}

	var ran int
	outcome, err := awaitShutdown(requests, serveDone, &pending,
		func(string) config.ShutdownFlags { return config.ShutdownFlags{} },
		func(ctx context.Context, flags config.ShutdownFlags) error {
			ran++
			return shutdown(ctx, flags)
		}, func() bool { return false })
	if err != nil {
		t.Fatalf("awaitShutdown: %v", err)
	}
	if outcome != outcomeShutdown || ran != 1 {
		t.Fatalf("outcome = %v, ran = %d; the aborted request must be skipped", outcome, ran)
	}
}

func TestAwaitShutdownReportsTimeout(t *testing.T) {
	requests := make(chan server.ShutdownRequest, 1)
	serveDone := make(chan error, 1)
	var pending atomic.Bool
	requests <- server.ShutdownRequest{Source: "SIGTERM"}
	outcome, err := awaitShutdown(requests, serveDone, &pending,
		func(string) config.ShutdownFlags { return config.ShutdownFlags{} },
		func(context.Context, config.ShutdownFlags) error {
			return fmt.Errorf("%w: drain", server.ErrShutdownTimeout)
		}, func() bool { return false })
	if !errors.Is(err, server.ErrShutdownTimeout) || outcome != outcomeTimeout {
		t.Fatalf("outcome = %v, err = %v, want outcomeTimeout with ErrShutdownTimeout", outcome, err)
	}
}

func TestStartupAbortedConsumesTheRequest(t *testing.T) {
	requests := make(chan server.ShutdownRequest, 1)
	if startupAborted(requests) {
		t.Fatal("startup aborted without a request")
	}
	requests <- server.ShutdownRequest{Source: "SIGTERM"}
	if !startupAborted(requests) {
		t.Fatal("startup did not see the pending request")
	}
	if startupAborted(requests) {
		t.Fatal("the request was not consumed")
	}
}
