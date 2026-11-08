// Package app 提供应用管理功能。
// 定义应用生命周期管理、服务器注册和上下文管理等功能。
package app

import (
	stdCtx "context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"
	"ult/config"
	"ult/pkg/logger"
	"ult/pkg/types"

	"go.uber.org/zap"
)

var _ AppInterface = (*App)(nil)

// AppInterface 应用接口，定义应用的基本属性和操作。
type AppInterface interface {
	ID() string              // 获取应用 ID
	Name() string            // 获取应用名称
	Version() string         // 获取应用版本
	Environment() string     // 获取运行环境
	Context() stdCtx.Context // 获取应用上下文
	Run() error              // 运行应用
}

// App 应用实例，管理服务器生命周期和应用配置。
type App struct {
	servers         []Server       // 服务器列表
	context         stdCtx.Context // 应用上下文，取消时触发停机
	cancel          []func()       // 服务停止后执行的取消函数
	shutdownTimeout time.Duration  // 所有服务停止和退出的总时限
	config          *config.Config // 应用配置
	logger          *logger.Logger // 日志记录器
}

// AppOption 应用选项函数类型。
type AppOption func(*App)

// WithAppContext 设置应用上下文选项。
// 取消该上下文会触发停机；传给服务器的运行上下文在服务排空后取消。
//
// 参数:
//   - ctx: 应用上下文
//
// 返回:
//   - AppOption: 应用选项函数
func WithAppContext(ctx stdCtx.Context) AppOption {
	return func(app *App) {
		app.context = ctx
	}
}

// WithAppCancel 设置应用取消函数选项，在服务停止和运行上下文取消后执行。
//
// 参数:
//   - fn: 取消函数
//
// 返回:
//   - AppOption: 应用选项函数
func WithAppCancel(fn func()) AppOption {
	return func(app *App) {
		app.cancel = append(app.cancel, fn)
	}
}

// WithAppServer 设置应用服务器选项。
//
// 参数:
//   - srv: 服务器列表
//
// 返回:
//   - AppOption: 应用选项函数
func WithAppServer(srv ...Server) AppOption {
	return func(app *App) {
		app.servers = srv
	}
}

// WithAppShutdownTimeout 设置所有服务停止和退出的总时限，默认为 30 秒。
// 非正值表示不等待在途请求，立即进入超时关闭流程。
func WithAppShutdownTimeout(timeout time.Duration) AppOption {
	return func(app *App) {
		app.shutdownTimeout = timeout
	}
}

// NewApp 创建新的应用实例。
// 初始化应用配置、日志记录器和关闭超时。
//
// 参数:
//   - config: 应用配置
//   - logger: 日志记录器
//   - opts: 应用选项列表
//
// 返回:
//   - *App: 应用实例
func NewApp(config *config.Config, logger *logger.Logger, opts ...AppOption) *App {
	var app = &App{
		config:          config,
		logger:          logger,
		context:         stdCtx.Background(),
		shutdownTimeout: 30 * time.Second,
	}

	for _, opt := range opts {
		opt(app)
	}

	return app
}

// ID 获取应用 ID。
//
// 返回:
//   - string: 应用 ID
func (app *App) ID() string {
	return app.config.App.ID
}

// Name 获取应用名称。
//
// 返回:
//   - string: 应用名称
func (app *App) Name() string {
	return app.config.App.Name
}

// Version 获取应用版本。
//
// 返回:
//   - string: 应用版本
func (app *App) Version() string {
	return app.config.App.Version
}

// Environment 获取运行环境。
//
// 返回:
//   - string: 运行环境（dev、prod 等）
func (app *App) Environment() string {
	return app.config.Environment
}

// Context 获取应用上下文。
//
// 返回:
//   - stdCtx.Context: 应用上下文
func (app *App) Context() stdCtx.Context {
	return app.context
}

