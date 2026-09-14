package server

import (
	"context"
	"errors"
	"net"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/elcuervo/emb/internal/onnx"
)

func TestInferenceAdmissionReservesFinalSlotAtomically(t *testing.T) {
	s := New("", nil, "", "", nil, WithMaxConcurrentRequests(1))
	start := make(chan struct{})
	type outcome struct {
		release func()
		err     string
	}
	outcomes := make(chan outcome, 32)
	var wg sync.WaitGroup
	for range 32 {
		wg.Go(func() {
			<-start
			release, err := s.admitInference()
			outcomes <- outcome{release: release, err: err}
		})
	}
	close(start)
	wg.Wait()
	close(outcomes)

	var accepted int
	for result := range outcomes {
		if result.release != nil {
			accepted++
			defer result.release()
		} else if !strings.HasPrefix(result.err, "ERR busy") {
			t.Fatalf("rejected admission error = %q", result.err)
		}
	}
	if accepted != 1 {
		t.Fatalf("accepted %d simultaneous requests at cap 1", accepted)
	}
}

func TestDrainingRejectsNewAdmissionAndWaitsForAcceptedWork(t *testing.T) {
	s := New("", nil, "", "", nil)
	release, replyErr := s.admitInference()
	if replyErr != "" {
		t.Fatal(replyErr)
	}
	s.beginDraining()
	if nextRelease, err := s.admitInference(); nextRelease != nil || err != "ERR server shutting down" {
		t.Fatalf("admission during drain returned release=%t, error=%q", nextRelease != nil, err)
	}

	done := make(chan struct{})
	go func() {
		s.active.Wait()
		close(done)
	}()
	select {
	case <-done:
		t.Fatal("drain completed before accepted work released")
	case <-time.After(20 * time.Millisecond):
	}
	release()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("drain did not complete")
	}
}

func TestShutdownPreservesAcceptedConnectionUntilReply(t *testing.T) {
	gate := make(chan struct{})
	addr, srv := serveWithPool(t,
		func() (onnx.Session, error) { return &blockingSession{gate: gate}, nil },
		1, 0, 32,
	)
	client := dial(t, addr)
	defer client.Close()
	client.Write(respCommand("EMB", "test", "hello"))
	waitFor(t, func() bool { return srv.activeReqs.Load() == 1 })

	shutdown := make(chan error, 1)
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		shutdown <- srv.Shutdown(ctx)
	}()
	waitFor(t, srv.shuttingDown.Load)

	// A timeout is also what an accepted idle connection does; send PING to
	// tell rejection (EOF/reset) apart from acceptance (reply or timeout).
	probe, err := net.DialTimeout("tcp", addr, 100*time.Millisecond)
	if err == nil {
		defer probe.Close()
		_ = probe.SetDeadline(time.Now().Add(200 * time.Millisecond))
		_, _ = probe.Write([]byte("*1\r\n$4\r\nPING\r\n"))
		buf := make([]byte, 1)
		n, readErr := probe.Read(buf)
		if readErr == nil {
			t.Fatalf("drained server answered the probe connection: %q", buf[:n])
		}
		if errors.Is(readErr, os.ErrDeadlineExceeded) {
			t.Fatal("drained server kept the probe connection open")
		}
	}

	close(gate)
	if reply := readRESP(t, client); !strings.HasPrefix(reply, "$") {
		t.Fatalf("accepted request did not receive embedding reply: %q", reply)
	}
	if err := <-shutdown; err != nil {
		t.Fatal(err)
	}
}

func TestShutdownReturnsTimeoutWithoutWaitingIndefinitely(t *testing.T) {
	s := New("", nil, "", "", nil)
	release, errText := s.admitInference()
	if errText != "" {
		t.Fatal(errText)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()
	err := s.Shutdown(ctx)
	if !errors.Is(err, ErrShutdownTimeout) {
		t.Fatalf("Shutdown error = %v, want ErrShutdownTimeout", err)
	}
	release()
}
