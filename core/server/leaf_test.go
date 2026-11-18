package server_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"gameserver/core/server"
)

type testModule struct {
	started   chan struct{}
	destroyed chan struct{}
}

type testHTTPServer struct {
	startErr    error
	shutdownErr error
}

func (s *testHTTPServer) Start() error {
	return s.startErr
}

func (s *testHTTPServer) Shutdown(context.Context) error {
	return s.shutdownErr
}

func (m *testModule) OnInit() {}

func (m *testModule) Run(closeSig chan bool) {
	close(m.started)
	<-closeSig
}

func (m *testModule) OnDestroy() {
	close(m.destroyed)
}

func TestRunStopsModulesWhenContextIsCanceled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	m := &testModule{
		started:   make(chan struct{}),
		destroyed: make(chan struct{}),
	}
	done := make(chan struct{})
	go func() {
		_ = server.Run(ctx, &testHTTPServer{}, m)
		close(done)
	}()

	select {
	case <-m.started:
	case <-time.After(time.Second):
		t.Fatal("server did not start the module")
	}
	cancel()

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("server did not stop after context cancellation")
	}
	select {
	case <-m.destroyed:
	default:
		t.Fatal("server returned before destroying the module")
	}
}

func TestRunStopsModulesWhenHTTPServerFailsToStart(t *testing.T) {
	startErr := errors.New("address already in use")
	m := &testModule{
		started:   make(chan struct{}),
		destroyed: make(chan struct{}),
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	err := server.Run(ctx, &testHTTPServer{startErr: startErr}, m)
	if !errors.Is(err, startErr) {
		t.Fatalf("Run() error = %v, want %v", err, startErr)
	}
	select {
	case <-m.destroyed:
	default:
		t.Fatal("server did not destroy the module after HTTP startup failed")
	}
}

type failingShutdownModule struct {
	testModule
	err error
}

func (m *failingShutdownModule) Shutdown() error {
	m.OnDestroy()
	return m.err
}

func TestRunReportsModuleShutdownErrorsAndStillStopsOtherModules(t *testing.T) {
	for _, phase := range []string{"startup", "shutdown"} {
		t.Run(phase, func(t *testing.T) {
			cleanupErr := errors.New("room projection cleanup failed")
			httpErr := errors.New("HTTP failed")
			httpServer := &testHTTPServer{}
			if phase == "startup" {
				httpServer.startErr = httpErr
			} else {
				httpServer.shutdownErr = httpErr
			}
			first := &testModule{started: make(chan struct{}), destroyed: make(chan struct{})}
			last := &failingShutdownModule{
				testModule: testModule{started: make(chan struct{}), destroyed: make(chan struct{})},
				err:        cleanupErr,
			}
			ctx, cancel := context.WithCancel(context.Background())
			cancel()
			err := server.Run(ctx, httpServer, first, last)
			if !errors.Is(err, cleanupErr) || !errors.Is(err, httpErr) {
				t.Fatalf("Run() = %v; want both HTTP and module cleanup errors", err)
			}
			for _, stopped := range []<-chan struct{}{first.destroyed, last.destroyed} {
				select {
				case <-stopped:
				default:
					t.Fatal("cleanup failure prevented another module from stopping")
				}
			}
		})
	}
}
