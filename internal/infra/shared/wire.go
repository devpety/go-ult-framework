// Package shared 提供跨层复用的通用依赖构造。
package shared

import (
	"github.com/google/wire"
)

// ProviderSet 装配图可用的通用依赖。
// 未接入装配目标的构造函数不放进集合，否则 Wire 会报 unused provider。
var ProviderSet = wire.NewSet(NewEnvironment)
