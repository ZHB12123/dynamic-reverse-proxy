# dynamic-reverse-proxy

一个支持**动态修改转发规则**的反向代理。运行时通过 HTTP 接口增删端口与路由，无需重启进程；每条路由还可以挂载 Lua 脚本，在请求转发前和响应返回前对 Header 与 Body 进行改写。

A reverse proxy whose rules can be changed at runtime, with Lua scripting support for request/response rewriting.

## 特性

- 动态管理：通过 HTTP API 实时添加/删除监听端口和转发规则，配置自动持久化
- 内置管理台：浏览器打开管理端口即可完成端口与路由的增删改查
- 多端口监听：一个进程可同时代理多个端口，每个端口可做反向代理，并可选开启静态文件目录分享
- 系统代理开关：每条路由可单独选择是否走系统代理，默认开启
- 精确与正则路径匹配：精确匹配优先，正则使用 Go RE2 语法
- Lua 脚本注入：支持请求脚本与响应脚本，可修改 Header 和 Body
- 配置持久化：规则保存在 `.route_map`（JSON），进程重启后自动加载

## 工作原理

```
                 ┌──────────────────────────────┐
  请求 ─────────▶│  业务端口 (如 :8080)          │
                 │                              │
                 │  路由表命中?                 │
                 │    ├─ 是 → Lua 请求脚本      │
                 │    │        → 反向代理上游   │
                 │    │        → Lua 响应脚本   │
                 │    └─ 否 → 静态文件 / 404    │
                 └──────────────────────────────┘

  管理端 ───────▶ 管理端口 (默认 :9998)
                 /add_port  /drop_port
                 /add_route /drop_route /query_route
```

- 每个端口对应一张路由表 `PortRouteTable`，其中 `PortProxy` 以请求路径为 key 保存代理规则
- 请求进入业务端口后，若能在路由表中找到精确匹配或正则匹配的路径，则转发到 `proxy_pass` 指定的上游；否则由该端口配置的静态目录提供文件服务，未开启静态目录分享时返回 404
- 每个端口拥有独立的 Lua 虚拟机（`*lua.LState`），用于执行该端口下路由绑定的脚本

## 目录结构

```
.
├── main.go                       # 程序入口、端口服务器、管理端口
├── .route_map                    # 路由规则持久化文件（JSON，自动生成）
├── route_manage/route_manage.go  # 路由表结构、增删查、持久化
├── lua_manage/lua_manage.go      # Lua 虚拟机管理与脚本执行
├── web/                          # 管理端前端页面（编译时内嵌）
│   ├── index.html
│   ├── style.css
│   └── app.js
├── go.mod
└── go.sum
```

## 快速开始

### 环境要求

- Go 1.17+

### 编译与运行

```bash
go build -o dynamic-reverse-proxy .
./dynamic-reverse-proxy
```

使用默认配置启动（管理端 `127.0.0.1:9998`）。如需修改监听地址：

```bash
./dynamic-reverse-proxy -host 0.0.0.0 -port 9997
```

或直接运行：

```bash
go run .
```

程序启动后会：

1. 从 `.route_map` 加载已有路由（若存在）
2. 为加载到的每个端口启动监听服务
3. 启动管理 HTTP 服务（默认 `127.0.0.1:9998`，可通过 `-host`、`-port` 修改）

## 命令行参数

| 参数 | 说明 | 默认值 |
| --- | --- | --- |
| `-host` | 管理接口监听地址 | `127.0.0.1` |
| `-port` | 管理接口监听端口 | `9998` |

同时支持 `--host`、`--port` 写法；未传入时使用上表默认值。查看帮助：

```bash
./dynamic-reverse-proxy -h
```

## 管理台

管理端内置了一个单页管理界面，无需额外部署。启动后访问管理端地址即可，例如：

```
http://127.0.0.1:9998/
```

（端口以 `-port` 参数为准，默认 `9998`。）

页面支持：

