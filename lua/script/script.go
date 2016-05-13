package script

import (
	"fmt"
	"github.com/gobwas/gws/lua/mod"
	"github.com/yuin/gopher-lua"
	"golang.org/x/net/context"
	"io"
)

const (
	functionMain      = "main"
	functionDone      = "done"
	functionSetup     = "setup"
	functionTeardown  = "teardown"
	functionTick      = "tick"
	functionReconnect = "reconnect"
)

type Script struct {
	luaState *lua.LState
}

func New() *Script {
	return &Script{
		luaState: lua.NewState(),
	}
}

func (s *Script) Preload(name string, m mod.Module) {
	s.luaState.PreloadModule(name, m.Exports())
}

func (s *Script) Do(code string) error {
	return s.luaState.DoString(code)
}

func (s *Script) Shutdown() {
	s.luaState.Close()
}

func (s *Script) HijackOutput(w io.Writer) {
	s.luaState.SetGlobal("print", s.luaState.NewFunction(func(L *lua.LState) int {
		var buf []byte
		for i := 1; ; i++ {
			def := L.Get(i)
			if _, ok := def.(*lua.LNilType); ok {
				break
			}
			buf = append(buf, []byte(def.String())...)
		}
		w.Write(append(buf, '\n'))
		return 0
	}))
}

func (s *Script) CallMain(ctx context.Context) error {
	return callAsync(ctx, functionMain, func() error {
		return s.luaState.CallByParam(lua.P{
			Fn:      s.luaState.GetGlobal(functionMain),
			NRet:    0,
			Protect: true,
		})
	})
}

func (s *Script) CallDone(ctx context.Context) error {
	return callAsync(ctx, functionDone, func() error {
		return s.luaState.CallByParam(lua.P{
			Fn:      s.luaState.GetGlobal(functionDone),
			NRet:    0,
			Protect: true,
		})
	})
}

func (s *Script) CallSetup(ctx context.Context, thread *lua.LTable, index int) error {
	return callAsync(ctx, functionSetup, func() error {
		return s.luaState.CallByParam(lua.P{
			Fn:      s.luaState.GetGlobal(functionSetup),
			NRet:    0,
			Protect: true,
		}, thread, lua.LNumber(index))
	})
}

func (s *Script) CallTeardown(ctx context.Context, thread *lua.LTable) error {
	return callAsync(ctx, functionTeardown, func() error {
		return s.luaState.CallByParam(lua.P{
			Fn:      s.luaState.GetGlobal(functionTeardown),
			NRet:    0,
			Protect: true,
		}, thread)
	})
}

func (s *Script) CallTick(ctx context.Context, thread *lua.LTable) error {
	return callAsync(ctx, functionTick, func() error {
		return s.luaState.CallByParam(lua.P{
			Fn:      s.luaState.GetGlobal(functionTick),
			NRet:    0,
			Protect: true,
		}, thread)
	})
}

func (s *Script) CallReconnect(ctx context.Context, thread *lua.LTable) (bool, error) {
	return callAsyncBool(ctx, functionReconnect, func() (ok bool, err error) {
		err = s.luaState.CallByParam(lua.P{
			Fn:      s.luaState.GetGlobal(functionReconnect),
			NRet:    1,
			Protect: true,
		}, thread)
		if err != nil {
			return
		}
		ok = s.luaState.ToBool(-1)
		s.luaState.Pop(1)
		return
	})
}

func (s *Script) Close() {
	s.luaState.Close()
}

func callAsync(ctx context.Context, label string, actor func() error) (err error) {
	result := make(chan error)
	go func() {
		err := actor()
		select {
		case <-ctx.Done():
		case result <- err:
		}
	}()

	select {
	case <-ctx.Done():
		err = ctx.Err()
	case err = <-result:
		//
	}

	if err != nil {
		err = fmt.Errorf("call lua %q function error: %s", label, err)
	}

	return
}

type asyncResult struct {
	ok  bool
	err error
}

func callAsyncBool(ctx context.Context, label string, actor func() (bool, error)) (ok bool, err error) {
	result := make(chan asyncResult)
	go func() {
		ok, err := actor()
		select {
		case <-ctx.Done():
		case result <- asyncResult{ok, err}:
		}
	}()

	select {
	case <-ctx.Done():
		err = ctx.Err()
	case res := <-result:
		ok = res.ok
		err = res.err
	}

	if err != nil {
		err = fmt.Errorf("call lua %q function error: %s", label, err)
	}

	return
}
