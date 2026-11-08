// Package errcode 提供统一错误码定义和管理。
// 支持多语言错误消息、HTTP 状态码映射、错误堆栈追踪和告警通知。
package errcode

import (
	"github.com/devpety/go-utils/v3/errors"
)

// BusinessError 接口验证。
var _ BusinessError = (*businessError)(nil)

// BusinessError 业务错误接口。
// 定义统一的错误处理规范，包含 HTTP 状态码、业务错误码、消息、描述等。
// 实现了 error 接口，可以作为标准错误使用。
// 修饰方法返回独立副本，调用者需要使用返回值。
type BusinessError interface {
	error                                   // 实现 error 接口
	Unwrap() error                          // 获取内部错误原因
	WithStackError(err error) BusinessError // 返回附加堆栈的副本
	StackError() error                      // 获取堆栈错误
	BusinessCode() int                      // 获取业务错误码
	HTTPCode() int                          // 获取 HTTP 状态码
	Message() string                        // 获取错误消息
	Desc() string                           // 获取错误描述
	Alert() BusinessError                   // 返回带告警标记的副本
	WithDesc(desc string) BusinessError     // 返回附加描述的副本
	IsAlert() bool                          // 是否需要告警
}

// businessError 业务错误实现结构体。
type businessError struct {
	httpCode        int    // HTTP 状态码
	businessCode    int    // 业务错误码
	message         string // 错误消息
	registryMessage bool   // 注册表错误按当前语言读取消息，显式消息保持固定
	desc            string // 显式提供的对外错误描述
	stackError      error  // 堆栈错误
	isAlert         bool   // 是否告警
}

// NewError 创建新的业务错误实例。
//
// 参数:
//   - httpCode: HTTP 状态码
//   - businessCode: 业务错误码
//   - message: 错误消息
//
// 返回:
//   - BusinessError: 业务错误接口
func NewError(httpCode, businessCode int, message string) BusinessError {
	return &businessError{
		httpCode:     httpCode,
		businessCode: businessCode,
		message:      message,
		isAlert:      false,
	}
}

// WithStackError 仅附加内部错误及堆栈，不改变对外描述；nil 保留原有信息。
func (e *businessError) WithStackError(err error) BusinessError {
	cloned := *e
	if err != nil {
		cloned.stackError = errors.WithStack(err)
	}
	return &cloned
}

// Unwrap 保留标准库 errors.Is 和 errors.As 的错误链。
func (e *businessError) Unwrap() error {
	return e.stackError
}

// StackError 获取堆栈错误。
func (e *businessError) StackError() error {
	return e.stackError
}

// HTTPCode 获取 HTTP 状态码。
func (e *businessError) HTTPCode() int {
	return e.httpCode
}

// BusinessCode 获取业务错误码。
func (e *businessError) BusinessCode() int {
	return e.businessCode
}

// Message 获取错误消息；注册表错误使用当前进程语言，显式消息不随语言变化。
func (e *businessError) Message() string {
	if e.registryMessage {
		return GetText(e.businessCode)
	}
	return e.message
}

// Desc 获取错误描述。
func (e *businessError) Desc() string {
	return e.desc
}

// WithDesc 显式设置可向客户端公开的描述，不应传入内部错误原文。
func (e *businessError) WithDesc(desc string) BusinessError {
	cloned := *e
	cloned.desc = desc
	return &cloned
}

// Alert 返回带告警标记的副本，不修改共享错误对象。
func (e *businessError) Alert() BusinessError {
	cloned := *e
	cloned.isAlert = true
	return &cloned
}

// IsAlert 判断是否需要告警。
func (e *businessError) IsAlert() bool {
	return e.isAlert
}

// Error 返回用于内部诊断的完整消息；HTTP 响应单独读取 Message 和 Desc。
func (e *businessError) Error() string {
	message := e.Message()
	if e.desc != "" {
		message += ": " + e.desc
	}
	if e.stackError != nil {
		message += ": " + e.stackError.Error()
	}
	return message
}