- 添加 / 删除监听端口，可选择是否分享静态文件目录
- 点击端口卡片展开二级面板，在面板内查看和删除该端口的转发路由（支持精确/正则匹配与系统代理开关）
- 点击“添加路由”或“修改”时弹出路由表单，修改时自动回填已有配置
- 为路由填写请求与响应 Lua 脚本
- 查看当前所有端口、路由及脚本启用状态
- 操作结果提示与配置刷新

转发路由按端口归属：添加端口后，点击对应端口卡片展开路由列表；添加和修改路由在弹窗中完成。前端资源位于 `web/`，通过 `go:embed` 在编译时内嵌进二进制；页面通过同源的 `/add_port`、`/drop_port`、`/add_route`、`/drop_route`、`/query_route` 接口工作。修改 `web/` 下的文件后需要重新编译才能生效。

## 管理 API

除管理台页面外，API 接口均为 `POST`，请求体为 JSON，返回值是纯文本提示（`/query_route` 返回 JSON）。

### 添加端口 `/add_port`

为一个端口创建监听服务。命中路由时执行反向代理；`filePath` 为可选项，填写后将该目录作为静态文件服务分享，留空则表示不分享静态目录，未命中路由的请求会直接返回 404。同一端口重复添加会被忽略。

```bash
curl -X POST http://127.0.0.1:9998/add_port \
  -H 'Content-Type: application/json' \
  -d '{"port":"8080","filePath":"./public"}'
```

| 字段 | 说明 |
| --- | --- |
| `port` | 监听端口，如 `"8080"` |
| `filePath` | 可选。静态文件根目录；为空字符串表示不分享静态目录 |

### 删除端口 `/drop_port`

停止监听并删除该端口下的所有路由与 Lua 虚拟机。

```bash
curl -X POST http://127.0.0.1:9998/drop_port \
  -H 'Content-Type: application/json' \
  -d '{"port":"8080"}'
```

### 添加路由 `/add_route`

为已存在的端口添加一条转发规则。默认 `location` 与请求路径**完全匹配**（区分大小写，不匹配前缀）；当 `regex` 为 `true` 时，`location` 将作为 Go RE2 正则表达式匹配请求路径。

```bash
curl -X POST http://127.0.0.1:9998/add_route \
  -H 'Content-Type: application/json' \
  -d '{
    "port": "8080",
    "location": "/api",
    "proxy_pass": "http://127.0.0.1:9000",
    "regex": false,
    "use_system_proxy": true,
    "lua_script_request": "",
    "lua_script_response": ""
  }'
```

| 字段 | 说明 |
| --- | --- |
| `port` | 所属端口，需先通过 `/add_port` 创建 |
| `location` | 匹配的请求路径；`regex` 为 `true` 时为正则表达式 |
| `proxy_pass` | 上游地址，如 `http://127.0.0.1:9000`。转发时目标路径取该 URL 的 Path |
| `regex` | 可选，是否使用正则匹配，默认 `false` |
| `use_system_proxy` | 可选，是否使用系统代理，默认 `true`；设为 `false` 时直连上游 |
| `lua_script_request` | 请求转发前执行的 Lua 脚本（可为空） |
| `lua_script_response` | 响应返回前执行的 Lua 脚本（可为空） |

> 匹配优先级：先精确匹配；未命中时再按正则匹配，多条正则命中时选择模式最长的一条（长度相同按字典序）。正则默认不加锚点，例如 `^/api/.*$` 可匹配 `/api/users`。
>
> 正则仅用于匹配请求路径，捕获组不会用于改写上游地址；上游路径仍取自 `proxy_pass`。
>
> 正则为 Go RE2 语法；无效正则或端口不存在时返回 HTTP 400。

### 删除路由 `/drop_route`

```bash
curl -X POST http://127.0.0.1:9998/drop_route \
  -H 'Content-Type: application/json' \
  -d '{"port":"8080","location":"/api"}'
```

### 查询路由 `/query_route`

