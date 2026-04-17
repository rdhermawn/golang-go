package webui

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"syscall"
	"testing"
	"time"
)

func TestStartServesHealthOnBoundListener(t *testing.T) {
	listener, err := Listen("127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	errCh := make(chan error, 1)
	go func() {
		errCh <- Start(ctx, listener, NewHub(10))
	}()

	client := &http.Client{Timeout: 500 * time.Millisecond}
	url := "http://" + listener.Addr().String() + "/api/health"
	deadline := time.Now().Add(2 * time.Second)

	for {
		resp, err := client.Get(url)
		if err == nil {
			defer resp.Body.Close()

			if resp.StatusCode != http.StatusOK {
				t.Fatalf("unexpected status: %d", resp.StatusCode)
			}

			var payload map[string]string
			if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
				t.Fatalf("decode health payload: %v", err)
			}
			if payload["status"] != "ok" {
				t.Fatalf("unexpected health payload: %+v", payload)
			}
			break
		}

		if time.Now().After(deadline) {
			t.Fatalf("health endpoint never became reachable: %v", err)
		}
		time.Sleep(25 * time.Millisecond)
	}

	cancel()

	select {
	case err := <-errCh:
		if err != nil {
			t.Fatalf("server exited with error: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("server did not shut down")
	}
}

func TestListenRejectsDuplicateAddress(t *testing.T) {
	listener, err := Listen("127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer listener.Close()

	dup, err := Listen(listener.Addr().String())
	if dup != nil {
		dup.Close()
		t.Fatal("expected duplicate listen to fail")
	}
	if !errors.Is(err, syscall.EADDRINUSE) {
		t.Fatalf("expected EADDRINUSE, got %v", err)
	}
}
