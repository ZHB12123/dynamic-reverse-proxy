package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"github.com/spf13/viper"
	lua "github.com/yuin/gopher-lua"
	"io"
	"log"
	"net/http"
	"net/http/httputil"
	"net/url"
	"reverse_proxy/lua_manage"
	"reverse_proxy/route_manage"
	"sync"
	"time"
)

var mtx sync.Mutex

var portServers map[string]PortServer

type PortServer struct {
	Port        string
	FilePath    string
	Server      *http.Server
	FileHandler http.Handler
}

func (port_server *PortServer) proxyNew(w http.ResponseWriter, r *http.Request) {
	requestPath := r.URL.Path
	passProxy := route_manage.RouteMap[port_server.Port].PortProxy[requestPath]
	port := port_server.Port

	proxy := new(httputil.ReverseProxy)
	proxy.Director = func(req *http.Request) {
		if passProxy.Location != "" {
			u, _ := url.Parse(passProxy.ProxyPass)
			req.URL.Path = u.Path
			req.URL.Scheme = u.Scheme
			req.URL.Host = u.Host
			req.Host = u.Host

			if passProxy.LuaScriptRequest != "" {
				body, _ := io.ReadAll(req.Body)

				headers := make(map[string]string)
				for key, value := range req.Header {
					headers[key] = value[0]
				}

				new_header, new_body := lua_manage.ExecuteLua(lua_manage.LuaVMs[port], passProxy.LuaScriptRequest, headers, string(body))

				buf := bytes.NewBufferString(new_body)
				req.Body = io.NopCloser(buf)

				for key, value := range new_header {
					req.Header.Set(key, value)
				}

				req.ContentLength = int64(buf.Len())
			}
		}
	}
	proxy.ModifyResponse = func(resp *http.Response) error {
		if passProxy.LuaScriptResponse == "" {
			return nil
		}
		body, _ := io.ReadAll(resp.Body)

		headers := make(map[string]string)
		for key, value := range resp.Header {
			headers[key] = value[0]
		}
		new_header, new_body := lua_manage.ExecuteLua(lua_manage.LuaVMs[port], passProxy.LuaScriptResponse, headers, string(body))

		buf := bytes.NewBufferString(new_body)
		resp.Body = io.NopCloser(buf)

		for key, value := range new_header {
			resp.Header.Set(key, value)
		}

		resp.Header.Set("Content-Length", fmt.Sprint(buf.Len()))
		return nil
	}
	proxy.ErrorHandler = func(w http.ResponseWriter, r *http.Request, err error) {
		if err != nil {
			fmt.Println(err.Error())
		}
	}

	if route_manage.RouteMap[port_server.Port].PortProxy[r.URL.Path].Location != "" {
		proxy.ServeHTTP(w, r)
	} else {
		portServers[port_server.Port].FileHandler.ServeHTTP(w, r)
	}
}

func addPort(port_info route_manage.PortInfo) {
	mtx.Lock()
	defer mtx.Unlock()
	if _, ok := route_manage.RouteMap[port_info.Port]; !ok {
		var portRoute route_manage.PortRouteTable
		portRoute.Port = port_info.Port
		portRoute.FilePath = port_info.FilePath
		portRoute.PortProxy = make(map[string]route_manage.Proxy)

		route_manage.RouteMap[port_info.Port] = portRoute
		lua_manage.AddLuaVM(port_info.Port)
		AddPortServer(port_info.Port, port_info.FilePath)
		log.Printf("Add port %v success!\n", port_info.Port)
	}
}

func dropPort(port_info route_manage.PortInfo) {
	mtx.Lock()
	defer mtx.Unlock()

	if _, ok := portServers[port_info.Port]; ok {
		portServers[port_info.Port].Server.Close()
		delete(portServers, port_info.Port)
	}
	lua_manage.DropLuaVM(port_info.Port)
	delete(route_manage.RouteMap, port_info.Port)
	log.Printf("Drop port %v success!\n", port_info.Port)
}

func AddPort(w http.ResponseWriter, r *http.Request) {
	var a route_manage.PortInfo
	requestBody, _ := io.ReadAll(r.Body)
	err := json.Unmarshal(requestBody, &a)
	if err != nil {
		w.Write([]byte(err.Error()))
		return
	}
	addPort(a)
	w.Write([]byte("Add port success!"))

	route_manage.SaveRouteMap()
}

func DropPort(w http.ResponseWriter, r *http.Request) {
	var a route_manage.PortInfo
	requestBody, _ := io.ReadAll(r.Body)
	err := json.Unmarshal(requestBody, &a)
	if err != nil {
		w.Write([]byte(err.Error()))
		return
	}
	dropPort(a)
	w.Write([]byte("Drop port success!"))

	route_manage.SaveRouteMap()
}

func AddPortServer(port string, file_path string) {
	go func() {
		serverMux := http.NewServeMux()

		server := &http.Server{
			Addr:         ":" + port,
			WriteTimeout: time.Second * 60,
			Handler:      serverMux,
		}

		var s PortServer
		s.Server = server
		s.FilePath = file_path
		s.Port = port
		s.FileHandler = http.FileServer(http.Dir(file_path))

		serverMux.HandleFunc("/", s.proxyNew)

		portServers[port] = s
		server.ListenAndServe()
	}()
}

func main() {
	route_manage.RouteMap = make(map[string]route_manage.PortRouteTable)
	lua_manage.LuaVMs = make(map[string]*lua.LState)

	portServers = make(map[string]PortServer)

	route_manage.LoadRouteMapCache()
	for _, p := range route_manage.RouteMap {
		AddPortServer(p.Port, p.FilePath)
	}
	go func() {
		var host string
		var port string

		config := viper.New()
		config.AddConfigPath(".")
		config.SetConfigName("config")
		config.SetConfigType("ini")
		if err := config.ReadInConfig(); err != nil {
			host = "127.0.0.1"
			port = "9998"
		} else {
			host = config.GetString("manage.host")
			port = config.GetString("manage.port")
		}

		serveMux := http.NewServeMux()

		serveMux.HandleFunc("/add_route", route_manage.AddRoute)
		serveMux.HandleFunc("/drop_route", route_manage.DropRoute)
		serveMux.HandleFunc("/query_route", route_manage.QueryRoute)
		serveMux.HandleFunc("/add_port", AddPort)
		serveMux.HandleFunc("/drop_port", DropPort)

		http.ListenAndServe(host+":"+port, serveMux)
	}()
	select {}
}
