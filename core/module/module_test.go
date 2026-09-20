package module_test

import (
	"sync"
	"sync/atomic"
	"testing"

	"gameserver/core/module"
)

type testModule struct {
	onInit    func()
	onRun     func(chan bool)
	onDestroy func()
}

func (m *testModule) OnInit() {
	if m.onInit != nil {
		m.onInit()
	}
}

func (m *testModule) Run(closeSig chan bool) {
	if m.onRun != nil {
		m.onRun(closeSig)
		return
	}
	<-closeSig
}

func (m *testModule) OnDestroy() {
	if m.onDestroy != nil {
		m.onDestroy()
	}
}

func TestRuntimeInitializesEveryModuleBeforeRunningAnyModule(t *testing.T) {
	var initialized atomic.Int32
	runAfterInitialized := make(chan int32, 2)

	newModule := func() *testModule {
		return &testModule{
			onInit: func() {
				initialized.Add(1)
			},
			onRun: func(closeSig chan bool) {
				runAfterInitialized <- initialized.Load()
				<-closeSig
			},
		}
	}

	runtime := module.NewRuntime(newModule(), newModule())
	runtime.Start()
	t.Cleanup(runtime.Stop)

	for range 2 {
		if got := <-runAfterInitialized; got != 2 {
			t.Fatalf("module started after %d initializations, want 2", got)
		}
	}
}

func TestRuntimeStopsModulesInReverseOrder(t *testing.T) {
	var mu sync.Mutex
	var events []string
	record := func(event string) {
		mu.Lock()
		defer mu.Unlock()
		events = append(events, event)
	}
	newModule := func(name string) *testModule {
		return &testModule{
			onInit: func() {
				record(name + ":init")
			},
			onRun: func(closeSig chan bool) {
				<-closeSig
				record(name + ":run-stopped")
			},
			onDestroy: func() {
				record(name + ":destroy")
			},
		}
	}

	runtime := module.NewRuntime(newModule("first"), newModule("second"))
	runtime.Start()
	runtime.Stop()

	want := []string{
		"first:init",
		"second:init",
		"second:run-stopped",
		"second:destroy",
		"first:run-stopped",
		"first:destroy",
	}
	if len(events) != len(want) {
		t.Fatalf("events = %v, want %v", events, want)
	}
	for i := range want {
		if events[i] != want[i] {
			t.Fatalf("events = %v, want %v", events, want)
		}
	}
}

func TestRuntimeInstancesStopIndependently(t *testing.T) {
	firstStopped := make(chan struct{})
	secondStopped := make(chan struct{})

	first := module.NewRuntime(&testModule{onDestroy: func() { close(firstStopped) }})
	second := module.NewRuntime(&testModule{onDestroy: func() { close(secondStopped) }})
	first.Start()
	second.Start()

	first.Stop()
	select {
	case <-firstStopped:
	default:
		t.Fatal("first runtime did not stop its module")
	}
	select {
	case <-secondStopped:
		t.Fatal("stopping first runtime stopped the second runtime")
	default:
	}

	second.Stop()
	select {
	case <-secondStopped:
	default:
		t.Fatal("second runtime did not stop its module")
	}
}
