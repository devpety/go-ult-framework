// Package http 提供 HTTP 服务器实现，基于 Gin 框架封装。
// 该包封装了常用的 HTTP 服务器功能，包括请求处理、中间件管理、CORS 支持和优雅关闭。
// 实现了自定义的 Context 包装器，提供请求验证、错误处理和响应格式化等额外功能。
package http

import (
	stdCtx "context"
	"errors"
	"fmt"
	nethttp "net/http"
	"time"
	"ult/config"
	"ult/errcode"
	"ult/pkg/app"
	"ult/pkg/logger"
	pkgtypes "ult/pkg/types"

	"github.com/devpety/go-utils/v3/http"
	utilsserver "github.com/devpety/go-utils/v3/server"
	"github.com/gin-gonic/gin"
)

// HTTPServer HTTP 服务器实例，实现了 app.Server 接口。
// 封装了 Gin 引擎，提供中间件链、CORS 处理、Panic 恢复和结构化日志等功能。
var _ app.Server = (*HTTPServer)(nil)

// HTTPServer 是已完成中间件和路由配置的运行实例。
// 通过 NewServer 创建后，可直接作为 http.Handler 使用或启动监听。
type HTTPServer struct {
	server  *http.Server
	engine  *gin.Engine
	config  *config.Config
	logger  *logger.Logger
	timeout time.Duration
}

// Config 返回应用配置。
func (srv *HTTPServer) Config() *config.Config {
	return srv.config
}

// Logger 返回服务器关联的日志记录器实例。
func (srv *HTTPServer) Logger() *logger.Logger {
	return srv.logger
}

// ServeHTTP 实现 http.Handler 接口。
// 将请求处理委托给底层的 Gin 引擎。
func (srv *HTTPServer) ServeHTTP(writer nethttp.ResponseWriter, request *nethttp.Request) {
	srv.engine.ServeHTTP(writer, request)
}

// ServerAgreement 返回配置的传输网络及底层有效 Endpoint，保留实际端口和显式公布地址。
// 自动 Endpoint 会提前绑定监听器；未启动的实例也需调用 Stop 释放端口。
func (srv *HTTPServer) ServerAgreement() (*app.ServerAgreement, error) {
	endpoint, err := srv.server.Endpoint()
	if err != nil {
		return nil, fmt.Errorf("获取 HTTP 服务入口失败: %w", err)
	}
	if endpoint == nil || endpoint.Host == "" || (endpoint.Scheme != "http" && endpoint.Scheme != "https") {
		return nil, fmt.Errorf("HTTP 服务入口必须包含有效的 HTTP 或 HTTPS 地址")
	}
	// go-utils 用 isSecure 标记 TLS，公布为访问 URL 时转换为 HTTPS scheme。
	publicEndpoint := *endpoint
	query := publicEndpoint.Query()
	if utilsserver.EndpointIsSecure(endpoint) {
		publicEndpoint.Scheme = "https"
	}
	if query.Has("isSecure") {
		query.Del("isSecure")
		publicEndpoint.RawQuery = query.Encode()
	}
	network := srv.config.Server.Http.Network
	if network == "" {
		network = "tcp"
	}
	return &app.ServerAgreement{Network: network, Addr: publicEndpoint.Host, Target: publicEndpoint.String()}, nil
}

// ServerType 返回服务器类型，不查询会触发监听器绑定的 Endpoint。
func (srv *HTTPServer) ServerType() string {
	return utilsserver.HTTPServerType
}

// StartBefore 服务器启动前的钩子方法。
// 可被重写以执行启动前初始化操作。
func (srv *HTTPServer) StartBefore() {}

// StartAfter 服务器启动后的钩子方法。
// 可被重写以执行启动后操作。
func (srv *HTTPServer) StartAfter() {}

// CancelBefore 服务器停止前的钩子方法。
// 可被重写以执行停止前清理操作。
func (srv *HTTPServer) CancelBefore() {}

// CancelAfter 服务器停止后的钩子方法。
// 可被重写以执行停止后清理操作。
func (srv *HTTPServer) CancelAfter() {}

// Start 开始监听并提供 HTTP 请求服务。
// 记录服务器地址并启动底层 HTTP 服务器。
//
// 参数:
//   - ctx: 服务器生命周期的上下文
//
// 返回:
//   - error: 服务器启动过程中发生的任何错误
func (srv *HTTPServer) Start(ctx stdCtx.Context) error {
	if ctx.Err() != nil {
		return srv.server.Start(ctx)
	}
	agreement, err := srv.ServerAgreement()
	if err != nil {
		// Stop 可能先于启动协程完成，保持底层停止后启动的正常退出语义。
		if errors.Is(err, nethttp.ErrServerClosed) {
			return nil
		}
		return errors.Join(err, srv.server.Close())
	}
	srv.logger.UseApp(ctx).Info(fmt.Sprintf("HTTP 服务入口 %s", agreement.Target))
	return srv.server.Start(ctx)
}

