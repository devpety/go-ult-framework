<h1 align="center">ULT Framework</h1>

<p align="center"><strong>基于 Gin 与 Wire 的 Go HTTP 应用框架</strong></p>

<p align="center">
  以清晰分层组织业务，统一请求处理、数据访问与服务生命周期。
</p>

<p align="center">
  <a href="go.mod"><img src="https://img.shields.io/badge/Go-1.23%2B-00ADD8?style=flat-square&amp;logo=go&amp;logoColor=white" alt="Go 1.23.0 及以上"></a>
  <a href="go.mod"><img src="https://img.shields.io/badge/Gin-v1.11.0-008ECF?style=flat-square" alt="Gin v1.11.0"></a>
  <a href="go.mod"><img src="https://img.shields.io/badge/Wire-v0.7.0-008ECF?style=flat-square" alt="Wire v0.7.0"></a>
</p>

<p align="center">
  <a href="#框架概览">框架概览</a> ·
  <a href="#快速开始">快速开始</a> ·
  <a href="#开发指南">开发指南</a> ·
  <a href="#配置参考">配置参考</a> ·
  <a href="#运行与部署">运行与部署</a> ·
  <a href="#命令参考">命令参考</a>
</p>

---

## 框架概览

ULT Framework 在 Gin 之上封装 HTTP 请求处理、数据连接和应用生命周期。业务依赖通过构造函数声明，由 Wire 在编译前生成装配与清理代码。Go 模块路径为 `ult`。

应用按 `router → api → service → repo → Data` 分层，基础组件集中在 `pkg/`。仓库提供健康检查接口和 `Test` 数据访问示例；业务 API、鉴权中间件与权限规则由应用接入。

### 核心能力

| 能力 | 内容 |
| --- | --- |
| HTTP 处理 | 构建期路由注册、路由分组、中间件编排、参数绑定与校验、统一响应 |
| 错误与链路 | 业务错误码、HTTP 状态映射、多语言消息、TraceID 与分类日志 |
| 数据访问 | MySQL / GORM、类型化查询器、命名数据库与 Redis 连接、事务原语及 Redis 租约锁 |
| 依赖装配 | Wire 编译期注入、显式构造函数、初始化错误传递与资源清理链 |
| 生命周期 | 有限预算的依赖初始化、连接重试、信号停机与连接回收 |
| 配套工具 | JWT 签发与验签、健康检查、就绪探针、可选邮件通知与 pprof |

<details>
<summary>文档导航</summary>

