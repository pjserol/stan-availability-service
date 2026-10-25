package main

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net"
	"net/http"
	"testing"
	"time"
)

// startServe runs serve on a random port with h, returning its address, a
// cancel func that triggers shutdown, and serve's result.
func startServe(t *testing.T, h http.Handler, drainTimeout time.Duration) (string, context.CancelFunc, <-chan error) {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	result := make(chan error, 1)
	go func() { result <- serve(ctx, ln, h, drainTimeout, slog.New(slog.DiscardHandler)) }()
	return ln.Addr().String(), cancel, result
}

// blockingHandler signals started when a request arrives, then waits for release.
func blockingHandler(started chan<- struct{}, release <-chan struct{}) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		started <- struct{}{}
		<-release
		_, _ = io.WriteString(w, "done")
	})
}

// waitUntilRefused waits for the listener to stop accepting new connections.
func waitUntilRefused(t *testing.T, addr string) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		conn, err := net.Dial("tcp", addr)
		if err != nil {
			return
		}
		_ = conn.Close()
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("listener still accepting connections after shutdown started")
}

func TestServeDrainsInFlightRequest(t *testing.T) {
	t.Parallel()
	started, release := make(chan struct{}), make(chan struct{})
	addr, cancel, result := startServe(t, blockingHandler(started, release), 5*time.Second)

	type response struct {
		body string
		err  error
	}
	responses := make(chan response, 1)
	go func() {
		resp, err := http.Get("http://" + addr)
		if err != nil {
			responses <- response{err: err}
			return
		}
		defer resp.Body.Close()
		b, err := io.ReadAll(resp.Body)
		responses <- response{body: string(b), err: err}
	}()

	<-started                 // the request is in flight
	cancel()                  // SIGTERM equivalent
	waitUntilRefused(t, addr) // shutdown has started: new connections are refused

	select {
	case err := <-result:
		t.Fatalf("serve returned %v before the in-flight request finished", err)
	case <-time.After(50 * time.Millisecond): // serve is still draining, as required
	}
	close(release) // let the in-flight request finish

	if got := <-responses; got.err != nil || got.body != "done" {
		t.Fatalf("in-flight request = %q, %v; want %q, nil", got.body, got.err, "done")
	}
	if err := <-result; err != nil {
		t.Fatalf("serve() = %v, want nil after a clean drain", err)
	}
}

func TestServeDrainTimeout(t *testing.T) {
	t.Parallel()
	started, release := make(chan struct{}), make(chan struct{})
	t.Cleanup(func() { close(release) })
	const drainTimeout = 100 * time.Millisecond
	addr, cancel, result := startServe(t, blockingHandler(started, release), drainTimeout)

	go func() {
		if resp, err := http.Get("http://" + addr); err == nil {
			resp.Body.Close()
		}
	}()

	<-started
	start := time.Now()
	cancel()

	select {
	case err := <-result:
		if !errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf("serve() = %v, want context.DeadlineExceeded", err)
		}
		if elapsed := time.Since(start); elapsed < drainTimeout {
			t.Errorf("serve gave up after %v, want at least the %v drain timeout", elapsed, drainTimeout)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("serve did not return after the drain timeout")
	}
}

func TestServeReturnsWhenServerFails(t *testing.T) {
	t.Parallel()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	_ = ln.Close() // a broken listener makes Serve fail immediately

	result := make(chan error, 1)
	go func() {
		result <- serve(context.Background(), ln, http.NotFoundHandler(), time.Second, slog.New(slog.DiscardHandler))
	}()

	select {
	case err := <-result:
		if !errors.Is(err, net.ErrClosed) {
			t.Fatalf("serve() = %v, want net.ErrClosed", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("serve kept waiting for a signal after the server failed")
	}
}
