// Package middleware 提供基于 HTTP 框架的中间件管理系统。
package middleware

import (
	utilsMiddleware "github.com/devpety/go-utils/v3/middleware"
	"github.com/gin-gonic/gin"
)

// Priority 中间件优先级类型。
type Priority = utilsMiddleware.Priority

// HandlerFunc 统一使用 Gin 的中间件处理函数类型。
type HandlerFunc = gin.HandlerFunc

// Middleware HTTP 框架中间件接口。
// 处理函数和依赖声明在编译阶段约束。
type Middleware interface {
	Name() string
	Priority() Priority
	Handler() HandlerFunc
	// Dependencies 返回中间件依赖列表。
	// 只校验依赖是否存在于有效注册列表，不改变优先级执行顺序。
	// 返回依赖的中间件名称列表（例如：["recovery", "request"]）。
	Dependencies() []string
}

// middlewareFunc 函数式中间件实现。
type middlewareFunc struct {
	name         string
	priority     Priority
	handler      HandlerFunc
	dependencies []string // 依赖的中间件名称列表
}

// Name 返回中间件名称。
func (m *middlewareFunc) Name() string { return m.name }

// Priority 返回中间件优先级。
func (m *middlewareFunc) Priority() Priority { return m.priority }

// Handler 返回中间件处理函数。
func (m *middlewareFunc) Handler() HandlerFunc {
	return m.handler
}

// Dependencies 返回中间件依赖列表。
// 函数式中间件默认无依赖。
func (m *middlewareFunc) Dependencies() []string {
	return m.dependencies
}

// NewMiddlewareFunc 创建函数式中间件。
//
// 参数:
//   - name: 中间件名称
//   - priority: 中间件优先级
//   - handler: 中间件处理函数
//
// 返回:
//   - Middleware: 中间件实例
func NewMiddlewareFunc(name string, priority Priority, handler HandlerFunc) Middleware {
	return NewMiddlewareFuncWithDependencies(name, priority, handler, nil)
}

// NewMiddlewareFuncWithDependencies 创建带依赖的函数式中间件。
//
// 参数:
//   - name: 中间件名称
//   - priority: 中间件优先级
//   - handler: 中间件处理函数
//   - dependencies: 依赖的中间件名称列表
//
// 返回:
//   - Middleware: 中间件实例
func NewMiddlewareFuncWithDependencies(
	name string,
	priority Priority,
	handler HandlerFunc,
	dependencies []string,
) Middleware {
	return &middlewareFunc{
		name:         name,
		priority:     priority,
		handler:      handler,
		dependencies: dependencies,
	}
}
