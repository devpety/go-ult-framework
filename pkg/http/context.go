// Package http 提供 HTTP 服务器实现，基于 Gin 框架封装。
package http

import (
	"bytes"
	stdCtx "context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"reflect"
	"sync"
	"ult/errcode"
	pkgtypes "ult/pkg/types"

	"github.com/devpety/go-utils/v3/validator"
	"github.com/gin-gonic/gin"
	"github.com/gin-gonic/gin/binding"
	govalidator "github.com/go-playground/validator/v10"
	"github.com/google/uuid"
)

// HandlerFunc 定义本包使用的处理函数类型。
// 接受自定义 Context 接口而非直接使用 Gin.Context。
type HandlerFunc func(ctx Context)

// Context 接口验证。
var _ Context = (*context)(nil)

// Context 定义 HTTP 请求上下文接口。
// 封装 Gin.Context 并提供额外功能：
// - 请求绑定（query、form、JSON、URI）
// - 响应处理（payload、error）
// - 请求信息访问
// - 分布式追踪的 TraceID 管理
type Context interface {
	// ShouldBindQuery 反序列化查询字符串参数。
	// 在结构体字段中使用 `form:"xxx"` 标签（不是 `query`）。
	ShouldBindQuery(obj interface{}) error

	// ShouldBindPostForm 反序列化 POST 表单数据（忽略查询字符串）。
	// 在结构体字段中使用 `form:"xxx"` 标签。
	ShouldBindPostForm(obj interface{}) error

	// ShouldBindForm 反序列化查询字符串和 POST 表单。
	// 当同一字段同时存在时，POST 表单优先。
	// 在结构体字段中使用 `form:"xxx"` 标签。
	ShouldBindForm(obj interface{}) error

	// ShouldBindJSON 反序列化 JSON 请求体。
	// 在结构体字段中使用 `json:"xxx"` 标签。
	ShouldBindJSON(obj interface{}) error

	// ShouldBindURI 反序列化路径参数（如 /user/:name）。
	// 在结构体字段中使用 `uri:"xxx"` 标签。
	ShouldBindURI(obj interface{}) error

	// Redirect 执行 HTTP 重定向。
	Redirect(code int, location string)

	// Param 通过键名返回路径参数值。
	Param(key string) string

	// TraceID 返回分布式追踪的 TraceID。
	// 如果不存在，则生成新的 UUID 并存储。
	TraceID() string

	// Validate 仅校验已绑定的结构体，不读取或覆盖请求数据。
	Validate(req interface{}) error

	// BindAndValidateJSON 绑定 JSON 并校验，失败时设置统一错误并中止请求。
	BindAndValidateJSON(req interface{}) error

	// BindAndValidateForm 绑定表单并校验，失败时设置统一错误并中止请求。
	BindAndValidateForm(req interface{}) error

	// WithValidator 设置自定义校验器。
	WithValidator(v validator.Validator)

	// WithAbortError 设置错误以中止请求。
	// 该错误将用于响应处理。
	WithAbortError(err errcode.BusinessError)
	// GetAbortError 获取中止错误。
	// 用于响应处理中间件获取错误信息。
	GetAbortError() errcode.BusinessError

	// WithPayload 设置成功响应的数据负载。
	WithPayload(payload interface{})

	// WithStatus 设置尚未提交的成功响应状态码，允许 200 到 299。
	WithStatus(code int)

	// GetPayload 获取响应数据负载。
	// 用于响应处理中间件获取数据。
	GetPayload() interface{}

	// Header 返回所有请求头的只读引用。
	// 注意：返回的是原始请求头的引用，不应修改。
	// 性能优：无内存分配，直接返回引用。
	Header() http.Header
	// CloneHeaders 克隆所有请求头，返回完整副本。
	// 返回的副本可以安全修改而不影响原始请求头。
	// 性能说明：每次调用都会创建完整的副本，仅在需要修改时使用。
	CloneHeaders() http.Header
	// GetHeader 通过键名返回特定的请求头值。
	GetHeader(key string) string
	// SetHeader 设置响应头。
	SetHeader(key, value string)

	// RequestInputParams 返回所有查询和表单参数。
	RequestInputParams() url.Values
	// RequestPostFormParams 仅返回 POST 表单参数。
	RequestPostFormParams() url.Values
	// Request 返回底层的 http.Request 对象。
	Request() *http.Request
	// RawData 返回原始请求体字节。
	RawData() []byte
	// Method 返回 HTTP 方法。
	Method() string
	// Host 返回请求主机。
	Host() string
	// Path 返回不带查询字符串的请求路径。
	Path() string
	// URI 返回未转义的请求 URI。
	URI() string
	// RequestContext 返回带有 TraceID 的请求上下文。
	// 当客户端关闭连接时，该上下文会被取消。
	RequestContext() stdCtx.Context

	// ResponseWriter 返回 Gin 响应写入器。
	ResponseWriter() gin.ResponseWriter

	// GinContext 返回底层的 Gin.Context。
	// 用于需要直接操作 Gin 上下文的场景（如第三方中间件）。
	GinContext() *gin.Context
}