// Stop 优雅关闭 HTTP 服务器，使用超时机制。
// 使用配置的超时时间进行优雅关闭。
// 等待失败时关闭残留连接，并保留原始错误。
//
// 参数:
//   - ctx: 关闭操作的上下文
//
// 返回:
//   - error: 关闭过程中发生的任何错误
func (srv *HTTPServer) Stop(ctx stdCtx.Context) error {
	ctx, cancel := stdCtx.WithTimeout(ctx, srv.timeout)
	defer cancel()
	if err := srv.server.Stop(ctx); err != nil {
		return errors.Join(err, srv.server.Close())
	}
	return nil
}

// handlerResponse 响应处理。
// 根据请求状态生成成功或错误响应，记录请求日志。
//
// 参数:
//   - reqTime: 请求开始时间
//   - ctx: Gin 上下文
func (srv *HTTPServer) handlerResponse(reqTime time.Time, ctx *gin.Context) {
	if ctx.GetBool(pkgtypes.ContextResponseAbortedNameKey) {
		return
	}
	
	var (
		resp            interface{}
		httpCode        int
		businessCode    int
		businessMessage string
		stackErr        error
		traceId         string
	)

	// 获取核心上下文 Context
	appCtx, ok := ctx.Value(pkgtypes.CoreContextNameKey).(Context)
	if !ok || appCtx == nil {
		if _, exists := ctx.Get(pkgtypes.ContextAbortErrorNameKey); !exists {
			return
		}
		// 初始化失败时没有池对象，使用临时包装器读取错误和记录请求日志。
		appCtx = &context{ctx: ctx}
	}

	// 获取链路追踪 TraceId
	traceId = appCtx.TraceID()

	// 发生错误
	if ctx.IsAborted() && (appCtx.GetAbortError() != nil || !ctx.Writer.Written()) {
		if err := appCtx.GetAbortError(); err != nil {
			httpCode = err.HTTPCode()
			businessCode = err.BusinessCode()
			businessMessage = err.Message()
			stackErr = err.StackError()
			resp = NewErrorResponse(traceId, businessCode, businessMessage, err.Desc())
		} else {
			err := errcode.ErrUnknownError
			httpCode = err.HTTPCode()
			businessCode = err.BusinessCode()
			businessMessage = err.Message()
			stackErr = ctx.Err()
			resp = NewErrorResponse(traceId, businessCode, businessMessage, err.Desc())
		}
	} else {
		// 已由中间件提交的响应（如 CORS 预检）保留其状态，不将 Abort 当作业务错误。
		httpCode = ctx.Writer.Status()
		businessMessage = nethttp.StatusText(httpCode)
		resp = NewSuccessResponse(traceId, appCtx.GetPayload())
	}

	// 未匹配路由仍经过完整中间件链和日志；默认正文交给 Gin 的 404/405 处理。
	unmatchedRoute := ctx.FullPath() == "" && !ctx.IsAborted() &&
		(httpCode == nethttp.StatusNotFound || httpCode == nethttp.StatusMethodNotAllowed)
	if !ctx.Writer.Written() && !unmatchedRoute {
		if httpCode == 0 {
			httpCode = nethttp.StatusInternalServerError
		}

		switch {
		case httpCode == nethttp.StatusNoContent || httpCode == nethttp.StatusResetContent || httpCode == nethttp.StatusNotModified:
			ctx.Status(httpCode)
			ctx.Writer.WriteHeaderNow()
		case appCtx.Method() == nethttp.MethodHead:
			ctx.Header("Content-Type", "application/json; charset=utf-8")
			ctx.Status(httpCode)
			ctx.Writer.WriteHeaderNow()
		default:
			ctx.JSON(httpCode, resp)
		}

		// `ctx.JSON` 在序列化失败时不会 panic，而是把错误 append 到`ctx.Errors` 后静默返回，且正文没写出去 （`Writer.Written()` 仍为`false` ）
		if !ctx.Writer.Written() && len(ctx.Errors) > 0 {
			businessErr := errcode.New(errcode.ServerError).WithStackError(ctx.Errors.Last().Err)
			appCtx.WithAbortError(businessErr)
			businessCode = businessErr.BusinessCode()
			businessMessage = businessErr.Message()
			stackErr = businessErr.StackError()
			ctx.JSON(nethttp.StatusInternalServerError, NewErrorResponse(traceId, businessCode, businessMessage, businessErr.Desc()))
		}
	}

	costSeconds := time.Since(reqTime).Seconds()

	// 请求日志打印
	srv.logger.RequestLog(appCtx.RequestContext(), &logger.RequestLogFormat{
		ClientIp:          ctx.ClientIP(),
		Method:            appCtx.Method(),
		Path:              appCtx.URI(),
		RequestProto:      ctx.Request.Proto,
		RequestReferer:    ctx.Request.Referer(),
		RequestUa:         ctx.Request.UserAgent(),
		RequestPostData:   ctx.Request.PostForm.Encode(),
		RequestBodyData:   string(appCtx.RawData()),
		RequestHeaderData: appCtx.Header(),
		HttpCode:          ctx.Writer.Status(),
		BusinessCode:      businessCode,
		BusinessMessage:   businessMessage,
		RequestTime:       reqTime,
		ResponseTime:      time.Now(),
		CostSeconds:       costSeconds,
	}, stackErr)
}
