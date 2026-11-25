package route_manage

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"regexp"
	"strings"
	"sync"
)

var mtx sync.RWMutex

var RouteMap map[string]PortRouteTable

type Proxy struct {
	Location          string
	ProxyPass         string
	LuaScriptResponse string
	LuaScriptRequest  string
	Regex             bool
	UseSystemProxy    *bool `json:"UseSystemProxy,omitempty"`
	regex             *regexp.Regexp
}

// UsesSystemProxy reports whether the route should use the system proxy.
// Routes created before this option existed default to true.
func (p Proxy) UsesSystemProxy() bool {
	return p.UseSystemProxy == nil || *p.UseSystemProxy
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
	Regex             bool   `json:"regex"`
	UseSystemProxy    *bool  `json:"use_system_proxy"`
}

type proxyMapping struct {
	Location          string `json:"location"`
	ProxyPass         string `json:"proxy_pass"`
	LuaScriptResponse string `json:"lua_script_response"`
	LuaScriptRequest  string `json:"lua_script_request"`
	Regex             bool   `json:"regex"`
	UseSystemProxy    bool   `json:"use_system_proxy"`
}

type routeMappings struct {
	Port     string         `json:"port"`
	FilePath string         `json:"file_path"`
	Mappings []proxyMapping `json:"mappings"`
}

func addRoute(route_mapping routeMapping) error {
	if route_mapping.Location == "" {
		return fmt.Errorf("location is required")
	}
	if !route_mapping.Regex && !strings.HasPrefix(route_mapping.Location, "/") {
		return fmt.Errorf("location must start with /")
	}

	var compiled *regexp.Regexp
	if route_mapping.Regex {
		re, err := regexp.Compile(route_mapping.Location)
		if err != nil {
			return fmt.Errorf("invalid regex: %v", err)
		}
		compiled = re
	}

	mtx.Lock()
	defer mtx.Unlock()

	if _, ok := RouteMap[route_mapping.Port]; !ok {
		return fmt.Errorf("port %s not found", route_mapping.Port)
	}

	var p Proxy
	p.Location = route_mapping.Location
	p.ProxyPass = route_mapping.ProxyPass
	p.LuaScriptResponse = route_mapping.LuaScriptResponse
	p.LuaScriptRequest = route_mapping.LuaScriptRequest
	p.Regex = route_mapping.Regex
	p.UseSystemProxy = route_mapping.UseSystemProxy
	p.regex = compiled
	RouteMap[route_mapping.Port].PortProxy[route_mapping.Location] = p
	log.Printf("Add route!\n")
	return nil
}

func dropRoute(route_mapping routeMapping) {
	mtx.Lock()
	defer mtx.Unlock()

	if table, ok := RouteMap[route_mapping.Port]; ok {
		delete(table.PortProxy, route_mapping.Location)
	}
	log.Printf("Drop route!\n")
}

func AddRoute(w http.ResponseWriter, r *http.Request) {
	var a routeMapping
	requestBody, _ := io.ReadAll(r.Body)
	err := json.Unmarshal(requestBody, &a)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if err := addRoute(a); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	w.Write([]byte("Add route success!"))

	SaveRouteMap()
}

func DropRoute(w http.ResponseWriter, r *http.Request) {
	var a routeMapping
	requestBody, _ := io.ReadAll(r.Body)
	err := json.Unmarshal(requestBody, &a)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	dropRoute(a)
	w.Write([]byte("Drop route success!"))

	SaveRouteMap()
}

func QueryRoute(w http.ResponseWriter, r *http.Request) {
	mtx.RLock()
	results := make([]routeMappings, 0, len(RouteMap))
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
			proxyNode.Regex = v.Regex
			proxyNode.UseSystemProxy = v.UsesSystemProxy()
			result.Mappings = append(result.Mappings, proxyNode)
		}
		results = append(results, result)
	}
	mtx.RUnlock()

	resultJson, _ := json.Marshal(results)

	w.Header().Set("Content-Type", "application/json")
	w.Write(resultJson)

	SaveRouteMap()
}

// MatchRoute returns the best matching route for the given path.
// Exact matches take precedence. Regex routes are evaluated in
// descending order of pattern length, so more specific patterns win.
func MatchRoute(port string, requestPath string) (Proxy, bool) {
	mtx.RLock()
	defer mtx.RUnlock()

	table, ok := RouteMap[port]
	if !ok {
		return Proxy{}, false
	}

	if proxy, ok := table.PortProxy[requestPath]; ok && proxy.Location != "" {
		return proxy, true
	}

	var matched Proxy
	found := false
	for _, proxy := range table.PortProxy {
		if !proxy.Regex || proxy.regex == nil {
			continue
		}
		if !proxy.regex.MatchString(requestPath) {
			continue
		}
		if !found ||
			len(proxy.Location) > len(matched.Location) ||
			(len(proxy.Location) == len(matched.Location) && proxy.Location < matched.Location) {
			matched = proxy
			found = true
		}
	}
	return matched, found
}

func SaveRouteMap() {
	mtx.RLock()
	routeMapJson, _ := json.Marshal(RouteMap)
	mtx.RUnlock()

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

	mtx.Lock()
	defer mtx.Unlock()

	for port, table := range RouteMap {
		for location, proxy := range table.PortProxy {
			proxy.regex = nil
			if proxy.Regex {
				compiled, err := regexp.Compile(proxy.Location)
				if err != nil {
					log.Printf("Invalid regex for port %s location %s: %v\n", port, location, err)
					proxy.Regex = false
				} else {
					proxy.regex = compiled
				}
			}
			table.PortProxy[location] = proxy
		}
	}
}