// context 是 Context 接口的内部实现。
// 封装 gin.Context 并存储额外的请求/响应数据。
type context struct {
	ctx            *gin.Context
	reqContext     stdCtx.Context // 缓存的 RequestContext
	traceIDOnce    sync.Once      // 确保 TraceID 只生成一次
	reqContextOnce sync.Once      // 确保 RequestContext 只创建一次
}

// reset 重置上下文对象，清空所有字段。
// 在归还到 Pool 前调用，确保下次使用时数据干净。
func (c *context) reset() {
	c.ctx = nil
	c.reqContext = nil             // 清空缓存的 RequestContext
	c.traceIDOnce = sync.Once{}    // 重置 sync.Once 以便下次使用
	c.reqContextOnce = sync.Once{} // 重置 sync.Once 以便下次使用
}

// init 初始化上下文，读取并存储原始请求体。
// 这允许在请求处理过程中多次读取请求体。
// 如果读取请求体失败，返回错误，调用者应该处理这个错误。
func (c *context) init(maxBodyBytes int64) error {
	if c.ctx.Request.Body != nil {
		c.ctx.Request.Body = http.MaxBytesReader(c.ctx.Writer, c.ctx.Request.Body, maxBodyBytes)
	}
	body, err := c.ctx.GetRawData()
	if err != nil {
		code := errcode.ServerError
		var sizeError *http.MaxBytesError
		var timeoutError net.Error
		switch {
		case errors.As(err, &sizeError):
			code = errcode.RequestBodyTooLargeError
		case errors.Is(err, stdCtx.DeadlineExceeded) || (errors.As(err, &timeoutError) && timeoutError.Timeout()):
			code = errcode.RequestTimeoutError
		}
		c.WithAbortError(errcode.New(code).WithStackError(err))
		return err
	}

	c.ctx.Set(pkgtypes.ContextBodyNameKey, body)
	c.ctx.Request.Body = io.NopCloser(bytes.NewBuffer(body))
	return nil
}

// ShouldBindQuery 将查询字符串参数绑定到结构体。
func (c *context) ShouldBindQuery(obj interface{}) error {
	return c.ctx.ShouldBindWith(obj, binding.Query)
}

// ShouldBindPostForm 将 POST 表单数据绑定到结构体。
func (c *context) ShouldBindPostForm(obj interface{}) error {
	return c.ctx.ShouldBindWith(obj, binding.FormPost)
}

// ShouldBindForm 将查询字符串和 POST 表单绑定到结构体。
func (c *context) ShouldBindForm(obj interface{}) error {
	return c.ctx.ShouldBindWith(obj, binding.Form)
}

// ShouldBindJSON 将 JSON 请求体绑定到结构体。
func (c *context) ShouldBindJSON(obj interface{}) error {
	return c.ctx.ShouldBindWith(obj, binding.JSON)
}

