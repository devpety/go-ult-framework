// Package autoload 提供配置模块自动加载。
// 各配置模块按功能拆分，便于管理和维护。
package autoload

// App 应用配置结构体。
// 定义应用的基本信息。
type App struct {
	ID               string `yaml:"id"`                 // 应用唯一标识
	Name             string `yaml:"name"`               // 应用名称
	Version          string `yaml:"version"`            // 应用版本
	StartupTimeoutMS int64  `yaml:"startup_timeout_ms"` // 所有依赖共享的初始化预算（毫秒），0 使用 30 秒
}
