package module

import (
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

func (r *Runtime) Stop() {
	for i := len(r.modules) - 1; i >= 0; i-- {
		m := r.modules[i]
		m.closeSig <- true
		m.wg.Wait()
		destroy(m)
	}
}

func run(m *module) {
	m.mi.Run(m.closeSig)
	m.wg.Done()
}

func destroy(m *module) {
	defer func() {
		if r := recover(); r != nil {
			if conf.LenStackBuf > 0 {
				buf := make([]byte, conf.LenStackBuf)
				l := runtime.Stack(buf, false)
				log.Error("%v: %s", r, buf[:l])
			} else {
				log.Error("%v", r)
			}
		}
	}()

	m.mi.OnDestroy()
}
