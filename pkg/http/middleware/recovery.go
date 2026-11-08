// Package middleware 提供基于 HTTP 框架的中间件管理系统。
package middleware

import (
	"fmt"
	nethttp "net/http"
	"os"
	"runtime/debug"
	"time"

	"ult/config"
	"ult/errcode"
	"ult/pkg/logger"
	"ult/pkg/proposal"
	"ult/pkg/types"
	pkgtypes "ult/pkg/types"

	utilsMiddleware "github.com/devpety/go-utils/v3/middleware"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"go.uber.org/zap"
)

// RecoveryConfig Recovery 中间件配置。
type RecoveryConfig struct {
	// Enabled 是否启用异常恢复中间件
	Enabled bool

	// AlertNotify 告警通知处理函数
	// 处理 panic 和显式调用 Alert 的业务错误
	AlertNotify proposal.NotifyHandler

	// Config 应用配置（用于告警通知）
	Config *config.Config

	// PrintStack 是否打印堆栈信息
	PrintStack bool
}

// Recovery 异常恢复中间件。
// 捕获 panic 并进行恢复，记录错误日志，发送告警通知。
type Recovery struct {
	config *RecoveryConfig
	logger *logger.Logger
}

const panicStackKey = "ult.recovery_stack"

// NewRecovery 创建 Recovery 中间件实例。
//
// 参数:
//   - config: Recovery 配置
//   - logger: 日志记录器
//
// 返回:
//   - *Recovery: Recovery 中间件实例
func NewRecovery(config *RecoveryConfig, logger *logger.Logger) *Recovery {
	return &Recovery{
		config: config,
		logger: logger,
	}
}

// Name 返回中间件名称。
func (r *Recovery) Name() string {
	return types.RecoveryMiddlewareName
}

// Priority 返回中间件优先级。
// Recovery 中间件必须在最前执行，设置为最高优先级。
func (r *Recovery) Priority() utilsMiddleware.Priority {
	return utilsMiddleware.PriorityHighest
}

// Enabled 返回是否启用。
func (r *Recovery) Enabled() bool {
	return r.config.Enabled
}

// Dependencies 返回中间件依赖列表。
// Recovery 中间件无依赖，必须最先执行。
func (r *Recovery) Dependencies() []string {
	return []string{}
}

// Handler 返回中间件处理函数。
func (r *Recovery) Handler() HandlerFunc {
	return r.handler()
}

// handler 返回中间件处理函数。
// 捕获 panic 并进行恢复处理。
func (r *Recovery) handler() HandlerFunc {
	return func(ctx *gin.Context) {
		defer func() {
			// 即使日志或通知回调失败，也必须完成响应和资源清理。
			defer r.completeRequest(ctx)
			if err := recover(); err != nil {
				r.recoverPanic(ctx, err)
			}
		}()
		ctx.Next()
	}
}

// completeRequest 完成请求处理，确保响应和资源清理。
func (r *Recovery) completeRequest(ctx *gin.Context) {
	defer r.notifyAlert(ctx)
	defer func() {
		if recovered := recover(); recovered != nil {
			r.recoverPanic(ctx, recovered)
			r.writeErrorResponse(ctx)
		}
	}()
	CompleteRequest(ctx)
	r.writeErrorResponse(ctx)
}

// recoverPanic 在保留统一收尾的同时，将传输中断交回 net/http 处理。
func (r *Recovery) recoverPanic(ctx *gin.Context, recovered interface{}) {
	if recovered == nethttp.ErrAbortHandler {
		// 中断信号不能被统一响应覆盖，收尾回调仍需执行以回收请求上下文。
		ctx.Set(pkgtypes.ContextResponseAbortedNameKey, true)
		ctx.Abort()
		panic(recovered)
	}
	r.handleRecovery(ctx, recovered)
	if ctx.Writer.Written() {
		// 状态与部分正文已提交，正常返回会让截断的 chunked 响应被视为完整响应。
		panic(nethttp.ErrAbortHandler)
	}
}

// 没有 Request 回调或收尾异常时，仍为尚未提交的失败请求返回统一错误。
func (r *Recovery) writeErrorResponse(ctx *gin.Context) {
	if ctx.Writer.Written() || ctx.GetBool(pkgtypes.ContextResponseAbortedNameKey) {
		return
	}
	value, _ := ctx.Get(pkgtypes.ContextAbortErrorNameKey)
	businessErr, ok := value.(errcode.BusinessError)
	if !ok || businessErr == nil {
		return
	}
	traceID := recoveryTraceID(ctx)
	httpCode := businessErr.HTTPCode()
	if httpCode == 0 {
		httpCode = nethttp.StatusInternalServerError
	}
	ctx.JSON(httpCode, gin.H{"trace_id": traceID, "code": businessErr.BusinessCode(), "message": businessErr.Message(), "desc": businessErr.Desc()})
}