// ShouldBindURI 将 URI 路径参数绑定到结构体。
func (c *context) ShouldBindURI(obj interface{}) error {
	return c.ctx.ShouldBindUri(obj)
}

// Param 返回 URI 路径参数的值。
func (c *context) Param(key string) string {
	return c.ctx.Param(key)
}

// Redirect 执行 HTTP 重定向到指定位置。
func (c *context) Redirect(code int, location string) {
	c.ctx.Redirect(code, location)
}

// TraceID 返回分布式追踪的 TraceID。
// 首先检查上下文或请求头中是否存在 TraceID。
// 如果未找到，则生成新的 UUID 并存储。
// 使用 sync.Once 确保在并发场景下只生成一次。
func (c *context) TraceID() string {
	// 使用 sync.Once 确保只生成一次 TraceID
	c.traceIDOnce.Do(func() {
		// 先检查是否已存在
		if traceId, ok := c.ctx.Get(pkgtypes.TraceIdName); ok {
			if tid, ok := traceId.(string); ok && len(tid) > 0 {
				return
			}
		}

		// 检查请求头
		var headerTraceId = c.GetHeader(pkgtypes.TraceIdName)
		if len(headerTraceId) <= 0 {
			headerTraceId = uuid.New().String()
		}

		c.ctx.Set(pkgtypes.TraceIdName, headerTraceId)
	})

	// 从上下文中获取 TraceID
	traceId, ok := c.ctx.Get(pkgtypes.TraceIdName)
	if !ok {
		return ""
	}
	tid, ok := traceId.(string)
	if !ok {
		return ""
	}
	return tid
}

// Validate 仅校验结构体，绑定方式由调用方显式选择。
func (c *context) Validate(req interface{}) error {
	validate, ok := c.ctx.Get(pkgtypes.ContextValidatorNameKey)
	if !ok {
		businessErr := errcode.New(errcode.ServerError).WithStackError(errors.New("请求验证器未初始化"))
		c.WithAbortError(businessErr)
		return businessErr
	}

	validatorInst, ok := validate.(validator.Validator)
	if !ok || validatorInst == nil {
		businessErr := errcode.New(errcode.ServerError).WithStackError(errors.New("请求验证器类型无效"))
		c.WithAbortError(businessErr)
		return businessErr
	}

	if err := validatorInst.Validate(req); err != nil {
		var invalid *govalidator.InvalidValidationError
		var businessErr errcode.BusinessError
		if errors.As(err, &invalid) {
			businessErr = errcode.New(errcode.ServerError).WithStackError(err)
		} else {
			businessErr = errcode.New(errcode.ParamValidateError).WithDesc(err.Error())
		}
		c.WithAbortError(businessErr)
		return businessErr
	}

	return nil
}

// BindAndValidateJSON 将 JSON 绑定到非 nil 结构体指针并校验。
func (c *context) BindAndValidateJSON(req interface{}) error {
	return c.bindAndValidate(req, binding.JSON)
}

// BindAndValidateForm 将查询参数和表单绑定到非 nil 结构体指针后校验。
func (c *context) BindAndValidateForm(req interface{}) error {
	return c.bindAndValidate(req, binding.Form)
}

// bindAndValidate 绑定并校验请求参数。
func (c *context) bindAndValidate(req interface{}, binder binding.Binding) error {
	value := reflect.ValueOf(req)
	if value.Kind() != reflect.Ptr || value.IsNil() || value.Elem().Kind() != reflect.Struct {
		businessErr := errcode.New(errcode.ServerError).WithStackError(fmt.Errorf("绑定目标必须为非 nil 的结构体指针，实际为 %T", req))
		c.WithAbortError(businessErr)
		return businessErr
	}
	if err := c.ctx.ShouldBindWith(req, binder); err != nil {
		var invalidUnmarshal *json.InvalidUnmarshalError
		var invalidValidation *govalidator.InvalidValidationError
		var validationErrors govalidator.ValidationErrors
		var businessErr errcode.BusinessError
		switch {
		case errors.As(err, &invalidUnmarshal), errors.As(err, &invalidValidation):
			businessErr = errcode.New(errcode.ServerError).WithStackError(err)
		case errors.As(err, &validationErrors):
			businessErr = errcode.New(errcode.ParamValidateError).WithStackError(err).WithDesc(err.Error())
		default:
			businessErr = errcode.New(errcode.ParamBindError).WithStackError(err)
		}
		c.WithAbortError(businessErr)
		return businessErr
	}
	return c.Validate(req)
}

