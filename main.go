package main

import (
	"bytes"
	"embed"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"log"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strconv"
	"sync"
	"time"

	lua "github.com/yuin/gopher-lua"

	"reverse_proxy/lua_manage"
	"reverse_proxy/route_manage"
)

//go:embed web/*
var webAssets embed.FS

var directTransport = func() *http.Transport {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.Proxy = nil
	return transport
}()

var mtx sync.Mutex

var manageHost = flag.String("host", "127.0.0.1", "management API listen host")
var managePort = flag.Int("port", 9998, "management API listen port")

var portServers map[string]PortServer

type PortServer struct {
	Port        string
	FilePath    string
	Server      *http.Server
	FileHandler http.Handler
}

func (port_server *PortServer) proxyNew(w http.ResponseWriter, r *http.Request) {
	rec, r := newRequestLog(r, port_server.Port)
	lrw := &loggingResponseWriter{ResponseWriter: w}

	log.Printf("%s %s %s host=%q client=%s", rec.prefix(), r.Method, r.URL.RequestURI(), r.Host, r.RemoteAddr)
	defer func() {
		status := lrw.status
		if status == 0 {
			status = http.StatusOK
		}
		log.Printf("%s status=%d bytes=%d duration=%s", rec.prefix(), status, lrw.bytes, rec.duration().Round(time.Microsecond))
	}()

	passProxy, matched := route_manage.MatchRoute(port_server.Port, r.URL.Path)
	port := port_server.Port

	proxy := new(httputil.ReverseProxy)
	if !passProxy.UsesSystemProxy() {
		proxy.Transport = directTransport
	}
	proxy.Director = func(req *http.Request) {
		if passProxy.Location != "" {
			u, err := url.Parse(passProxy.ProxyPass)
			if err != nil {
				log.Printf("%s invalid proxy_pass=%q: %v", rec.prefix(), passProxy.ProxyPass, err)
				return
			}

			req.URL.Path = passProxy.UpstreamPath(req.URL.Path)
			req.URL.RawPath = ""
			req.URL.Scheme = u.Scheme
			req.URL.Host = u.Host
			req.Host = u.Host

			log.Printf("%s forward %s %s system_proxy=%t", rec.prefix(), req.Method, req.URL.String(), passProxy.UsesSystemProxy())

			if passProxy.LuaScriptRequest != "" {
				var body []byte
				if req.Body != nil {
					var readErr error
					body, readErr = io.ReadAll(req.Body)
					if readErr != nil {
						log.Printf("%s read request body failed: %v", rec.prefix(), readErr)
					}
				}

				headers := make(map[string]string)
				for key, value := range req.Header {
					if len(value) > 0 {
						headers[key] = value[0]
					}
				}

				new_header, new_body, luaErr := lua_manage.ExecuteLua(lua_manage.LuaVMs[port], passProxy.LuaScriptRequest, headers, string(body))
				if luaErr != nil {
					log.Printf("%s request lua script failed location=%q: %v", rec.prefix(), passProxy.Location, luaErr)
					req.Body = io.NopCloser(bytes.NewReader(body))
					req.ContentLength = int64(len(body))
					return
				}

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
		rec := requestLogFrom(resp.Request)

		body, readErr := io.ReadAll(resp.Body)
		if readErr != nil {
			log.Printf("%s read response body failed: %v", rec.prefix(), readErr)
			return nil
		}

		headers := make(map[string]string)
		for key, value := range resp.Header {
			if len(value) > 0 {
				headers[key] = value[0]
			}
		}
		new_header, new_body, luaErr := lua_manage.ExecuteLua(lua_manage.LuaVMs[port], passProxy.LuaScriptResponse, headers, string(body))
		if luaErr != nil {
			log.Printf("%s response lua script failed location=%q: %v", rec.prefix(), passProxy.Location, luaErr)
			resp.Body = io.NopCloser(bytes.NewReader(body))
			return nil
		}

		buf := bytes.NewBufferString(new_body)
		resp.Body = io.NopCloser(buf)

		for key, value := range new_header {
			resp.Header.Set(key, value)
		}

		resp.ContentLength = int64(buf.Len())
		resp.Header.Set("Content-Length", fmt.Sprint(buf.Len()))
		return nil
	}
	proxy.ErrorHandler = func(w http.ResponseWriter, req *http.Request, err error) {
		log.Printf("%s upstream request failed proxy_pass=%q url=%q: %v", rec.prefix(), passProxy.ProxyPass, req.URL.String(), err)
		http.Error(w, "Bad Gateway", http.StatusBadGateway)
	}

	if matched {
		log.Printf("%s matched location=%q regex=%t proxy_pass=%q system_proxy=%t lua_request=%t lua_response=%t",
			rec.prefix(), passProxy.Location, passProxy.Regex, passProxy.ProxyPass, passProxy.UsesSystemProxy(),
			passProxy.LuaScriptRequest != "", passProxy.LuaScriptResponse != "")
		proxy.ServeHTTP(lrw, r)
	} else if fileHandler := portServers[port_server.Port].FileHandler; fileHandler != nil {
		log.Printf("%s no route matched, serving static file_path=%q", rec.prefix(), portServers[port_server.Port].FilePath)
		fileHandler.ServeHTTP(lrw, r)
	} else {
		log.Printf("%s no route matched and static sharing disabled, returning 404", rec.prefix())
		http.NotFound(lrw, r)
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
		log.Printf("Add port :%s success, file_path=%q static=%t", port_info.Port, port_info.FilePath, port_info.FilePath != "")
		return
	}
	log.Printf("Add port :%s ignored, port already exists", port_info.Port)
}

func dropPort(port_info route_manage.PortInfo) {
	mtx.Lock()
	defer mtx.Unlock()

	routeCount := 0
	if table, ok := route_manage.RouteMap[port_info.Port]; ok {
		routeCount = len(table.PortProxy)
	}

	serverStopped := false
	if _, ok := portServers[port_info.Port]; ok {
		if err := portServers[port_info.Port].Server.Close(); err != nil {
			log.Printf("Drop port :%s close server failed: %v", port_info.Port, err)
		} else {
			serverStopped = true
		}
		delete(portServers, port_info.Port)
	}
	lua_manage.DropLuaVM(port_info.Port)
	delete(route_manage.RouteMap, port_info.Port)
	log.Printf("Drop port :%s success, server_stopped=%t routes_removed=%d", port_info.Port, serverStopped, routeCount)
}

func AddPort(w http.ResponseWriter, r *http.Request) {
	var a route_manage.PortInfo
	requestBody, _ := io.ReadAll(r.Body)
	err := json.Unmarshal(requestBody, &a)
	if err != nil {
		log.Printf("API add_port invalid request from %s: %v", r.RemoteAddr, err)
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
		log.Printf("API drop_port invalid request from %s: %v", r.RemoteAddr, err)
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
		if file_path != "" {
			s.FileHandler = http.FileServer(http.Dir(file_path))
		}

		serverMux.HandleFunc("/", s.proxyNew)

		portServers[port] = s
		log.Printf("Port :%s listening, file_path=%q static=%t", port, file_path, file_path != "")
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Printf("Port :%s server stopped unexpectedly: %v", port, err)
		}
	}()
}

func main() {
	log.SetFlags(log.LstdFlags | log.Lmicroseconds)

	flag.Parse()
	if *managePort < 1 || *managePort > 65535 {
		log.Fatalf("Invalid management port: %d\n", *managePort)
	}

	route_manage.RouteMap = make(map[string]route_manage.PortRouteTable)
	lua_manage.LuaVMs = make(map[string]*lua.LState)

	portServers = make(map[string]PortServer)

	route_manage.LoadRouteMapCache()

	totalRoutes := 0
	for _, p := range route_manage.RouteMap {
		totalRoutes += len(p.PortProxy)
		lua_manage.AddLuaVM(p.Port)
		AddPortServer(p.Port, p.FilePath)
	}
	log.Printf("Startup complete, ports=%d routes=%d", len(route_manage.RouteMap), totalRoutes)

	go func() {
		serveMux := http.NewServeMux()

		serveMux.HandleFunc("/add_route", route_manage.AddRoute)
		serveMux.HandleFunc("/drop_route", route_manage.DropRoute)
		serveMux.HandleFunc("/query_route", route_manage.QueryRoute)
		serveMux.HandleFunc("/add_port", AddPort)
		serveMux.HandleFunc("/drop_port", DropPort)

		webFS, err := fs.Sub(webAssets, "web")
		if err != nil {
			log.Printf("Load web assets failed: %v\n", err)
		} else {
			serveMux.Handle("/", http.FileServer(http.FS(webFS)))
		}

		addr := net.JoinHostPort(*manageHost, strconv.Itoa(*managePort))
		log.Printf("Management API listening on %s\n", addr)
		if err := http.ListenAndServe(addr, serveMux); err != nil {
			log.Printf("Management API stopped: %v\n", err)
		}
	}()
	select {}
}
