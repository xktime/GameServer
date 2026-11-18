package module

import (
	"errors"
	"fmt"
	"gameserver/core/conf"
	"gameserver/core/log"
	"runtime"
	"sync"
)

type Module interface {
	OnInit()
	OnDestroy()
	Run(closeSig chan bool)
}

// ShutdownModule 允许模块将关闭失败交回运行时；实现后替代 OnDestroy 调用。
type ShutdownModule interface {
	Shutdown() error
}

type module struct {
	mi       Module
	closeSig chan bool
	wg       sync.WaitGroup
}

type Runtime struct {
	modules []*module
}

func NewRuntime(modules ...Module) *Runtime {
	managedModules := make([]*module, len(modules))
	for i, mi := range modules {
		managedModules[i] = &module{
			mi:       mi,
			closeSig: make(chan bool, 1),
		}
	}
	return &Runtime{modules: managedModules}
}

func (r *Runtime) Start() {
	for _, m := range r.modules {
		m.mi.OnInit()
	}

	for _, m := range r.modules {
		m.wg.Add(1)
		go run(m)
	}
}

func (r *Runtime) Stop() error {
	var failures []error
	for i := len(r.modules) - 1; i >= 0; i-- {
		m := r.modules[i]
		m.closeSig <- true
		m.wg.Wait()
		if err := destroy(m); err != nil {
			failures = append(failures, fmt.Errorf("shutdown module %T: %w", m.mi, err))
		}
	}
	return errors.Join(failures...)
}

func run(m *module) {
	m.mi.Run(m.closeSig)
	m.wg.Done()
}

func destroy(m *module) (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("module shutdown panic: %v", r)
			if conf.LenStackBuf > 0 {
				buf := make([]byte, conf.LenStackBuf)
				l := runtime.Stack(buf, false)
				log.Error("%v: %s", r, buf[:l])
			} else {
				log.Error("%v", r)
			}
		}
	}()

	if shutdown, ok := m.mi.(ShutdownModule); ok {
		return shutdown.Shutdown()
	}
	m.mi.OnDestroy()
	return nil
}
