package http

import (
	"fmt"
	"net"
	nethttp "net/http"
	"strconv"
	"time"
	"ult/config"
	pkgmiddleware "ult/pkg/http/middleware"
	"ult/pkg/logger"
	"ult/pkg/types"

	"github.com/devpety/go-utils/v3/http"
	"github.com/devpety/go-utils/v3/middleware"
	"github.com/devpety/go-utils/v3/server/system"
	"github.com/devpety/go-utils/v3/validator"
	"github.com/gin-gonic/gin"
)

// NewServer 完成中间件和路由配置后返回可直接使用的服务器。
// 已知配置错误以及中间件、路由配置中的 panic 返回为 error，不返回半成品实例。
func NewServer(cfg *config.Config, log *logger.Logger, srvOpts []http.ServerOption, opts ...Option) (*HTTPServer, error) {
	options, err := serverOptions(opts...)
	if err != nil {
		return nil, err
	}
	if cfg == nil {
		return nil, fmt.Errorf("HTTP 配置不能为空")
	}
	if log == nil || log.Logger == nil {
		return nil, fmt.Errorf("HTTP 日志记录器不能为空")
	}

	gin.SetMode(gin.ReleaseMode)
	srv := &HTTPServer{engine: gin.New(), config: cfg, logger: log, timeout: options.timeout}
	if err := srv.buildHandler(options); err != nil {
		return nil, err
	}

	// 配置成功后才创建传输服务器，构建失败不会占用监听端口。
	srvOpts = append([]http.ServerOption{http.WithServerAddress(net.JoinHostPort(cfg.Server.Http.Host, strconv.Itoa(cfg.Server.Http.Port)))}, srvOpts...)
	srv.server = newTransportServer(options, srvOpts)
	srv.server.Handler = srv
	if options.openBrowser != "" {
		_ = system.OpenBrowser(options.openBrowser)
	}
	return srv, nil
}

// newTransportServer 创建 HTTP 传输服务器，根据配置设置超时时间。
func newTransportServer(options *option, srvOpts []http.ServerOption) *http.Server {
	transport := &nethttp.Server{
		ReadHeaderTimeout: options.readHeaderTimeout, ReadTimeout: options.readTimeout,
		WriteTimeout: options.writeTimeout, IdleTimeout: options.idleTimeout,
	}
	return http.NewServer(transport, srvOpts...)
}

// buildHandler 构建 HTTP 处理链，包括全局中间件和路由处理函数。
// 处理链在 Gin 中注册，确保在请求处理前安装所有中间件。
func (srv *HTTPServer) buildHandler(options *option) (err error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			if cause, ok := recovered.(error); ok {
				err = fmt.Errorf("HTTP 处理链构建失败: %w", cause)
			} else {
				err = fmt.Errorf("HTTP 处理链构建失败: %v", recovered)
			}
		}
	}()

	requestValidator := validator.New(
		validator.WithLocale(srv.config.Validator.Locale),
		validator.WithTagname(srv.config.Validator.Tagname))
	manager := pkgmiddleware.NewManager()
	middlewares := []pkgmiddleware.Middleware{
		pkgmiddleware.NewDefaultRecovery(srv.config, srv.logger, options.alertNotify),
		srv.createRequest(requestValidator, options.maxBodyBytes),
	}
	middlewares = append(middlewares, options.middlewares...)
	for _, item := range middlewares {
		count := manager.Count()
		if err := manager.Use(item); err != nil {
			return err
		}
		if manager.Count() > count && item.Priority() == middleware.PriorityHighest &&
			item.Name() != types.RecoveryMiddlewareName && item.Name() != types.CorsMiddlewareName {
			srv.logger.Warn(fmt.Sprintf("最高优先级 PriorityHighest 应用于 Recovery 异常恢复中间件, 当前中间件为 `%s`, 请注意编排顺序, 必须保证 Recovery 为第一位中间件使用", item.Name()))
		}
	}
	handlers, err := manager.Build()
	if err != nil {
		return err
	}
	// Gin 注册路由时复制处理链，必须先安装完整的全局中间件。
	srv.engine.Use(handlers...)
	registration := &routeRegistration{}
	defer registration.close()
	root := &router{group: &srv.engine.RouterGroup, registration: registration}
	for _, register := range options.routes {
		register(root)
	}
	if options.pprof {
		registration.configure(func() { registerPProf(srv.engine, options.pprofEnvironment) })
	}
	return nil
}

// createRequest 在构建时捕获校验器和请求限制，运行时不再依赖构建选项。
func (srv *HTTPServer) createRequest(requestValidator validator.Validator, maxBodyBytes int64) pkgmiddleware.Middleware {
	// 创建 Context 初始化函数
	// 该函数负责创建 Context 并完成初始化
	contextInitializer := func(ctx *gin.Context) (interface{}, error) {
		appCtx, err := newContextWithLimit(ctx, maxBodyBytes)
		if err != nil {
			// newContext 已经设置了错误并中止了请求
			// 这里返回 nil 和错误，中间件会根据返回的错误进行相应的处理
			return nil, err
		}
		appCtx.WithValidator(requestValidator)
		return appCtx, nil
	}

	// 创建响应处理函数
	response := func(reqTime time.Time, ctx *gin.Context) {
		if appCtx, ok := ctx.Value(types.CoreContextNameKey).(Context); ok && appCtx != nil {
			defer func() {
				ctx.Set(types.CoreContextNameKey, nil)

				// 成功使用上下文后回收
				recoveryContext(appCtx)
			}()
		}
		srv.handlerResponse(reqTime, ctx)
	}

	// 创建 Request 中间件
	return pkgmiddleware.NewDefaultRequest(
		requestValidator,
		contextInitializer,
		response,
	)
}