// WithValidator 设置请求验证的验证器实例。
func (c *context) WithValidator(v validator.Validator) {
	c.ctx.Set(pkgtypes.ContextValidatorNameKey, v)
}

// WithAbortError 设置业务错误并中止请求。
// HTTP 状态码由错误的 HTTPCode 方法决定。
func (c *context) WithAbortError(err errcode.BusinessError) {
	if err != nil {
		httpCode := err.HTTPCode()
		if httpCode == 0 {
			httpCode = http.StatusInternalServerError
		}

		c.ctx.Set(pkgtypes.ContextAbortErrorNameKey, err)
		c.ctx.Status(httpCode)
		c.ctx.Abort()
	}
}

// GetAbortError 获取中止错误。
// 用于响应处理中间件获取错误信息。
func (c *context) GetAbortError() errcode.BusinessError {
	err, ok := c.ctx.Get(pkgtypes.ContextAbortErrorNameKey)
	if !ok {
		return nil
	}
	businessErr, ok := err.(errcode.BusinessError)
	if !ok {
		return nil
	}
	return businessErr
}

// WithPayload 设置成功响应的数据负载。
func (c *context) WithPayload(payload interface{}) {
	c.ctx.Set(pkgtypes.ContextPayloadNameKey, payload)
}

// WithStatus 设置成功响应状态码；调用方应在写出响应前完成设置。
func (c *context) WithStatus(code int) {
	if code < http.StatusOK || code >= http.StatusMultipleChoices {
		panic(fmt.Sprintf("成功响应状态码必须在 200 到 299 之间，实际为 %d", code))
	}
	if c.ctx.Writer.Written() {
		panic("响应已提交，不能修改状态码")
	}
	c.ctx.Status(code)
}

// GetPayload 获取响应数据负载。
// 用于响应处理中间件获取数据。
func (c *context) GetPayload() interface{} {
	if payload, ok := c.ctx.Get(pkgtypes.ContextPayloadNameKey); ok {
		return payload
	}
	return nil
}

// Header 返回所有请求头的只读引用。
// 注意：返回的是原始请求头的引用，不应修改。
// 性能优：无内存分配，直接返回引用。
// 如果需要修改请求头，请使用 CloneHeaders() 方法。
func (c *context) Header() http.Header {
	return c.ctx.Request.Header
}

// CloneHeaders 克隆所有请求头，返回完整副本。
// 返回的副本可以安全修改而不影响原始请求头。
// 性能说明：每次调用都会创建完整的副本，仅在需要修改时使用。
// 如果只需要读取请求头，建议使用 Header() 方法。
func (c *context) CloneHeaders() http.Header {
	header := c.ctx.Request.Header
	clone := make(http.Header, len(header))
	for k, v := range header {
		value := make([]string, len(v))
		copy(value, v)

		clone[k] = value
	}

	return clone
}

// GetHeader 通过键名返回特定的请求头值。
func (c *context) GetHeader(key string) string {
	return c.ctx.GetHeader(key)
}

// SetHeader 设置响应头。
func (c *context) SetHeader(key, value string) {
	c.ctx.Header(key, value)
}

// RequestInputParams 返回所有查询和表单参数的组合。
func (c *context) RequestInputParams() url.Values {
	_ = c.ctx.Request.ParseForm()
	return c.ctx.Request.Form
}