返回当前所有端口及其路由配置的 JSON 数组。

```bash
curl -X POST http://127.0.0.1:9998/query_route
```

响应示例：

```json
[
  {
    "port": "8080",
    "file_path": "./public",
    "mappings": [
      {
        "location": "/api",
        "proxy_pass": "http://127.0.0.1:9000",
        "regex": false,
        "use_system_proxy": true,
        "lua_script_response": "",
        "lua_script_request": ""
      }
    ]
  }
]
```

## Lua 脚本

脚本在对应路由命中时执行，可用的全局变量为：

| 全局变量 | 类型 | 说明 |
| --- | --- | --- |
| `headers` | table | Header 键值对。取值时使用每个 Header 的第一个值 |
| `body` | string | 消息体。脚本执行后读取该全局变量作为新的 Body |

约定：

- 脚本开始执行前会注入 `headers` 与 `body` 全局变量
- 脚本对 `headers` 表的增删改会写回请求/响应
- 脚本通过给 `body` 赋值来修改消息体，可基于原始 `body` 进行替换
- 脚本执行后，请求脚本会同步更新 `Content-Length`；响应脚本同样会重设 `Content-Length`

### 请求脚本示例：添加认证头

```lua
headers["X-From-Proxy"] = "dynamic-reverse-proxy"
headers["Authorization"] = "Bearer my-token"
```

### 响应脚本示例：修改 Body 与 Header

```lua
headers["X-Powered-By"] = "dynamic-reverse-proxy"
body = string.gsub(body, "foo", "bar")
```

> Lua 虚拟机按端口复用，请避免在脚本中依赖持久化的全局状态。

## 系统代理

每条转发路由默认使用系统代理，由 Go 标准库 `http.ProxyFromEnvironment` 提供：

- 读取 `HTTP_PROXY`、`HTTPS_PROXY`、`NO_PROXY` 环境变量
- `localhost` 与回环地址默认绕过代理
- 关闭某条路由的“使用系统代理”后，该路由通过独立的直连 Transport 访问上游，不经过代理

> 代理环境变量在进程启动后只读取一次，修改后需重启服务。

## 持久化

- 所有增删操作以及 `/query_route` 调用都会把当前路由表写入 `.route_map`
- 文件内容为 `map[port]PortRouteTable` 的 JSON 序列化结果
- 启动时自动读取该文件恢复路由；文件不存在时从空配置开始

示例 `.route_map`（压缩后的 JSON）：

```json
{"8080":{"Port":"8080","FilePath":"./public","PortProxy":{"/api":{"Location":"/api","ProxyPass":"http://127.0.0.1:9000","LuaScriptResponse":"","LuaScriptRequest":"","Regex":false,"UseSystemProxy":true},"^/api/.*$":{"Location":"^/api/.*$","ProxyPass":"http://127.0.0.1:9001","LuaScriptResponse":"","LuaScriptRequest":"","Regex":true,"UseSystemProxy":false}}}}
```

## 注意事项

- 端口需先添加再配置路由；为不存在的端口添加路由不会生效
- 未分享静态目录（`filePath` 为空）且请求未命中路由时返回 404
- 端口监听失败（如端口被占用）不会中断进程，也不会返回错误，请自行确认端口可用
- 路由先精确匹配，再按正则匹配；正则按模式长度优先，查询字符串不参与匹配但会随请求转发到上游
- 上游路径取自 `proxy_pass` 的 Path，原始请求路径不会拼接保留
- 系统代理基于 `HTTP_PROXY`/`HTTPS_PROXY`/`NO_PROXY` 环境变量，回环地址默认绕过
- 管理接口无鉴权，请勿直接暴露在公网
- 每个端口复用一个 Lua 虚拟机，脚本并发执行时需注意状态隔离

## 依赖

- [yuin/gopher-lua](https://github.com/yuin/gopher-lua) — Lua 脚本执行
- Go 标准库 `net/http/httputil` — 反向代理
