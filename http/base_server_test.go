package http_test

import (
	"context"
	"errors"
	"net"
	"net/http"
	"testing"
	"time"

	gamehttp "gameserver/http"
)

func TestServerStartReportsAddressInUse(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("reserve address: %v", err)
	}
	defer listener.Close()

	server := gamehttp.NewServer(listener.Addr().String())
	if err := server.Start(); err == nil {
		t.Fatal("Start() error = nil, want address-in-use error")
	}
}

func TestServerShutdownReleasesListener(t *testing.T) {
	probe, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("reserve address: %v", err)
	}
	addr := probe.Addr().String()
	if err := probe.Close(); err != nil {
		t.Fatalf("release reserved address: %v", err)
	}

	server := gamehttp.NewServer(addr)
	if err := server.Start(); err != nil {
		t.Fatalf("Start() error = %v", err)
	}

	client := &http.Client{Timeout: time.Second}
	response, err := client.Get("http://" + addr + "/")
	if err != nil {
		t.Fatalf("GET /: %v", err)
	}
	if err := response.Body.Close(); err != nil {
		t.Fatalf("close response body: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := server.Shutdown(ctx); err != nil {
		t.Fatalf("Shutdown() error = %v", err)
	}

	listener, err := net.Listen("tcp", addr)
	if err != nil {
		t.Fatalf("listener was not released: %v", err)
	}
	if err := listener.Close(); err != nil {
		t.Fatalf("close verification listener: %v", err)
	}
}

func TestHandlerServerServesProvidedHandler(t *testing.T) {
	probe, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("reserve address: %v", err)
	}
	addr := probe.Addr().String()
	if err := probe.Close(); err != nil {
		t.Fatalf("release reserved address: %v", err)
	}

	server := gamehttp.NewHandlerServer(addr, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	if err := server.Start(); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		_ = server.Shutdown(ctx)
	})

	client := &http.Client{Timeout: time.Second}
	response, err := client.Get("http://" + addr + "/")
	if err != nil {
		t.Fatalf("GET /: %v", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusNoContent {
		t.Fatalf("status = %d, want %d", response.StatusCode, http.StatusNoContent)
	}
}

func TestServerShutdownClosesActiveRequestsAfterTimeout(t *testing.T) {
	probe, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("reserve address: %v", err)
	}
	addr := probe.Addr().String()
	if err := probe.Close(); err != nil {
		t.Fatalf("release reserved address: %v", err)
	}

	requestStarted := make(chan struct{})
	requestStopped := make(chan struct{})
	releaseRequest := make(chan struct{})
	server := gamehttp.NewHandlerServer(addr, http.HandlerFunc(func(_ http.ResponseWriter, request *http.Request) {
		close(requestStarted)
		select {
		case <-request.Context().Done():
		case <-releaseRequest:
		}
		close(requestStopped)
	}))
	if err := server.Start(); err != nil {
		t.Fatalf("Start() error = %v", err)
	}

	clientDone := make(chan struct{})
	go func() {
		defer close(clientDone)
		_, _ = (&http.Client{Timeout: 5 * time.Second}).Get("http://" + addr + "/")
	}()
	defer func() {
		close(releaseRequest)
		<-clientDone
	}()

	select {
	case <-requestStarted:
	case <-time.After(time.Second):
		t.Fatal("request did not reach handler")
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Millisecond)
	defer cancel()
	if err := server.Shutdown(ctx); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("Shutdown() error = %v, want %v", err, context.DeadlineExceeded)
	}

	select {
	case <-requestStopped:
	case <-time.After(200 * time.Millisecond):
		t.Fatal("active request remained after shutdown timeout")
	}
}