// RequestPostFormParams 仅返回 POST 表单参数。
func (c *context) RequestPostFormParams() url.Values {
	_ = c.ctx.Request.ParseForm()
	return c.ctx.Request.PostForm
}

// Request 返回底层的 http.Request 对象。
func (c *context) Request() *http.Request {
	return c.ctx.Request
}

// RawData 返回原始请求体字节的副本。
// 返回副本是为了防止外部修改影响存储在上下文中的原始数据。
func (c *context) RawData() []byte {
	body, ok := c.ctx.Get(pkgtypes.ContextBodyNameKey)
	if !ok {
		return nil
	}

	bodyBytes, ok := body.([]byte)
	if !ok {
		return nil
	}

	// 返回副本，避免外部修改影响原始数据
	copied := make([]byte, len(bodyBytes))
	copy(copied, bodyBytes)
	return copied
}

// Method 返回 HTTP 方法（GET、POST 等）。
func (c *context) Method() string {
	return c.ctx.Request.Method
}

// Host 返回请求主机。
func (c *context) Host() string {
	return c.ctx.Request.Host
}

// Path 返回不带查询字符串的请求路径。
func (c *context) Path() string {
	return c.ctx.Request.URL.Path
}

// URI 返回带查询字符串的未转义请求 URI。
func (c *context) URI() string {
	uri, _ := url.QueryUnescape(c.ctx.Request.URL.RequestURI())
	return uri
}

// RequestContext 返回带有 TraceID 的请求上下文，用于分布式追踪。
// 当客户端关闭连接时，该上下文会被取消。
// 使用 sync.Once 缓存 RequestContext，多次调用只创建一次。
func (c *context) RequestContext() stdCtx.Context {
	c.reqContextOnce.Do(func() {
		reqContext := new(pkgtypes.RequestContext)
		reqContext.WithTraceID(c.TraceID())
		c.reqContext = pkgtypes.NewRequestContext(c.ctx.Request.Context(), reqContext)
	})
	return c.reqContext
}

// ResponseWriter 返回 Gin 响应写入器，用于直接操作响应。
func (c *context) ResponseWriter() gin.ResponseWriter {
	return c.ctx.Writer
}

// GinContext 返回底层的 Gin.Context。
// 用于需要直接操作 Gin 上下文的场景（如第三方中间件）。
func (c *context) GinContext() *gin.Context {
	return c.ctx
}

// contextPool 是用于复用上下文对象的 sync.Pool。
// 通过减少内存分配来提高性能。
var contextPool = &sync.Pool{
	New: func() interface{} {
		return new(context)
	},
}

// newContext 从池中创建或获取上下文对象，并完成初始化。
// 用额外功能封装 Gin 上下文。
// 如果初始化失败（如读取请求体失败），返回错误。
func newContext(ctx *gin.Context) (Context, error) {
	return newContextWithLimit(ctx, defaultMaxBodyBytes)
}

// newContextWithLimit 从池中创建或获取上下文对象，并完成初始化。
// 允许指定最大请求体字节数。
// 如果初始化失败（如读取请求体失败），返回错误。
func newContextWithLimit(ctx *gin.Context, maxBodyBytes int64) (Context, error) {
	context := contextPool.Get().(*context)
	context.ctx = ctx
	initialized := false
	defer func() {
		if !initialized {
			// 初始化失败立即回收
			recoveryContext(context)
		}
	}()
	if err := context.init(maxBodyBytes); err != nil {
		// init() 方法已经设置了错误并中止了请求
		// 这里返回 nil 和错误，调用者应该处理这个错误
		return nil, err
	}
	initialized = true
	return context, nil
}

// recoveryContext 使用后将上下文归还到池中。
// 调用 reset 方法清空所有字段，防止内存泄漏。
// 注意：如果未来 context 结构体添加新字段，需在 reset 方法中添加清理逻辑。
func recoveryContext(ctx Context) {
	c, ok := ctx.(*context)
	if !ok {
		return
	}
	c.reset()
	contextPool.Put(c)
}
