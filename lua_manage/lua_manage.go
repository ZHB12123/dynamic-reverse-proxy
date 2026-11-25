package lua_manage

import (
	"fmt"

	lua "github.com/yuin/gopher-lua"
)

var LuaVMs map[string]*lua.LState

func AddLuaVM(port string) {
	if _, ok := LuaVMs[port]; !ok {
		LuaVMs[port] = lua.NewState()
	}
}

func DropLuaVM(port string) {
	if _, ok := LuaVMs[port]; ok {
		LuaVMs[port].Close()
		delete(LuaVMs, port)
	}
}

// ExecuteLua runs script with the current headers and body injected as globals.
// On failure it returns the original headers and body together with the error,
// so the caller can log the failure and continue without applying changes.
func ExecuteLua(luaVM *lua.LState, script string, headers map[string]string, body string) (map[string]string, string, error) {
	if luaVM == nil {
		return headers, body, fmt.Errorf("lua VM is not available")
	}

	headersTable := luaVM.NewTable()
	for key, value := range headers {
		headersTable.RawSetString(key, lua.LString(value))
	}
	luaVM.SetGlobal("headers", headersTable)
	luaVM.SetGlobal("body", lua.LString(body))

	if err := luaVM.DoString(script); err != nil {
		return headers, body, err
	}

	newHeaders := make(map[string]string)
	headersValue := luaVM.GetGlobal("headers")
	headersRet, ok := headersValue.(*lua.LTable)
	if !ok {
		return headers, body, fmt.Errorf("global 'headers' is %s, expected table", headersValue.Type())
	}
	headersRet.ForEach(func(k lua.LValue, v lua.LValue) {
		newHeaders[k.String()] = v.String()
	})
	newBody := luaVM.GetGlobal("body")
	return newHeaders, newBody.String(), nil
}
