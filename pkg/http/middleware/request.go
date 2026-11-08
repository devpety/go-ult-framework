// Package middleware 提供基于 HTTP 框架的中间件管理系统。
package middleware

import (
	"net/http"
	"time"

	"ult/errcode"
	"ult/pkg/types"
	pkgtypes "ult/pkg/types"

	utilsMiddleware "github.com/devpety/go-utils/v3/middleware"
	"github.com/devpety/go-utils/v3/validator"
	"github.com/gin-gonic/gin"
)

// RequestConfig Request 中间件配置。
type RequestConfig struct {
	// Enabled 是否启用请求处理中间件
	Enabled bool

	// Validator 数据验证器
	Validator validator.Validator

	// ContextInitializer Context 初始化函数
	// 该函数负责创建和初始化 Context，包括读取请求体、设置验证器等
	// 返回的 Context 应该已经完成初始化（调用过 init() 方法）
	ContextInitializer ContextInitializer

	// Response 响应处理函数
	// 该函数负责处理响应，包括错误响应、成功响应、日志记录等
	Response Response
}

// ContextInitializer Context 初始化函数类型。
// 该函数负责创建和初始化 Context，包括读取请求体、设置验证器等。
// 返回的 Context 应该已经完成初始化（调用过 init() 方法）。
type ContextInitializer func(ctx *gin.Context) (interface{}, error)

// ResponseHandler 响应处理函数类型。
// 该函数负责处理响应，包括错误响应、成功响应、日志记录等。
type Response func(reqTime time.Time, ctx *gin.Context)

// Request 请求处理中间件。
// 负责 Context 初始化、验证器设置和响应处理。
// 该中间件在 Recovery 和 CORS 之后、业务中间件之前执行，确保业务 Context 可用。
type Request struct {
	config *RequestConfig
}

// NewRequest 创建 Request 中间件实例。
//
// 参数:
//   - config: Request 配置
//
// 返回:
//   - *Request: Request 中间件实例
func NewRequest(config *RequestConfig) *Request {
	return &Request{
		config: config,
	}
}

// Name 返回中间件名称。
func (r *Request) Name() string {
	return types.RequestMiddlewareName
}

// Priority 返回中间件优先级。
// Request 必须在所有业务中间件之前执行，设置为高优先级。
// 但比 Recovery 稍低（Recovery 是 PriorityHighest=0，这里使用 PriorityHigh=1），
// 确保 Recovery 能捕获 Request 中的 panic。
func (r *Request) Priority() utilsMiddleware.Priority {
	return utilsMiddleware.PriorityHigh
}

// Enabled 返回是否启用。
func (r *Request) Enabled() bool {
	return r.config.Enabled
}

// Dependencies 返回中间件依赖列表。
// Request 中间件依赖 Recovery，确保 Recovery 能捕获 Request 中的 panic。
func (r *Request) Dependencies() []string {
	return []string{types.RecoveryMiddlewareName} // 依赖 Recovery 中间件
}

// Handler 返回中间件处理函数。
func (r *Request) Handler() HandlerFunc {
	return r.handler()
}

// handler 返回中间件处理函数。
// 初始化 Context、设置验证器、处理响应。
func (r *Request) handler() HandlerFunc {
	return func(ctx *gin.Context) {
		// 在初始化前登记收尾，初始化失败也走统一错误响应。
		ts := time.Now()
		ctx.Set(requestCompletionKey, &requestCompletion{response: func() {
			if r.config.Response != nil {
				r.config.Response(ts, ctx)
			}
		}})
		// 初始化 Context（通过配置的初始化函数）
		if r.config.ContextInitializer != nil {
			appCtx, err := r.config.ContextInitializer(ctx)
			if err != nil {
				if _, exists := ctx.Get(pkgtypes.ContextAbortErrorNameKey); !exists {
					ctx.Set(pkgtypes.ContextAbortErrorNameKey, errcode.New(errcode.ServerError).WithStackError(err))
				}
				status := http.StatusInternalServerError
				if value, exists := ctx.Get(pkgtypes.ContextAbortErrorNameKey); exists {
					if failure, ok := value.(errcode.BusinessError); ok && failure != nil && failure.HTTPCode() > 0 {
						status = failure.HTTPCode()
					}
				}
				ctx.Status(status)
				ctx.Abort()
				return
			}

			// 存储 Context 到 gin.Context（appCtx 是 interface{} 类型）
			ctx.Set(pkgtypes.CoreContextNameKey, appCtx)
		}

		ctx.Next()
	}
}

// DefaultRequestConfig 返回默认 Request 配置。
//
// 参数:
//   - validator: 数据验证器
//   - contextInitializer: Context 初始化函数
//   - response: 响应处理函数
//
// 返回:
//   - *RequestConfig: 默认 Request 配置
func DefaultRequestConfig(
	validator validator.Validator,
	contextInitializer ContextInitializer,
	response Response,
) *RequestConfig {
	return &RequestConfig{
		Enabled:            true,
		Validator:          validator,
		ContextInitializer: contextInitializer,
		Response:           response,
	}
}

// NewDefaultRequest 创建默认 Request 中间件。
// 提供便捷的创建方式，使用默认配置。
//
// 参数:
//   - validator: 数据验证器
//   - contextInitializer: Context 初始化函数
//   - response: 响应处理函数
//
// 返回:
//   - *Request: Request 中间件实例
func NewDefaultRequest(
	validator validator.Validator,
	contextInitializer ContextInitializer,
	response Response,
) *Request {
	return NewRequest(DefaultRequestConfig(validator, contextInitializer, response))
}
