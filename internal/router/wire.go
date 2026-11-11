// Package router 提供路由层实现。
// 路由层负责注册 HTTP 路由，将 API 处理器绑定到对应的路由路径。
package router

import (
	"github.com/google/wire"
)

// ProviderSet Wire 依赖注入提供者集合。
var ProviderSet = wire.NewSet(NewHTTPRouter)
