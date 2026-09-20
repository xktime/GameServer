package utils

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"
)

func TestHttpDoHonorsContextDeadline(t *testing.T) {
	requestStarted := make(chan struct{})
	releaseResponse := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		close(requestStarted)
		<-releaseResponse
	}))
	defer server.Close()
	defer close(releaseResponse)

	serverURL, err := url.Parse(server.URL)
	if err != nil {
		t.Fatalf("parse test server URL: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	result := make(chan error, 1)
	go func() {
		_, requestErr := HttpDo(ctx, "/", HttpGetMethod, "", serverURL.Scheme, serverURL.Host)
		result <- requestErr
	}()

	select {
	case <-requestStarted:
	case <-time.After(time.Second):
		t.Fatal("HTTP request did not reach the server")
	}

	select {
	case requestErr := <-result:
		if !errors.Is(requestErr, context.DeadlineExceeded) {
			t.Fatalf("HttpDo() error = %v, want context deadline exceeded", requestErr)
		}
	case <-time.After(250 * time.Millisecond):
		t.Fatal("HttpDo did not return after the context deadline")
	}
}
