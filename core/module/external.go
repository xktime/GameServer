package module

type External interface {
	InitExternal()
	GetModule() Module
}
