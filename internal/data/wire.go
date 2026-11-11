// Package data 提供数据层实现。
package data

import (
	"github.com/google/wire"
)

// ProviderSet Wire 依赖注入提供者集合。
var ProviderSet = wire.NewSet(NewData, NewDataRepo)
