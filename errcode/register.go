// Package errcode 提供统一错误码定义和管理。
package errcode

import (
	"maps"
	"strings"
	"sync"
)

// registry 全局错误码注册表实例。
var registry = &codeRegistry{
	local: ZhCN,
	textsByLocale: map[string]map[int]string{
		ZhCN: maps.Clone(zhCNText),
		EnUS: maps.Clone(enUSText),
	},
	httpCodes: maps.Clone(httpCode),
}

// codeRegistry 错误码注册表结构体。
// 支持多语言错误消息和 HTTP 状态码映射。
type codeRegistry struct {
	mu            sync.RWMutex              // 读写锁，保证并发安全
	local         string                    // 当前语言
	textsByLocale map[string]map[int]string // 各语言独立的错误码消息映射
	httpCodes     map[int]int               // 错误码到 HTTP 状态码映射
}

// NewRegistry 设置当前进程语言，不清空已经注册的翻译。
//
// 参数:
//   - local: 语言代码 (zh-cn/en-us)
func NewRegistry(local string) {
	registry.mu.Lock()
	defer registry.mu.Unlock()

	local = strings.ToLower(local)
	switch local {
	case ZhCN:
		registry.local = ZhCN
	case EnUS:
		registry.local = EnUS
	default:
		registry.local = ZhCN
	}
}

// GetRegistry 获取错误码注册表实例。
//
// 返回:
//   - *codeRegistry: 注册表实例
func GetRegistry() *codeRegistry {
	return registry
}

// RegisterHTTPCodes 注册 HTTP 状态码映射。
//
// 参数:
//   - codes: 错误码到 HTTP 状态码的映射
func RegisterHTTPCodes(codes map[int]int) {
	registry.mu.Lock()
	defer registry.mu.Unlock()

	for key, value := range codes {
		registry.httpCodes[key] = value
	}
}

// RegisterTexts 向指定语言注册消息，不切换当前语言。
//
// 参数:
//   - local: 语言代码
//   - texts: 错误码到消息文本的映射
func RegisterTexts(local string, texts map[int]string) {
	registry.mu.Lock()
	defer registry.mu.Unlock()

	local = strings.ToLower(local)
	if registry.textsByLocale[local] == nil {
		registry.textsByLocale[local] = make(map[int]string)
	}
	for key, value := range texts {
		registry.textsByLocale[local][key] = value
	}
}

// GetText 根据错误码获取消息文本。
//
// 参数:
//   - code: 业务错误码
//
// 返回:
//   - string: 错误消息文本
func GetText(code int) string {
	registry.mu.RLock()
	defer registry.mu.RUnlock()

	return registry.textsByLocale[registry.local][code]
}

// GetHTTPCode 根据业务错误码获取 HTTP 状态码。
//
// 参数:
//   - code: 业务错误码
//
// 返回:
//   - int: HTTP 状态码
func GetHTTPCode(code int) int {
	registry.mu.RLock()
	defer registry.mu.RUnlock()

	return registry.httpCodes[code]
}

// New 根据业务错误码创建业务错误实例。
// HTTP 状态码在创建时获取，消息文本在读取时按当前语言解析。
//
// 参数:
//   - code: 业务错误码
//
// 返回:
//   - BusinessError: 业务错误实例
func New(code int) BusinessError {
	return &businessError{
		httpCode:        GetHTTPCode(code),
		businessCode:    code,
		registryMessage: true,
	}
}