// panic 可能发生在 Request 初始化前，恢复边界仍需为日志、告警和响应确定同一个 ID。
func recoveryTraceID(ctx *gin.Context) string {
	traceID := ctx.GetString(pkgtypes.TraceIdName)
	if traceID == "" {
		traceID = ctx.GetHeader(pkgtypes.TraceIdName)
		if traceID == "" {
			traceID = uuid.NewString()
		}
		ctx.Set(pkgtypes.TraceIdName, traceID)
	}
	return traceID
}

// handleRecovery 处理 panic 恢复逻辑。
// 记录内部错误和堆栈，设置失败响应；告警在统一收尾阶段发送。
//
// 参数:
//   - ctx: Gin 上下文
//   - err: panic 错误信息
func (r *Recovery) handleRecovery(ctx *gin.Context, err interface{}) {
	// 先记录失败状态，防止日志或用户通知函数的异常把请求变成成功。
	ctx.Set(pkgtypes.ContextAbortErrorNameKey, errcode.New(errcode.ServerError).Alert())
	ctx.Status(nethttp.StatusInternalServerError)
	ctx.Abort()
	defer func() {
		if recovered := recover(); recovered != nil {
			fmt.Fprintf(os.Stderr, "Recovery 日志或通知处理失败: %v\n", recovered)
		}
	}()
	cause, ok := err.(error)
	if !ok {
		cause = fmt.Errorf("panic: %v", err)
	}
	ctx.Set(pkgtypes.ContextAbortErrorNameKey, errcode.New(errcode.ServerError).WithStackError(cause).Alert())
	// 获取堆栈信息
	var stack = string(debug.Stack())
	ctx.Set(panicStackKey, stack)
	traceID := recoveryTraceID(ctx)
	requestContext := &pkgtypes.RequestContext{}
	requestContext.WithTraceID(traceID)
	logContext := pkgtypes.NewRequestContext(ctx.Request.Context(), requestContext)

	// 记录错误日志
	r.logger.UseApp(logContext).Error(
		"got panic",
		zap.String("panic", fmt.Sprintf("%+v", err)),
		zap.String("stack", stack),
	)

	// 打印堆栈信息（如果配置允许）
	if r.config.PrintStack {
		fmt.Printf("Panic recovered: %v\nStack trace:\n%s\n", err, stack)
	}
}

// notifyAlert 是业务错误和 panic 共用的告警出口，通知失败不影响响应和资源回收。
func (r *Recovery) notifyAlert(ctx *gin.Context) {
	defer func() {
		if recovered := recover(); recovered != nil {
			fmt.Fprintf(os.Stderr, "告警通知处理失败: %v\n", recovered)
		}
	}()
	if r.config.AlertNotify == nil {
		return
	}
	value, _ := ctx.Get(pkgtypes.ContextAbortErrorNameKey)
	failure, ok := value.(errcode.BusinessError)
	if !ok || failure == nil || !failure.IsAlert() {
		return
	}
	message := &proposal.AlertMessage{
		TraceID:      recoveryTraceID(ctx),
		HOST:         ctx.Request.Host,
		URI:          ctx.Request.URL.RequestURI(),
		Method:       ctx.Request.Method,
		ErrorMessage: failure.Error(),
		ErrorStack:   ctx.GetString(panicStackKey),
		Timestamp:    time.Now(),
	}
	if message.ErrorStack == "" && failure.StackError() != nil {
		message.ErrorStack = fmt.Sprintf("%+v", failure.StackError())
	}
	if r.config.Config != nil {
		message.ProjectName = r.config.Config.App.Name
		message.Environment = r.config.Config.Environment
	}
	r.config.AlertNotify(message)
}

// DefaultRecoveryConfig 返回默认 Recovery 配置。
//
// 参数:
//   - cfg: 应用配置
//   - alertNotify: 告警通知处理函数（可选）
//
// 返回:
//   - *RecoveryConfig: 默认 Recovery 配置
func DefaultRecoveryConfig(cfg *config.Config, alertNotify proposal.NotifyHandler) *RecoveryConfig {
	return &RecoveryConfig{
		Enabled:     true,
		AlertNotify: alertNotify,
		Config:      cfg,
		PrintStack:  true,
	}
}

// NewDefaultRecovery 创建默认 Recovery 中间件。
// 提供便捷的创建方式，使用默认配置。
//
// 参数:
//   - cfg: 应用配置
//   - logger: 日志记录器
//   - alertNotify: 告警通知处理函数（可选）
//
// 返回:
//   - *Recovery: Recovery 中间件实例
func NewDefaultRecovery(cfg *config.Config, logger *logger.Logger, alertNotify proposal.NotifyHandler) *Recovery {
	return NewRecovery(DefaultRecoveryConfig(cfg, alertNotify), logger)
}
