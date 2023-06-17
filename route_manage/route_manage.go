package route_manage

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"sync"
)

var mtx sync.Mutex

var RouteMap map[string]PortRouteTable

type Proxy struct {
	Location          string
	ProxyPass         string
	LuaScriptResponse string
	LuaScriptRequest  string
}

type PortRouteTable struct {
	Port      string
	FilePath  string
	PortProxy map[string]Proxy
}

type PortInfo struct {
	Port     string `json:"port"`
	FilePath string `json:"filePath"`
}

type routeMapping struct {
	Port              string `json:"port"`
	Location          string `json:"location"`
	ProxyPass         string `json:"proxy_pass"`
	LuaScriptResponse string `json:"lua_script_response"`
	LuaScriptRequest  string `json:"lua_script_request"`
}

type proxyMapping struct {
	Location          string `json:"location"`
	ProxyPass         string `json:"proxy_pass"`
	LuaScriptResponse string `json:"lua_script_response"`
	LuaScriptRequest  string `json:"lua_script_request"`
}

type routeMappings struct {
	Port     string         `json:"port"`
	FilePath string         `json:"file_path"`
	Mappings []proxyMapping `json:"mappings"`
}

func (routeTable *PortRouteTable) addRoute(route_mapping routeMapping) {
	mtx.Lock()
	defer mtx.Unlock()
	if _, ok := RouteMap[route_mapping.Port]; ok {
		var p Proxy
		p.Location = route_mapping.Location
		p.ProxyPass = route_mapping.ProxyPass
		p.LuaScriptResponse = route_mapping.LuaScriptResponse
		p.LuaScriptRequest = route_mapping.LuaScriptRequest
		RouteMap[route_mapping.Port].PortProxy[route_mapping.Location] = p
	}
	log.Printf("Add route!\n")
}

func (routeTable *PortRouteTable) dropRoute(route_mapping routeMapping) {
	mtx.Lock()
	defer mtx.Unlock()
	fmt.Println(route_mapping)

	if _, ok := RouteMap[route_mapping.Port].PortProxy[route_mapping.Location]; ok {
		delete(RouteMap[route_mapping.Port].PortProxy, route_mapping.Location)
	}
	log.Printf("Drop route!\n")
}

func AddRoute(w http.ResponseWriter, r *http.Request) {
	var a routeMapping
	requestBody, _ := io.ReadAll(r.Body)
	err := json.Unmarshal(requestBody, &a)
	if err != nil {
		w.Write([]byte(err.Error()))
		return
	}
	table := RouteMap[a.Port]
	table.addRoute(a)
	w.Write([]byte("Add route success!"))

	SaveRouteMap()
}

func DropRoute(w http.ResponseWriter, r *http.Request) {
	var a routeMapping
	requestBody, _ := io.ReadAll(r.Body)
	err := json.Unmarshal(requestBody, &a)
	if err != nil {
		w.Write([]byte(err.Error()))
		return
	}
	table := RouteMap[a.Port]
	table.dropRoute(a)
	w.Write([]byte("Drop route success!"))

	SaveRouteMap()
}

func QueryRoute(w http.ResponseWriter, r *http.Request) {
	var results []routeMappings

	for port, route := range RouteMap {
		var result routeMappings
		result.Port = port
		result.FilePath = route.FilePath
		for _, v := range route.PortProxy {
			var proxyNode proxyMapping
			proxyNode.Location = v.Location
			proxyNode.ProxyPass = v.ProxyPass
			proxyNode.LuaScriptResponse = v.LuaScriptResponse
			proxyNode.LuaScriptRequest = v.LuaScriptRequest
			result.Mappings = append(result.Mappings, proxyNode)
		}
		results=append(results,result)
	}
	resultJson,_:=json.Marshal(results)

	w.Header().Set("Content-Type","application/json")
	w.Write(resultJson)

	SaveRouteMap()
}

func SaveRouteMap() {
	routeMapJson, _ := json.Marshal(RouteMap)
	cacheFile, _ := os.OpenFile(".route_map", os.O_CREATE|os.O_RDWR|os.O_TRUNC, 0666)
	defer cacheFile.Close()
	cacheFile.Write(routeMapJson)
}

func LoadRouteMapCache() {
	cacheFile, err := os.Open(".route_map")
	if err != nil {
		return
	}
	defer cacheFile.Close()

	cacheContent, _ := io.ReadAll(cacheFile)

	json.Unmarshal(cacheContent, &RouteMap)
}