- [快速开始](#快速开始)
- [架构与目录](#架构与目录)
- [开发指南](#开发指南)
  - [HTTP 构建与路由](#http-构建与路由)
  - [中间件](#中间件)
  - [请求绑定与响应](#请求绑定与响应)
  - [业务错误](#业务错误)
  - [请求上下文与链路标识](#请求上下文与链路标识)
  - [数据访问与生成](#数据访问与生成)
  - [JWT 工具](#jwt-工具)
  - [新增业务模块](#新增业务模块)
- [配置参考](#配置参考)
- [运行与部署](#运行与部署)
- [命令参考](#命令参考)
- [主要依赖](#主要依赖)
- [致谢](#致谢)

</details>

## 快速开始

### 环境要求

- Go 1.23.0 或更高版本。
- `make`、`sed`；使用下方密钥生成命令时需要 `openssl`。
- 按需准备 MySQL、Redis；也可使用[最小配置](#最小配置)先启动健康检查服务。

### 1. 初始化配置

从仓库根目录执行：

```bash
go mod download
make init
```

`make init` 在 `.env.yml` 不存在时复制 [.env.example.yml](.env.example.yml)，保留已有配置文件。

### 2. 配置运行参数

编辑 `.env.yml`，完成以下设置：

| 配置项 | 设置方式 |
| --- | --- |
| MySQL、Redis | 填写实际连接参数；暂不使用时分别设置为 `db: {}`、`redis: {}`。启用某类连接时，须包含名为 `default` 的连接 |
| JWT | 填写 `jwt.app`、`jwt.issuer` 和项目自己的密钥。入口始终初始化 JWT；HS256 密钥至少 32 字节 |
| 邮件通知 | 按需配置；关闭时将 `notify.recover.email.host` 设为空字符串，或省略 `notify` 节点 |

为 `jwt.secret` 生成新密钥：

```bash
openssl rand -hex 32
```

将生成结果填入配置文件。完整的无数据库、无 Redis、无邮件配置见[最小配置](#最小配置)。

### 3. 启动服务

```bash
make generate
make run
```

默认监听 `127.0.0.1:10001`。生成器依赖需完整记录在模块清单中；如遇 `missing go.sum entry`，按 [Wire 依赖注入](#wire-依赖注入)补齐固定版本工具依赖，再执行生成。

### 4. 验证服务

```bash
curl -i http://127.0.0.1:10001/heartbeat/state
curl -i http://127.0.0.1:10001/heartbeat/ready
```

| 接口 | 用途与响应 |
| --- | --- |
| `GET /heartbeat/state` | 查看整体及各已配置连接的健康状态；正常执行返回 HTTP 200，依赖异常体现在响应数据中 |
| `GET /heartbeat/ready` | 就绪探针；所有已配置依赖正常时返回 200，否则返回统一的 503 错误响应 |

每次探测中的数据库与 Redis 检查共用 2 秒期限。整体状态为 `healthy`、`degraded` 或 `unhealthy`。未配置数据连接时，返回 `healthy`，并省略空的 `components` 字段：

```json
{
  "trace_id": "b2401c9e-1f6f-4183-952a-b539ddabbb71",
  "data": {
    "status": "healthy",
    "timestamp": "2026-11-08T16:00:00+08:00"
  }
}
```

### 编译与配置路径

```bash
make build
./bin/server -config .env.yml
```

通过 `CONFIG` 或 `-config` 选择其他配置文件：

```bash
make run CONFIG=./configs/local.yml
make gormgen CONFIG=./configs/local.yml
go run ./cmd -config ./configs/local.yml
```

`configs/local.yml` 需自行创建。配置文件路径按进程工作目录解析，日志目录和 JWT 相对密钥路径以配置文件所在目录为基准。

## 架构与目录

业务按 `router → api → service → repo → Data` 组织，依赖通过构造函数传入。服务和仓库使用标准 `context.Context`，HTTP Context 留在处理层。

装配定义见 [cmd/wire.go](cmd/wire.go)，生成链见 [cmd/wire_gen.go](cmd/wire_gen.go)。注入入口接收 `ctx`、`conf`、`appLogger`，返回应用、清理函数和错误。数据初始化失败会返回错误；后续 HTTP 构建失败时，Wire 生成代码会调用已经获得的清理函数。

| 目录 | 职责与入口 |
| --- | --- |
| [cmd/](cmd/) | 启动、信号上下文、Wire 注入定义及生成结果 |
| [config/](config/) | YAML 加载、配置结构；字段定义位于 `config/autoload/` |
| [internal/server/](internal/server/) | 将应用配置转换为 HTTP 选项，接入 CORS、路由、pprof 和邮件通知 |
| [internal/router/](internal/router/) | 组织路由分组，将路径绑定到 API 处理器 |
| [internal/api/](internal/api/) | 绑定请求、调用服务、设置成功或错误响应 |
| [internal/service/](internal/service/) | 应用逻辑；健康检查服务还负责通过数据仓库探测连接 |
| [internal/data/](internal/data/) | 注入 `Data`，管理默认查询器、Redis 客户端和连接清理 |
| [internal/data/repo/](internal/data/repo/) | 业务仓库接口与实现 |
| [internal/data/model/](internal/data/model/) | 数据模型，`BaseModel` 包含 ID、创建时间、更新时间和软删除字段 |
| [internal/data/dbquery/](internal/data/dbquery/) | GORM/gen 查询器生成结果 |
| [internal/data/redis/action/](internal/data/redis/action/) | Redis 租约锁 |
| [internal/infra/auth/](internal/infra/auth/) | 按配置构造 `auth.JWT`，解析对称密钥与非对称 PEM 文件 |
| [internal/infra/shared/](internal/infra/shared/) | 通用依赖构造：`Environment`、`Datetime`；`ProviderSet` 已接入装配图 |
| [pkg/http/](pkg/http/) | HTTP 构建、路由、Context、统一响应及中间件 |
| [pkg/app/](pkg/app/) | 应用上下文和服务生命周期 |
| [pkg/startup/](pkg/startup/) | 数据依赖初始化总预算、单次期限与重试控制 |
| [pkg/repositories/](pkg/repositories/) | 按名称管理数据库和 Redis 连接 |
| [pkg/db/](pkg/db/)、[pkg/cache/](pkg/cache/) | MySQL/GORM 与 Redis 客户端封装 |
| [pkg/logger/](pkg/logger/)、[pkg/types/](pkg/types/) | 日志分类、TraceID 和请求上下文类型 |
| [pkg/proposal/](pkg/proposal/)、[pkg/notify/](pkg/notify/) | 告警消息、通知回调和邮件实现 |
| [errcode/](errcode/) | 业务错误、HTTP 状态码和中英文消息 |
| [generate/gormgen/](generate/gormgen/) | 查询器生成入口与模型注册 |

## 开发指南

业务模块沿用现有分层和 Wire 工作流扩展。以下 Go 示例使用项目内包路径，涉及 `internal/` 的示例应放在本模块内使用。

### HTTP 构建与路由

应用通常通过 `internal/server.NewHTTPServer` 构建服务器。该层将 YAML 中的正文限制、HTTP 超时和 CORS 设置转换成底层选项，并注册应用路由、pprof 及可选邮件通知。

底层 `pkg/http.NewServer` 的返回值为 `(*HTTPServer, error)`。它默认安装 Recovery 和 Request；CORS、路由及其他能力通过选项加入。直接调用底层构造函数时，需要自行传入相应选项，不能假定应用层的全部 YAML 配置会自动转换。

下面是独立构建 Handler 的示例；常规业务模块只需扩展现有 `internal/router/`：

```go
package example

import (
	"ult/config"
	"ult/pkg/http"
	"ult/pkg/http/middleware"
	"ult/pkg/logger"
)

func NewExampleServer(conf *config.Config, log *logger.Logger) (*http.HTTPServer, error) {
	cors, err := middleware.NewCORS(&middleware.CORSConfig{
		Enabled:        true,
		AllowedOrigins: []string{"https://app.example.com"},
	})
	if err != nil {
		return nil, err
	}
	return http.NewServer(conf, log, nil,
		http.WithMiddleware(cors),
		http.WithRoutes(func(root http.RouterGroup) {
			group := root.Group("/api")
			group.GET("/ping", func(ctx http.Context) {
				ctx.WithPayload(map[string]string{"message": "pong"})
			})
		}),
	)
}
```

#### 构建约定

构建过程先创建并安装完整中间件链，再按 `WithRoutes` 顺序同步注册路由。无效中间件、空处理函数、冲突路由等构建错误返回给调用方；中间件配置或路由回调中的 panic 会转换为构建错误。构建期间不绑定监听端口。

构建完成后可直接将服务器作为 `net/http.Handler` 使用，或调用 `Start(ctx)` 启动监听。此时不能再通过保留的路由组追加路由，也不支持热更新处理链。

`RouterGroup` 提供 `Group`、`Any`、`GET`、`POST`、`PUT`、`PATCH`、`DELETE`、`OPTIONS` 和 `HEAD`。处理函数类型为 `func(http.Context)`，而全局中间件使用 `gin.HandlerFunc`。

#### 构建选项

常用选项如下：

| 选项 | 含义 |
| --- | --- |
| `WithRoutes(register)` | 在构建阶段注册路由，可多次追加 |
| `WithMiddleware(items...)` | 追加全局中间件 |
| `WithMaxBodyBytes(bytes)` | 设置正文上限，必须大于零 |
| `WithReadHeaderTimeout(d)`、`WithReadTimeout(d)` | 请求头、整个请求读取期限 |
| `WithWriteTimeout(d)`、`WithIdleTimeout(d)` | 响应写入、连接空闲期限 |
| `WithTimeout(d)` | 单个 HTTP 服务优雅关闭期限，默认 5 秒 |
| `WithAlertNotify(handler)` | 配置内置 Recovery 的通知出口 |
| `WithPProf(environment)` | 按环境注册 `/debug/pprof`；`prod` 不注册 |
| `EnableOpenBrowser(url)` | 构建完成时尝试打开指定 URL |

构造函数第三个参数是 go-utils 的 `[]http.ServerOption`，用于底层网络、地址、显式 Endpoint 和 TLS 配置。当前 YAML 配置没有证书或 TLS 字段。

`ServerAgreement()` 返回 `Network`、`Addr`、`Target`。自动 Endpoint 的查询会提前绑定监听器，以获得实际端口（包括配置端口为 0 的情况）；即使没有调用 `Start`，查询后也应调用 `Stop` 释放端口。`ServerType()` 不触发绑定。

### 中间件

应用内置链按以下顺序执行：

`Recovery → CORS（启用时）→ Request → 业务中间件 → 路由处理函数`。

自定义中间件按声明的优先级稳定排序。

| 中间件 | 优先级 | 职责 |
| --- | --- | --- |
| Recovery | `PriorityHighest`，0 | 最外层恢复边界，统一收尾及通知 |
| CORS | `PriorityHighest`，0 | 在正文读取前处理跨域，依靠先注册 Recovery 的稳定排序保持恢复边界 |
| Request | `PriorityHigh`，1 | 创建 HTTP Context、限制并缓存正文、设置校验器、登记收尾回调 |
| 业务中间件 | 通常为 `PriorityHigh`、`PriorityNormal` 或 `PriorityLow` | 在需要时读取已经初始化的业务 Context |

合法来源的正文超限、读取超时等响应保留 CORS 头；默认预检请求直接返回 204，不进入 Request 和业务处理。禁用 CORS 或来源不在允许列表中时，不添加允许来源头。

`middleware.Middleware` 必须实现 `Name()`、`Priority()`、`Handler()`、`Dependencies()`。`Enabled() bool` 是可选接口：实现时在注册阶段过滤禁用项，未实现时默认启用。

依赖声明只校验名称是否存在，不负责拓扑排序，也不会覆盖优先级。使用业务 Context 的中间件，应在 Request 之后执行，并可声明对 `types.RequestMiddlewareName` 的依赖。内置名称 `recovery`、`request`、`cors` 不应被重复使用。

#### 自定义中间件

函数式中间件示例：

```go
package example

import (
	"ult/pkg/http"
	"ult/pkg/http/middleware"
	"ult/pkg/types"

	utilsMiddleware "github.com/devpety/go-utils/v3/middleware"
	"github.com/gin-gonic/gin"
)

func RequestMetadata() middleware.Middleware {
	return middleware.NewMiddlewareFuncWithDependencies(
		"request-metadata",
		utilsMiddleware.PriorityNormal,
		func(ctx *gin.Context) {
			request := ctx.MustGet(types.CoreContextNameKey).(http.Context)
			request.SetHeader("X-Request-Method", request.Method())
			ctx.Next()
		},
		[]string{types.RequestMiddlewareName},
	)
}
```

将返回值传入 `http.WithMiddleware(RequestMetadata())`。需要独立管理时，`middleware.NewManager()` 提供 `Use`、`UseFunc`、`Build`、`List`、`Get`、`Has`、`Count`、`Remove`、`Clear`；`Use` 和 `Build` 返回的错误需要处理。`List` 返回切片副本，但中间件实例仍是原对象，运行期不要修改其配置。

### 请求绑定与响应

#### 参数绑定与校验

| 方法 | 行为 |
| --- | --- |
| `BindAndValidateJSON(&req)` | 绑定 JSON，再执行框架校验；失败自动设置错误并 Abort |
| `BindAndValidateForm(&req)` | 绑定查询和表单，再执行框架校验；失败自动设置错误并 Abort |
| `Validate(&req)` | 只校验现有结构体，不读取或覆盖请求数据；失败自动设置错误并 Abort |
| `ShouldBindQuery`、`ShouldBindPostForm`、`ShouldBindForm`、`ShouldBindJSON`、`ShouldBindURI` | 返回 Gin 的绑定及其内置校验结果；不自动转换成框架错误或 Abort |

组合绑定方法要求目标为非 nil 的结构体指针。普通解析错误返回 `ParamBindError`（400），参数校验错误返回 `ParamValidateError`（422）；错误的绑定目标、无效校验器等程序错误返回 `ServerError`（500）。

`json`、`form`、`uri` 分别指定绑定字段；框架校验规则使用 `validate` 标签。`validator.tagname` 指定错误消息显示的字段标签，例如 `label`，不改变规则标签。Gin 绑定器还会执行 `binding` 标签校验。

处理函数示例：

```go
package example

import (
	"context"
	nethttp "net/http"
	"ult/errcode"
	"ult/pkg/http"
)

type CreateItemRequest struct {
	Name string `json:"name" form:"name" label:"名称" validate:"required,min=1,max=30"`
}

type ItemService interface {
	Create(ctx context.Context, name string) (any, error)
}

type ItemHandler struct {
	service ItemService
}

func (h *ItemHandler) Create(ctx http.Context) {
	var req CreateItemRequest
	if err := ctx.BindAndValidateJSON(&req); err != nil {
		return
	}
	result, err := h.service.Create(ctx.RequestContext(), req.Name)
	if err != nil {
		ctx.WithAbortError(errcode.New(errcode.ServerError).WithStackError(err))
		return
	}
	ctx.WithStatus(nethttp.StatusCreated)
	ctx.WithPayload(result)
}
```

查询和 URI 参数可先用对应的 `ShouldBind*` 方法绑定，再调用 `Validate`。绑定失败时需要自行调用 `WithAbortError(errcode.New(errcode.ParamBindError).WithStackError(err))` 并返回。

#### 统一响应

`WithPayload` 仅保存负载；`WithStatus` 设置尚未提交的成功状态码，允许 200–299，默认 200。`WithAbortError` 保存 `BusinessError`、设置状态并 Abort，但不会让当前 Go 函数立即返回，调用后仍需 `return`。

成功响应包含 `trace_id` 和 `data`，错误响应包含 `trace_id`、`code`、`message`、`desc`：

```json
{
  "trace_id": "b2401c9e-1f6f-4183-952a-b539ddabbb71",
  "code": 200004,
  "message": "参数校验错误",
  "desc": "名称为必填字段"
}
```

- 匹配的 HEAD 请求不写正文；204、205 等无正文成功状态也不写正文。
- 未匹配路径使用 Gin 默认 404 正文，不会自动变成框架业务错误 JSON。
- 通过 `GinContext()` 直接提交 JSON、文件、重定向或 Flush 时，框架不再追加统一正文。
- 正常统一 JSON 序列化失败且响应尚未提交时，会改写为统一 500。

#### 异常恢复与请求收尾

Request 只登记回调，由最外层 Recovery 在下游返回或发生 panic 后执行；回调每个请求最多执行一次。

- 响应尚未提交的普通 panic：记录异常，返回统一 500。
- 响应已经提交的普通 panic：记录异常，再用 `http.ErrAbortHandler` 终止底层传输，避免将截断的 chunked 正文作为完整响应结束。
- 收到标准库 `http.ErrAbortHandler`：原样透传，不改写为业务错误 JSON；已经创建的 HTTP Context 仍执行回收。
- 统一响应回调中的 panic 也经过相同的恢复边界。

自定义 Recovery 时需要在正常和异常路径调用 `middleware.CompleteRequest(ctx)`，同时保留传输中断语义。它只执行 Request 登记的回调，不替代完整的恢复中间件。接入点见 [completion.go](pkg/http/middleware/completion.go) 和 [recovery.go](pkg/http/middleware/recovery.go)。

### 业务错误

错误常量为 `int`，使用 `errcode.New(code)` 或已有 `Err*` 创建、派生错误。当前 HTTP 映射见 [errhttp.go](errcode/errhttp.go)：

| 常量 | 业务码 | HTTP |
| --- | --- | --- |
| `ServerError` | 100001 | 500 |
| `ServiceUnavailableError` | 100002 | 503 |
| `AuthorizationError` | 200001 | 401 |
| `ParamBindError` | 200002 | 400 |
| `ParamValidateError` | 200004 | 422 |
| `RequestBodyTooLargeError` | 200014 | 413 |
| `RequestTimeoutError` | 200015 | 408 |
| 其他现有 `200xxx` 错误 | 见 [code.go](errcode/code.go) | 400 |

`BusinessError` 实现 `error` 和 `Unwrap`，区分内部原因与对外描述：

- `WithStackError(err)` 保存内部错误和堆栈，不会自动设置 `desc`；传入 nil 保留原有内部信息。
- `WithDesc(text)` 设置响应中的公开描述，适合明确的业务提示。
- `Alert()` 标记通知需求。普通业务错误不会自动获得该标记，普通 panic 会被标记。
- 上述修饰方法都返回独立副本，必须使用返回值；不会修改共享的 `Err*`。
- `errors.Is` / `errors.As` 可检查附加的内部原因；比较派生错误的业务类型时使用 `BusinessCode()`，不要依赖与原错误指针相等。
- HTTP 响应只读取消息和公开描述；`Error()` 包含内部原因，供内部诊断使用。

#### 注册业务错误

注册应用错误的示例：

```go
package example

import (
	"net/http"
	"ult/errcode"
)

const ItemConflict = 200100

func RegisterBusinessErrors() {
	errcode.RegisterHTTPCodes(map[int]int{
		ItemConflict: http.StatusConflict,
	})
	errcode.RegisterTexts(errcode.ZhCN, map[int]string{
		ItemConflict: "项目已存在",
	})
}
```

在创建对应错误和开始处理请求前完成注册，再使用 `errcode.New(ItemConflict)`。如需新增包级预创建变量，可沿用现有错误码文件和 `vars.go` 的方式。

`NewRegistry` 设置进程级语言，支持 `zh-cn`、`en-us`，不按请求切换。注册表错误的消息在读取时按当前语言解析；`NewError` 的显式消息保持固定。HTTP 状态码在错误创建时确定。

### 请求上下文与链路标识

API 调用下游时传递 `ctx.RequestContext()`，服务、仓库及日志继续使用这个标准上下文。

HTTP Context 从已有缓存或请求头 `x-md-trace-id` 读取 ID，缺少时生成 UUID。`TraceID()` 和 `RequestContext()` 用 `sync.Once` 缓存初始化结果。Gin Context、响应写入器和 HTTP Context 按请求内串行访问。

HTTP Context 仅在当前请求内使用，收尾时清除引用并归还对象池。后台任务应自行管理上下文；下游已取得的标准请求上下文独立保存 TraceID，并遵守请求取消语义。

| 访问方式 | 语义 |
| --- | --- |
| `Header()` | 原请求头引用，按只读方式使用 |
| `CloneHeaders()` | 完整副本，修改副本不影响请求头 |
| `RawData()` | 缓存正文的字节副本，修改副本不影响原始数据 |
| `Request()`、`GinContext()`、`ResponseWriter()` | 底层对象，遵守各自的生命周期和并发约束 |

日志按 `app`、`sql`、`request` 分类；应用通过注入的 `*logger.Logger` 调用 `UseApp(ctx)` 记录，SQL 和请求日志使用同一 TraceID 提取入口。`types.TraceIDFromContext` 只读取结构化请求上下文，不读取同名字符串键，不生成新 ID；没有有效 ID 时日志字段为 `null`。

非 HTTP 入口可以通过 `types.NewRequestContext` 或 `pkg/app.NewAppContextWithTraceId` 创建带 TraceID 的上下文。

### 数据访问与生成

#### 默认连接和请求上下文

`internal/data.Data` 的主要入口：

| 方法 | 返回内容 |
| --- | --- |
| `DB()` | 默认连接的 `*dbquery.Query` |
| `GormDB()` | 默认连接的 `*gorm.DB` |
| `Redis()` | 默认 Redis 客户端 |
| `WithContext(ctx)` | 新的数据视图，将 GORM 和生成查询器绑定到调用上下文 |
| `DataRepo()` | 数据仓库视图，包含数据库与 Redis 连接集合 |

未配置相应连接时，默认 DB / Redis 入口可能为 nil，业务模块应明确其依赖。Redis 命令还需显式传入 `ctx`，`WithContext` 不会把共享 Redis 客户端永久绑定到某个请求。

查询器从注入的数据实例获取，字段与执行器使用同一个实例：

```go
package example

import (
	"context"
	"errors"
	"ult/internal/data"
	"ult/internal/data/model"
)

func FindTest(ctx context.Context, source data.Data, id int) (*model.Test, error) {
	query := source.WithContext(ctx).DB()
	if query == nil {
		return nil, errors.New("未配置默认数据库")
	}
	return query.Test.WithContext(ctx).Where(query.Test.ID.Eq(id)).First()
}
```

现有实现见 [internal/data/repo/test.go](internal/data/repo/test.go)。框架不依赖 `dbquery.Q` / `dbquery.Test` 全局默认查询器。

额外连接通过 `source.DataRepo().DbRepo.DB("名称")`、`source.DataRepo().RedisRepo.Redis("名称")` 取得，再由 `Get()` 访问客户端。名称不存在时返回 nil。仓库的 `All()` 返回映射副本，连接实例仍共享；连接统一由数据层清理，业务代码不要自行关闭共享连接。

#### 模型与查询器生成

新增模型后，在 [generate/gormgen/db/default.go](generate/gormgen/db/default.go) 的 `g.ApplyBasic(...)` 中注册，再执行 `make gormgen`。模型可以嵌入 `BaseModel`，不要再重复定义同名 ID 字段。

生成器使用 Go 模型生成查询器，仍会连接默认数据库完成客户端初始化；它不负责建库、建表或迁移。[static/db/ult.sql](static/db/ult.sql) 当前为空，需要自行准备表结构。生成文件通过生成器更新，不手工维护。

示例 `TestRepo.Update` 使用生成器的 `Save`，执行 `INSERT ... ON DUPLICATE KEY UPDATE`，具有插入或更新语义。纯更新场景应在业务仓库中明确使用相应的更新操作。

#### 事务

生成的 `Query` 提供 `Transaction`、`Begin`、`Commit`、`Rollback` 等原语。事务内应使用回调中的 `tx`，其他仓库重新获取根 Data 连接不会自动参与该事务：

```go
package example

import (
	"context"
	"errors"
	"ult/internal/data"
	"ult/internal/data/dbquery"
	"ult/internal/data/model"
)

func CreateTwo(ctx context.Context, source data.Data, first, second *model.Test) error {
	query := source.WithContext(ctx).DB()
	if query == nil {
		return errors.New("未配置默认数据库")
	}
	return query.Transaction(func(tx *dbquery.Query) error {
		if err := tx.Test.WithContext(ctx).Create(first); err != nil {
			return err
		}
		return tx.Test.WithContext(ctx).Create(second)
	})
}
```

#### Redis 租约锁

[internal/data/redis/action/lock.go](internal/data/redis/action/lock.go) 提供 `NewLock(client, key, duration)`、`Lock(ctx)`、`TryLock(ctx, retryInterval, timeout)`、`Unlock(ctx)`、`IsLocked(ctx)`、`IsHeldByMe(ctx)`。

- 租约至少 1 毫秒，按毫秒向下取整，不自动续期；临界区必须在租约内完成。
- 获取返回 `false, nil` 表示被其他令牌占用；连接故障、取消和租约失效返回错误。
- 同一实例不支持并发变更所有权，也不能在未释放前再次获取。
- 解锁使用独立 2 秒期限，不继承调用方取消，但保留上下文值；仅删除自身令牌对应的键。
- `IsLocked`、`IsHeldByMe` 是即时快照，不能替代原子获取，也不保证后续操作期间租约仍有效。

### JWT 工具

应用通过 `internal/infra/auth.NewJWT(conf)` 构造 `auth.JWT`。入口只用它做启动期配置校验，业务模块把 `NewJWT` 加入自己的 `ProviderSet` 即可按需注入。配置字段为 `app`、`issuer`、`algorithm`、`secret`、`private_key_file`、`public_key_file`、`audience`。

| 算法 | 配置要求 |
| --- | --- |
| HS256 / HS384 / HS512 | 只配置 `secret`，最低长度分别为 32 / 48 / 64 字节 |
| RS256/384/512、PS256/384/512 | 配置 RSA 私钥或公钥文件，密钥至少 2048 位 |
| ES256 / ES384 / ES512 | 配置匹配 P-256 / P-384 / P-521 的 EC 密钥文件 |
| EdDSA | 配置 Ed25519 密钥文件 |

算法留空使用 HS256。非对称算法只能选择一个私钥或公钥文件，并清空 `secret`；只提供公钥的实例可以验签，不能签发。每个文件只接受一个未加密 PEM 块：私钥支持 PKCS#8、RSA PKCS#1、EC SEC1，公钥支持 PKIX 或 RSA PKCS#1。相对路径以配置目录为基准。

使用示例：

```go
package example

import (
	"time"

	"github.com/devpety/go-utils/v3/auth"
)

func IssueToken(jwt auth.JWT, id string) (string, error) {
	return jwt.GenerateToken(id, time.Hour)
}

func ParseTokenID(jwt auth.JWT, token string) (string, error) {
	claims, err := jwt.ParseToken(token)
	if err != nil {
		return "", err
	}
	return claims.ID, nil
}
```

签发时 `id` 写入 `jti`，`sub` 为配置的应用名；验签约束固定算法、签名、`iss`、`sub`、过期时间和生效时间。配置 `audience` 时要求任一接收方匹配；不配置时不约束接收方，签发默认 audience 为 `id`。令牌过期时刻必须位于未来。

业务鉴权中间件负责读取 Authorization 请求头、调用解析工具，并执行权限判断。

### 新增业务模块

按以下步骤接入业务模块；不涉及持久化的模块可从服务层开始：

1. 在 `internal/data/model/` 定义模型，加入 GORM 生成器模型列表，执行 `make gormgen` 更新查询器。
2. 在 `internal/data/repo/` 定义仓库接口及实现，通过构造函数注入 `data.Data`，按请求传递标准上下文。
3. 在 `internal/service/` 实现业务逻辑，通过构造函数注入所需仓库与日志等通用依赖。
4. 在 `internal/api/` 绑定、校验请求，调用服务，设置响应并及时返回。
5. 将构造函数加入对应 `ProviderSet`；在 `internal/router/` 注册路径，需要时扩展路由构造函数的依赖。
6. 执行 `make wire`，检查生成的装配和清理链，再编译项目。

已有模块参考：[仓库](internal/data/repo/test.go)、[服务](internal/service/heartbeat.go)、[API](internal/api/heartbeat.go)、[路由](internal/router/router.go)。新增依赖继续通过现有构造函数和 `ProviderSet` 装配。

## 配置参考

配置模板见 [.env.example.yml](.env.example.yml)，字段定义见 [config/autoload/](config/autoload/)。

加载器读取单个显式指定的 YAML 文件，严格校验未知字段，并拒绝空文件和多个 YAML 文档。配置在启动时加载，文件内容按字面值解析；环境变量合并、`${...}` 替换及热更新需由应用另行处理。

### 最小配置

下面是不使用数据库、Redis 和邮件的完整配置示例。将 `jwt.secret` 替换为生成的密钥后，可保存为本地配置并启动心跳服务：

```yaml
environment: dev
app:
  id: ult.service.001
  name: ult
  version: 1.0.0
  startup_timeout_ms: 30000
datetime:
  location: Asia/Shanghai
  cst_layout: "2006-01-02 15:04:05"
language:
  local: zh-cn
validator:
  locale: zh
  tagname: label
logger:
  max_size: 128
  max_backups: 5
  max_age: 7
  local_time: true
  compress: true
db: {}
redis: {}
jwt:
  app: ult.service
  issuer: ult.local
  algorithm: HS256
  secret: CHANGE_ME
  audience: []
server:
  http:
    network: tcp
    host: 127.0.0.1
    port: 10001
    cors:
      enabled: false
```

`language.local` 使用 `zh-cn` / `en-us`；校验器的 `validator.locale` 使用 `zh` / `en`，两者不是同一套取值。默认入口的文件日志位于 `<配置目录>/runtime/logs/<app.name>.log`。

### HTTP 限制与 CORS

以下字段配置为 0 时使用默认值：

| `server.http` 字段 | 单位 | 默认值 |
| --- | --- | --- |
| `max_body_bytes` | 字节 | 2097152（2 MiB） |
| `read_header_timeout_ms` | 毫秒 | 5000 |
| `read_timeout_ms` | 毫秒 | 15000，包含请求头和正文 |
| `write_timeout_ms` | 毫秒 | 30000 |
| `idle_timeout_ms` | 毫秒 | 60000 |

上述字段不接受负值，读取请求头期限不能大于整个请求读取期限。HTTP 正文在进入业务处理前完整读取并缓存；超限返回 413，读取超时返回 408。I/O 读写超时不等同于给业务代码添加执行 Deadline，业务操作的期限仍通过标准上下文控制。

CORS 配置片段，合并到 `server.http.cors`：

```yaml
enabled: true
allowed_origins:
  - https://app.example.com
allowed_methods: [GET, POST, PUT, PATCH, DELETE, HEAD]
allowed_headers: [Content-Type, Authorization, x-md-trace-id]
allow_credentials: false
```

启用时必须有来源列表。来源为完整 HTTP/HTTPS Origin，不包含路径、查询、片段或通配子域；单独的 `*` 只在不携带凭证时允许。底层 `CORSConfig.OptionsPassthrough` 只通过 Go 配置提供，当前 YAML 未暴露该字段。

### 数据连接初始化

所有已配置连接共享 `app.startup_timeout_ms` 初始化预算，0 使用 30000 毫秒。数据库和 Redis 按连接名排序、依次初始化，任一已配置连接失败都会使应用初始化失败，并关闭已经打开的连接。成功交付的连接不会继承已经结束的初始化上下文。

| 配置 | 语义 | 0 的含义 |
| --- | --- | --- |
| `db.<name>.max_retries` | 数据库初始化最大尝试次数，包含首次尝试 | 3 次 |
| `db.<name>.retry_delay` | 数据库初始化尝试间隔，秒 | 1 秒 |
| `db.<name>.connect_timeout_ms` | 数据库单次初始化期限，毫秒 | 5000 毫秒 |
| `redis.<name>.startup_max_attempts` | Redis 初始化最大尝试次数，包含首次尝试 | 3 次 |
| `redis.<name>.startup_retry_delay` | Redis 初始化尝试间隔，秒 | 1 秒 |
| `redis.<name>.connect_timeout_ms` | Redis 单次初始化期限，毫秒 | 5000 毫秒 |

以上尝试仍受共享总预算和调用方取消约束。Redis 初始化重试与运行期命令重试分别配置。

### MySQL

数据库封装基于 MySQL / GORM。可以使用 `dsn`，或配置 `host`、`port`、`username`、`password`、`db_name`、`charset`、`parse_time`、`loc`；非空 DSN 优先决定连接参数，连接池和表名前缀独立配置。

| 字段 | 语义 |
| --- | --- |
| `prefix` | GORM 表名前缀，命名策略使用单数表名 |
| `max_open_conn` | 最大连接数；框架传入配置原值，0 表示不限 |
| `max_idle_conn` | 最大空闲连接数；0 不保留空闲连接 |
| `max_life_time` | 连接寿命，秒；0 不限制 |
| `parse_time` | 字符串配置，例如 `"true"` |

需要有限连接池时显式填写参数，模板中的值为 100 个最大连接、10 个空闲连接、1800 秒寿命。连接配置非空时必须有 `default`；额外连接由名称区分，不自动形成读写分离。

### Redis

当前使用单节点客户端；多个命名连接分别初始化，不自动组成集群或故障转移客户端。`network: tcp` 时由 `addr`、`port` 组成地址；`network: unix` 时直接使用 `addr` 路径。

| 字段 | 单位及特殊值 |
| --- | --- |
| `max_retries` | 运行期命令最大重试次数；0 或 -1 禁用，正数用于可安全重放命令的连接 |
| `dial_timeout`、`pool_timeout` | 毫秒；0 使用底层默认值 |
| `read_timeout`、`write_timeout` | 毫秒；0 使用默认值，-1 仅禁用默认 socket 超时，仍考虑调用上下文 Deadline |
| `min_retry_backoff`、`max_retry_backoff` | 毫秒；0 分别使用 8 / 512 毫秒，-1 禁用退避 |
| `max_conn_age` | 秒；0 或 -1 不限制连接寿命 |
| `idle_timeout` | 秒；0 使用 30 分钟，-1 禁用空闲过期检查 |
| `pool_size`、`min_idle_conns`、`pool_fifo` | 连接池大小、最小空闲连接数、FIFO 策略 |

运行期 socket 读写是否立即响应提前取消，受固定版本 go-redis 实现约束；传入上下文不等于可以强制中断任意阻塞操作。完整转换规则见 [pkg/cache/redis.go](pkg/cache/redis.go)。

### 通知与 pprof

`notify.recover.email.host` 非空时，应用层将邮件处理器接入内置 Recovery。还需配置端口、用户、密码和收件人；`to` 使用逗号分隔。通知消息类型为 `proposal.AlertMessage`，自定义出口通过 `WithAlertNotify(proposal.NotifyHandler)` 配置。

应用层调用 `WithPProf(config.Environment)`，非 `prod` 环境注册 `/debug/pprof`；`prod` 不注册。JWT 工具不会自动为这些路由或业务路由添加鉴权。

## 运行与部署

### 应用生命周期

入口通过信号上下文将取消传给 Wire 的数据初始化，再交给 `pkg/app.App` 管理运行生命周期。入口监听 `SIGINT`、`SIGTERM`、`SIGHUP`、`SIGQUIT`。

运行阶段收到信号、上层上下文取消、服务错误或所有服务正常退出时，会进入停止流程：使用独立的限时上下文调用服务 `Stop`，随后取消运行上下文并等待启动协程退出。HTTP 停止失败时再关闭残留连接；`Run` 返回后由入口执行 Wire 清理函数，关闭数据库和 Redis。

| 预算 | 默认值 | 调整入口 |
| --- | --- | --- |
| 数据依赖共享初始化预算 | 30 秒 | YAML `app.startup_timeout_ms` |
| 单次数据连接初始化期限 | 5 秒 | 各连接 `connect_timeout_ms` |
| 应用服务停止和退出总预算 | 30 秒 | `pkg/app.WithAppShutdownTimeout` |
| 单个 HTTP 服务停止期限 | 5 秒 | `pkg/http.WithTimeout` |

HTTP 停止受自身期限和应用总预算中较早者约束。当前后两项通过 Go 选项配置，没有对应的 YAML 字段。业务仍需传递请求上下文、处理取消信号，并自行管理后台任务和已劫持连接的生命周期。

### 容器入口

仓库提供 [Dockerfile](Dockerfile) 和 [docker-compose.yml](docker-compose.yml)。当前容器命令执行 `make init && make generate && make run`，Compose 发布 10001 端口，运行时仍需要 Go 工具链、make 和 sed。

容器配置需要将 `server.http.host` 设为 `0.0.0.0`，并填写从容器内可达的数据库和 Redis 地址；模板的 `127.0.0.1` 只监听容器回环接口。

现有基础镜像标签为 `raylin666/golang:1.17-wire`，镜像实际工具链需要满足当前 Go 1.23.0 的要求。该入口在容器启动时生成代码，再通过 `go run` 运行服务；使用前需核对镜像工具链与运行配置。

## 命令参考

| 命令 | 用途 |
| --- | --- |
| `make init` | 在缺少 `.env.yml` 时复制配置模板 |
| `make generate` | 执行 `go generate -mod=readonly ./...`，当前生成入口为 Wire |
| `make wire` | 使用 `go.mod` 锁定的 Wire 生成 `cmd/wire_gen.go`，并统一其生成指令 |
| `go generate ./...` | 通过 `cmd/wire_gen.go` 中的指令调用 `make -C .. wire` |
| `make gormgen` | 连接默认 MySQL，按已注册 Go 模型生成查询器 |
| `make run` | 执行 `go run ./cmd -config "$(CONFIG)"` |
| `make build` | 将入口编译为 `bin/server` |
| `make help` | 显示命令说明 |

### Wire 依赖注入

Wire 版本来自 [go.mod](go.mod)，当前为 `v0.7.0`。生成流程不依赖全局安装的 `wire` 二进制，也不执行 `go get ...@latest` 或 `go mod tidy`。Wire 子进程继承只读模块模式；缺少模块声明时会报错，需要显式处理依赖变更。

Wire CLI 的依赖也需要保留在模块清单中。如果依赖整理后生成或检查提示 `missing go.sum entry`（例如缺少 `github.com/google/subcommands`），先显式补齐当前固定版本的工具依赖：

```bash
go get github.com/google/wire/cmd/wire@v0.7.0
```

该准备命令会更新 `go.mod`、`go.sum`，执行后检查依赖差异，再重试生成。日常生成入口仍保持只读模式，不自动整理或升级依赖。

Wire 自带生成器会写入 `-mod=mod` 指令，因此 `make wire` 在生成后将该指令替换为统一的 Makefile 入口。使用上述入口生成，不需要手工修改生成文件。

### GORM 查询器生成

GORM 查询器使用独立命令，`make generate` 不会执行 `make gormgen`。输出目录可通过生成器参数指定：

```bash
go run ./generate/gormgen -config .env.yml -out internal/data/dbquery
```

### 编译与静态检查

以下在项目根目录执行命令。Wire 检查与生成使用相同的工具依赖：

```bash
go build -mod=readonly ./...
go vet -mod=readonly ./...
go run -mod=readonly github.com/google/wire/cmd/wire check ./cmd
```

应用启动信息中的版本来自 `app.version`。修改展示版本请设置该字段；当前入口未读取 Makefile 注入的 `main.Version` 链接变量。

## 主要依赖

版本以 [go.mod](go.mod) 和 [go.sum](go.sum) 为准，下表列出当前框架使用的主要组件：

| 组件 | 模块 | 版本 |
| --- | --- | --- |
| Go | `go` 指令 | 1.23.0 |
| 通用工具 | `github.com/devpety/go-utils/v3` | v3.0.1 |
| HTTP | `github.com/gin-gonic/gin` | v1.11.0 |
| 注入生成 | `github.com/google/wire` | v0.7.0 |
| ORM | `gorm.io/gorm` | v1.31.1 |
| MySQL 方言 | `gorm.io/driver/mysql` | v1.5.7 |
| 查询器生成 | `gorm.io/gen` | v0.3.18 |
| Redis | `github.com/redis/go-redis/v9` | v9.0.5 |
| JWT | `github.com/golang-jwt/jwt/v5` | v5.3.1 |
| 日志 | `go.uber.org/zap` | v1.27.0 |
| 校验 | `github.com/go-playground/validator/v10` | v10.27.0 |
| CORS | `github.com/rs/cors` | v1.8.1 |
| pprof 路由 | `github.com/gin-contrib/pprof` | v1.4.0 |
| YAML | `gopkg.in/yaml.v3` | v3.0.1 |
