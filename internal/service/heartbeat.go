// Package service 提供业务逻辑层实现。
// 服务层负责处理业务逻辑，调用数据层获取数据，为 API 层提供服务接口。
package service

import (
	"context"
	"maps"
	"slices"
	"time"
	"ult/internal/app"
	"ult/internal/data"
	"ult/internal/data/repo"
)

// HeartbeatStatusResponse 健康检查状态结构体。
// 包含整体状态、时间戳和各组件状态。
type HeartbeatStatusResponse struct {
	Status     string                        `json:"status"`               // 整体状态: healthy/degraded/unhealthy
	Timestamp  string                        `json:"timestamp"`            // 检查时间戳
	Components map[string]HeartbeatComponent `json:"components,omitempty"` // 各组件状态
}

// HeartbeatComponent 健康检查组件状态结构体。
type HeartbeatComponent struct {
	Status  string `json:"status"`            // 组件状态: healthy/unhealthy
	Message string `json:"message,omitempty"` // 状态消息（错误信息）
}

// HeartbeatService 健康检查服务。
// 提供数据库和 Redis 连接状态检查功能。
type HeartbeatService struct {
	dataRepo     *data.DataRepo // 数据仓库
	testRepo     repo.TestRepo  // 测试数据仓库
	tools        *app.Tools     // 应用工具包
	probeTimeout time.Duration
}

// NewHeartbeatService 创建新的健康检查服务实例。
//
// 参数:
//   - dataRepo: 数据仓库
//   - testRepo: 测试数据仓库
//   - tools: 应用工具包
//
// 返回:
//   - *HeartbeatService: 健康检查服务实例
func NewHeartbeatService(dataRepo *data.DataRepo, testRepo repo.TestRepo, tools *app.Tools) *HeartbeatService {
	return &HeartbeatService{
		dataRepo:     dataRepo,
		testRepo:     testRepo,
		tools:        tools,
		probeTimeout: 2 * time.Second,
	}
}

// State 获取系统健康状态。
// 检查数据库和 Redis 连接状态，返回整体健康状态。
//
// 参数:
//   - ctx: 上下文
//
// 返回:
//   - HeartbeatStatusResponse: 健康状态
func (h *HeartbeatService) State(ctx context.Context) HeartbeatStatusResponse {
	timeout := h.probeTimeout
	if timeout <= 0 {
		timeout = 2 * time.Second
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	status := HeartbeatStatusResponse{Status: "healthy", Timestamp: time.Now().Format(time.RFC3339), Components: make(map[string]HeartbeatComponent)}
	if h.dataRepo == nil {
		status.Status = "unhealthy"
		status.Components["data"] = HeartbeatComponent{Status: "unhealthy", Message: "data repository is nil"}
		return status
	}
	failures := 0
	check := func(name string, ping func(context.Context) error) {
		component := HeartbeatComponent{Status: "healthy"}
		switch {
		case ping == nil:
			component = HeartbeatComponent{Status: "unhealthy", Message: "configured connection is missing"}
		case ctx.Err() != nil:
			component = HeartbeatComponent{Status: "unhealthy", Message: ctx.Err().Error()}
		default:
			if err := ping(ctx); err != nil {
				component = HeartbeatComponent{Status: "unhealthy", Message: err.Error()}
			}
		}
		if component.Status == "unhealthy" {
			failures++
		}
		status.Components[name] = component
	}
	dbNames := h.dataRepo.DBNames
	if len(dbNames) == 0 && h.dataRepo.DbRepo != nil {
		dbNames = slices.Sorted(maps.Keys(h.dataRepo.DbRepo.All()))
	}
	for _, name := range dbNames {
		if h.dataRepo.DbRepo == nil || h.dataRepo.DbRepo.DB(name) == nil {
			check("db_"+name, nil)
			continue
		}
		check("db_"+name, h.dataRepo.DbRepo.DB(name).PingContext)
	}
	redisNames := h.dataRepo.RedisNames
	if len(redisNames) == 0 && h.dataRepo.RedisRepo != nil {
		redisNames = slices.Sorted(maps.Keys(h.dataRepo.RedisRepo.All()))
	}
	for _, name := range redisNames {
		if h.dataRepo.RedisRepo == nil || h.dataRepo.RedisRepo.Redis(name) == nil {
			check("redis_"+name, nil)
			continue
		}
		check("redis_"+name, h.dataRepo.RedisRepo.Redis(name).Ping)
	}
	if failures > 0 {
		status.Status = "degraded"
		if failures == len(status.Components) {
			status.Status = "unhealthy"
		}
	}
	if err := ctx.Err(); err != nil {
		status.Status = "unhealthy"
		status.Components["probe"] = HeartbeatComponent{Status: "unhealthy", Message: err.Error()}
	}
	return status
}
