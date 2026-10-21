package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"sync/atomic"
	"syscall"

	"github.com/elcuervo/emb/internal/config"
	"github.com/elcuervo/emb/internal/server"
)

// shutdownOutcome says how the serving loop ended.
type shutdownOutcome int

const (
	// outcomeServeEnded: the serve goroutine ended on its own.
	outcomeServeEnded shutdownOutcome = iota
	// outcomeShutdown: a shutdown request ran to completion.
	outcomeShutdown
	// outcomeTimeout: the drain deadline passed with work still in flight.
	outcomeTimeout
)

// watchSignals turns SIGINT and SIGTERM into shutdown requests and enforces
// Redis's second-signal rule: a SIGINT while a shutdown is pending exits at
// once with status 1, and a repeated SIGTERM changes nothing.
func watchSignals(signals <-chan os.Signal, pending *atomic.Bool, requests chan<- server.ShutdownRequest, exit func(int)) {
	for received := range signals {
		switch received {
		case syscall.SIGINT:
			if pending.Load() {
				log.Print("You insist... exiting now.")
				exit(1)
				return
			}
			log.Print("Received SIGINT scheduling shutdown...")
			pending.Store(true)
			requests <- server.ShutdownRequest{Source: "SIGINT"}
		case syscall.SIGTERM:
			if pending.Load() {
				log.Print("Received SIGTERM while a shutdown is pending; ignoring it")
				continue
			}
			log.Print("Received SIGTERM scheduling shutdown...")
			pending.Store(true)
			requests <- server.ShutdownRequest{Source: "SIGTERM"}
		}
	}
}

// awaitShutdown serves until the serve goroutine ends or a shutdown request
// runs to completion. An aborted shutdown goes back to waiting, as in Redis.
func awaitShutdown(requests <-chan server.ShutdownRequest, serveDone <-chan error, pending *atomic.Bool, resolveFlags func(string) config.ShutdownFlags, shutdown func(context.Context, config.ShutdownFlags) error, shutdownInProgress func() bool) (shutdownOutcome, error) {
	for {
		select {
		case serveErr := <-serveDone:
			if serveErr != nil {
				return outcomeServeEnded, fmt.Errorf("server error: %w", serveErr)
			}
			return outcomeServeEnded, nil
		case req := <-requests:
			if req.Preset && !shutdownInProgress() {
				// SHUTDOWN ABORT resolved this request before it started.
				pending.Store(false)
				continue
			}
			pending.Store(true)
			flags := req.Flags
			if req.Source != "SHUTDOWN" {
				flags = resolveFlags(req.Source)
			}
			// The server creates its drain budget after the grace; the request
			// context only carries cancellation.
			err := shutdown(context.Background(), flags)
			switch {
			case errors.Is(err, server.ErrShutdownAborted):
				pending.Store(false)
				continue
			case errors.Is(err, server.ErrShutdownTimeout):
				return outcomeTimeout, err
			case err != nil:
				return outcomeShutdown, fmt.Errorf("shutting down server: %w", err)
			}
			if serveErr := <-serveDone; serveErr != nil {
				return outcomeShutdown, fmt.Errorf("server shutdown: %w", serveErr)
			}
			return outcomeShutdown, nil
		}
	}
}

// startupAborted reports whether a shutdown was requested before the listener
// opened. The request is consumed so the serving loop does not run it again.
func startupAborted(requests <-chan server.ShutdownRequest) bool {
	select {
	case <-requests:
		return true
	default:
		return false
	}
}
