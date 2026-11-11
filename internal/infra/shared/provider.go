// Package shared 提供跨层复用的通用依赖构造。
package shared

import (
	"ult/config"

	"github.com/devpety/go-utils/v3/server/system"
)

// NewEnvironment 按配置构造运行环境，非标准取值回落为 dev。
func NewEnvironment(conf *config.Config) system.Environment {
	return system.NewEnvironment(conf.Environment)
}

// NewDatetime 按配置构造日期时间实例。
// 供入口构建日志和业务层按需注入，需要时把本函数加入 `wire.Build`。
func NewDatetime(conf *config.Config) *system.Datetime {
	return system.NewDatetime(
		system.WithLocation(conf.Datetime.Location),
		system.WithCSTLayout(conf.Datetime.CSTLayout))
}
