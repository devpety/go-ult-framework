// Package middleware 提供基于 HTTP 框架的中间件管理系统。
package middleware

import (
	"fmt"
	"reflect"
	"sort"
	"strings"

	utilsMiddleware "github.com/devpety/go-utils/v3/middleware"
)

// Manager 中间件管理器。
// 私有组合通用 Manager，所有条目通过强类型注册入口写入。
type Manager struct {
	manager *utilsMiddleware.Manager
}

// managerEntry 仅在内部对接通用管理器，不向调用方暴露宽泛的 Handler 接口。
type managerEntry struct {
	Middleware
}

func (entry managerEntry) Handler() utilsMiddleware.Handler {
	return entry.Middleware.Handler()
}

func (entry managerEntry) Enabled() bool {
	config, ok := entry.Middleware.(utilsMiddleware.Config)
	return !ok || config.Enabled()
}

// NewManager 创建中间件管理器。
//
// 返回:
//   - *Manager: 中间件管理器实例
func NewManager() *Manager {
	return &Manager{
		manager: utilsMiddleware.NewManager(),
	}
}

// Use 添加中间件。
//
// 参数:
//   - middleware: 中间件实例
//
// 返回:
//   - error: 中间件为 nil 时返回配置错误
func (m *Manager) Use(middleware Middleware) error {
	if isNilMiddleware(middleware) {
		return fmt.Errorf("中间件不能为 nil，实际类型: %T", middleware)
	}
	m.manager.Use(managerEntry{Middleware: middleware})
	return nil
}

// UseFunc 使用函数方式添加中间件。
//
// 参数:
//   - name: 中间件名称
//   - priority: 中间件优先级
//   - handler: 中间件处理函数
//
// 返回:
//   - error: 注册配置错误
func (m *Manager) UseFunc(name string, priority Priority, handler HandlerFunc) error {
	return m.Use(NewMiddlewareFunc(name, priority, handler))
}

// List 返回按注册顺序排列的独立切片；中间件实例仍是原配置对象。
func (m *Manager) List() []Middleware {
	entries := m.manager.List()
	items := make([]Middleware, len(entries))
	for index, entry := range entries {
		// 私有管理器只能通过 Use 写入 managerEntry。
		items[index] = entry.(managerEntry).Middleware
	}
	return items
}

// Count 返回有效中间件数量。
func (m *Manager) Count() int { return m.manager.Count() }

// Clear 清空有效注册列表。
func (m *Manager) Clear() { m.manager.Clear() }

// Remove 移除指定名称的中间件。
func (m *Manager) Remove(name string) bool { return m.manager.Remove(name) }

// Has 判断指定名称的中间件是否已注册。
func (m *Manager) Has(name string) bool { return m.manager.Has(name) }

// Get 返回指定名称的中间件；不存在时返回 nil。
func (m *Manager) Get(name string) Middleware {
	entry := m.manager.Get(name)
	if entry == nil {
		return nil
	}
	return entry.(managerEntry).Middleware
}

// Build 构建中间件链。
// 按优先级稳定排序并验证有效依赖及空处理函数；非法配置返回 error。
//
// 返回:
//   - []HandlerFunc: 中间件处理函数列表
//   - error: 名称、依赖或处理函数无效
func (m *Manager) Build() ([]HandlerFunc, error) {
	// 父管理器在注册时过滤禁用项，校验与构建必须使用同一份有效列表。
	items := m.List()
	if err := m.validateDependencies(items); err != nil {
		return nil, err
	}
	sort.SliceStable(items, func(i, j int) bool {
		return items[i].Priority() < items[j].Priority()
	})
	handlers := make([]HandlerFunc, 0, len(items))
	for _, item := range items {
		handler := item.Handler()
		if handler == nil {
			return nil, fmt.Errorf("中间件 %q 的处理器不能为空", item.Name())
		}
		handlers = append(handlers, handler)
	}
	return handlers, nil
}

// validateDependencies 验证中间件依赖关系。
// 检查每个中间件的依赖是否都已注册。
func (m *Manager) validateDependencies(items []Middleware) error {
	// 构建已注册中间件的名称集合
	registered := make(map[string]bool)
	for _, item := range items {
		if isNilMiddleware(item) {
			return fmt.Errorf("中间件不能为 nil，实际类型: %T", item)
		}
		name := item.Name()
		if strings.TrimSpace(name) == "" {
			return fmt.Errorf("中间件名称不能为空")
		}
		if registered[name] {
			return fmt.Errorf("中间件名称重复: %q", name)
		}
		registered[name] = true
	}

	// 验证每个中间件的依赖
	for _, item := range items {
		for _, dep := range item.Dependencies() {
			if !registered[dep] {
				return fmt.Errorf(
					"中间件依赖验证失败: 中间件 '%s' 依赖 '%s'，但 '%s' 未注册",
					item.Name(),
					dep,
					dep,
				)
			}
		}
	}
	return nil
}

// isNilMiddleware 检查中间件是否为 nil。
// 同时识别接口中保存的 nil 指针。
func isNilMiddleware(value interface{}) bool {
	if value == nil {
		return true
	}
	reflected := reflect.ValueOf(value)
	switch reflected.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Ptr, reflect.Slice:
		return reflected.IsNil()
	default:
		return false
	}
}
