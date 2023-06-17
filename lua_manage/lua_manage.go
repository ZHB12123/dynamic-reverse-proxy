package lua_manage

import (
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

func ExecuteLua(luaVM *lua.LState, script string, headers map[string]string, body string) (map[string]string, string) {
	headersTable := luaVM.NewTable()
	for key, value := range headers {
		headersTable.RawSetString(key, lua.LString(value))
	}
	luaVM.SetGlobal("headers", headersTable)

	luaVM.DoString(script)

	newHeaders := make(map[string]string)
	headersRet := luaVM.GetGlobal("headers").(*lua.LTable)
	headersRet.ForEach(func(k lua.LValue, v lua.LValue) {
		newHeaders[k.String()] = v.String()
	})
	newBody := luaVM.GetGlobal("body")
	return newHeaders, newBody.String()
}
