// Package api 提供 API 处理层实现。
// API 层负责处理 HTTP 请求，进行数据校验和响应，调用服务层处理业务逻辑。
package api

import (
	"ult/errcode"
	"ult/internal/service"
	"ult/pkg/http"
)

// HeartbeatInterface 接口验证。
var _ HeartbeatInterface = (*HeartbeatHandler)(nil)

// HeartbeatInterface 健康检查 API 接口。
// 定义健康检查相关的 HTTP 处理方法。
type HeartbeatInterface interface {
	State(ctx http.Context) // 获取系统健康状态
	Ready(ctx http.Context) // 检查已配置依赖是否就绪
}

// HeartbeatHandler 健康检查 API 处理器。
type HeartbeatHandler struct {
	service *service.HeartbeatService // 健康检查服务
}

// NewHeartbeatHandler 创建新的健康检查 API 处理器实例。
//
// 参数:
//   - service: 健康检查服务
//
// 返回:
//   - HeartbeatInterface: 健康检查 API 接口
func NewHeartbeatHandler(service *service.HeartbeatService) HeartbeatInterface {
	return &HeartbeatHandler{
		service: service,
	}
}

// State 获取系统健康状态的 HTTP 处理函数。
// 调用服务层获取数据库和 Redis 连接状态。
func (h *HeartbeatHandler) State(ctx http.Context) {
	var resp = h.service.State(ctx.RequestContext())
	ctx.WithPayload(resp)
}

// Ready 为就绪探针返回 200 或 503，诊断接口 State 保持原有 HTTP 状态语义。
func (h *HeartbeatHandler) Ready(ctx http.Context) {
	resp := h.service.State(ctx.RequestContext())
	if resp.Status != "healthy" {
		ctx.WithAbortError(errcode.New(errcode.ServiceUnavailableError))
		return
	}
	ctx.WithPayload(resp)
}
