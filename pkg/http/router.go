// Package http 提供 HTTP 服务器实现，基于 Gin 框架封装。
package http

import (
	"net/http"
	"sync"
	"ult/pkg/types"

	"github.com/gin-gonic/gin"
)

var _ RouterGroup = (*router)(nil)

// RouterGroup 路由组接口，定义路由分组功能。
type RouterGroup interface {
	Group(relativePath string, handlers ...HandlerFunc) RouterGroup
	IRouter
}

// IRouter 路由接口，定义 HTTP 方法路由注册功能。
type IRouter interface {
	Any(relativePath string, handlers ...HandlerFunc)
	GET(relativePath string, handlers ...HandlerFunc)
	POST(relativePath string, handlers ...HandlerFunc)
	DELETE(relativePath string, handlers ...HandlerFunc)
	PATCH(relativePath string, handlers ...HandlerFunc)
	PUT(relativePath string, handlers ...HandlerFunc)
	OPTIONS(relativePath string, handlers ...HandlerFunc)
	HEAD(relativePath string, handlers ...HandlerFunc)
}

// routeRegistration 的锁仅保护构建期间的配置及关闭，不进入请求处理路径。
// 根路由组和子路由组共享状态，防止外部保留引用后修改运行中的 Gin 路由树。
type routeRegistration struct {
	mu     sync.Mutex
	closed bool
}

// configure 配置路由组，确保在构建阶段完成注册。
func (registration *routeRegistration) configure(configure func()) {
	registration.mu.Lock()
	defer registration.mu.Unlock()
	if registration.closed {
		panic("路由配置已结束，请通过 WithRoutes 在构建阶段完成注册")
	}
	configure()
}

// close 关闭路由组，防止后续配置。
func (registration *routeRegistration) close() {
	registration.mu.Lock()
	defer registration.mu.Unlock()
	registration.closed = true
}

type router struct {
	group        *gin.RouterGroup
	registration *routeRegistration
}

// Group 创建子路由组，用于组织路由。
// 子路由组的路径是相对于路由组的路径，例如 "/api/v1"。
func (r *router) Group(relativePath string, handlers ...HandlerFunc) RouterGroup {
	var child RouterGroup
	r.registration.configure(func() {
		child = &router{
			group:        r.group.Group(relativePath, wrapHandlers(handlers...)...),
			registration: r.registration,
		}
	})
	return child
}

// register 注册 HTTP 方法路由，根据请求方法和路径匹配处理函数。
// 用于处理指定方法的请求，例如 "/api/v1/users"。
// register 注册 HTTP 方法路由，用于构建阶段注册路由。
// method 是 HTTP 方法，relativePath 是路由路径，handlers 是路由处理函数。
func (r *router) register(method, relativePath string, handlers ...HandlerFunc) {
	r.registration.configure(func() { r.group.Handle(method, relativePath, wrapHandlers(handlers...)...) })
}

// Any 注册任意 HTTP 方法路由，匹配所有请求方法。
// 用于处理未指定方法的请求，例如 "/api/v1"。
func (r *router) Any(relativePath string, handlers ...HandlerFunc) {
	r.registration.configure(func() { r.group.Any(relativePath, wrapHandlers(handlers...)...) })
}

// GET 注册 GET 方法路由，用于处理 GET 请求。
// 例如处理 "/api/v1/users" 路由。
func (r *router) GET(relativePath string, handlers ...HandlerFunc) {
	r.register(http.MethodGet, relativePath, handlers...)
}

// POST 注册 POST 方法路由，用于处理 POST 请求。
// 例如处理 "/api/v1/users" 路由。
func (r *router) POST(relativePath string, handlers ...HandlerFunc) {
	r.register(http.MethodPost, relativePath, handlers...)
}

// DELETE 注册 DELETE 方法路由，用于处理 DELETE 请求。
// 例如处理 "/api/v1/users/123" 路由。
func (r *router) DELETE(relativePath string, handlers ...HandlerFunc) {
	r.register(http.MethodDelete, relativePath, handlers...)
}

// PATCH 注册 PATCH 方法路由，用于处理 PATCH 请求。
// 例如处理 "/api/v1/users/123" 路由。
func (r *router) PATCH(relativePath string, handlers ...HandlerFunc) {
	r.register(http.MethodPatch, relativePath, handlers...)
}

// PUT 注册 PUT 方法路由，用于处理 PUT 请求。
// 例如处理 "/api/v1/users/123" 路由。
func (r *router) PUT(relativePath string, handlers ...HandlerFunc) {
	r.register(http.MethodPut, relativePath, handlers...)
}

// OPTIONS 注册 OPTIONS 方法路由，用于处理 OPTIONS 请求。
// 例如处理 "/api/v1/users" 路由。
func (r *router) OPTIONS(relativePath string, handlers ...HandlerFunc) {
	r.register(http.MethodOptions, relativePath, handlers...)
}

// HEAD 注册 HEAD 方法路由，用于处理 HEAD 请求。
// 例如处理 "/api/v1/users" 路由。
func (r *router) HEAD(relativePath string, handlers ...HandlerFunc) {
	r.register(http.MethodHead, relativePath, handlers...)
}

// wrapHandlers 包装处理函数，将自定义 HandlerFunc 转换为 Gin HandlerFunc。
// 复用 Request 中间件已初始化的 Context，避免重复创建。
func wrapHandlers(handlers ...HandlerFunc) []gin.HandlerFunc {
	funcs := make([]gin.HandlerFunc, len(handlers))
	for i, handler := range handlers {
		if handler == nil {
			panic("路由处理函数不能为空")
		}
		funcs[i] = func(c *gin.Context) {
			// 路由只在完整请求处理链安装后创建，直接复用 Request 初始化的上下文。
			handler(c.MustGet(types.CoreContextNameKey).(Context))
		}
	}

	return funcs
}