// Run 运行应用。
// 启动所有注册的服务器，并设置优雅关闭处理。
// 优雅关闭流程：
// 1. 收到关闭信号、上层上下文取消、服务运行错误或所有服务正常退出
// 2. 使用独立的限时上下文停止服务器，等待在途请求完成
// 3. 取消运行上下文，等待服务启动协程退出
// 4. 执行应用取消函数，返回启动或关闭错误
//
// 返回:
//   - error: 运行错误
func (app *App) Run() error {
	signalCtx, stopSignals := signal.NotifyContext(app.context, os.Interrupt, syscall.SIGTERM, syscall.SIGHUP, syscall.SIGQUIT)
	defer stopSignals()

	// 停机触发不能直接取消请求；保留上层值，将取消推迟到服务排空后。
	runCtx, cancelRun := stdCtx.WithCancel(stdCtx.WithoutCancel(app.context))
	defer func() {
		cancelRun()
		for _, cancel := range app.cancel {
			cancel()
		}
	}()
	ctx := NewAppContext(runCtx, app)
	if signalCtx.Err() != nil {
		return nil
	}

	type serverResult struct {
		serverType string
		err        error
	}
	// 每个服务仅发送一次结果，即使关闭超时返回也不会阻塞发送方。
	results := make(chan serverResult, len(app.servers))
	remaining := len(app.servers)
	serverError := func(result serverResult) error {
		app.logger.UseApp(ctx).Error(fmt.Sprintf("%s 服务启动错误", result.serverType), zap.Error(result.err))
		return fmt.Errorf("%s 服务启动错误: %w", result.serverType, result.err)
	}
	for _, server := range app.servers {
		srvType := server.ServerType()
		server.StartBefore()
		go func() {
			results <- serverResult{serverType: srvType, err: server.Start(ctx)}
		}()
		server.StartAfter()
	}

	var runErr error
waitForStop:
	for remaining > 0 {
		select {
		case <-signalCtx.Done():
			break waitForStop
		case result := <-results:
			remaining--
			if result.err != nil {
				runErr = serverError(result)
				break waitForStop
			}
		}
	}
	stopSignals()

	shutdownCtx, cancelShutdown := stdCtx.WithTimeout(stdCtx.WithoutCancel(ctx), app.shutdownTimeout)
	defer cancelShutdown()
	for _, server := range app.servers {
		srvType := server.ServerType()
		server.CancelBefore()
		if err := server.Stop(shutdownCtx); err != nil {
			app.logger.UseApp(ctx).Error(fmt.Sprintf("%s 服务关闭错误", srvType), zap.Error(err))
			runErr = errors.Join(runErr, fmt.Errorf("%s 服务关闭错误: %w", srvType, err))
		} else {
			app.logger.UseApp(ctx).Info(fmt.Sprintf("%s 服务已关闭成功", srvType))
		}
		server.CancelAfter()
	}
	cancelRun()

	for remaining > 0 {
		select {
		case result := <-results:
			remaining--
			if result.err != nil && !errors.Is(result.err, stdCtx.Canceled) {
				runErr = errors.Join(runErr, serverError(result))
			}
		case <-shutdownCtx.Done():
			return errors.Join(runErr, fmt.Errorf("等待服务退出超时: %w", shutdownCtx.Err()))
		}
	}
	return runErr
}

// appKey 应用上下文键。
type appKey struct{}

// NewAppContext 创建带有应用实例的上下文。
//
// 参数:
//   - ctx: 基础上下文
//   - s: 应用实例
//
// 返回:
//   - stdCtx.Context: 带有应用实例的上下文
func NewAppContext(ctx stdCtx.Context, s AppInterface) stdCtx.Context {
	return stdCtx.WithValue(ctx, appKey{}, s)
}

// FromAppContext 从上下文中获取应用实例。
//
// 参数:
//   - ctx: 上下文
//
// 返回:
//   - s: 应用实例
//   - ok: 是否存在
func FromAppContext(ctx stdCtx.Context) (s AppInterface, ok bool) {
	s, ok = ctx.Value(appKey{}).(AppInterface)
	return
}

// NewAppContextWithTraceId 创建带有应用实例和 TraceID 的上下文。
//
// 参数:
//   - ctx: 基础上下文
//   - app: 应用实例
//   - traceId: 链路追踪 ID
//
// 返回:
//   - stdCtx.Context: 带有应用实例和 TraceID 的上下文
func NewAppContextWithTraceId(ctx stdCtx.Context, app AppInterface, traceId string) stdCtx.Context {
	ctx = NewAppContext(ctx, app)
	var reqContext = new(types.RequestContext)
	reqContext.WithTraceID(traceId)
	return types.NewRequestContext(ctx, reqContext)
}
