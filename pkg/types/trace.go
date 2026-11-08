// Package types 提供通用类型定义。
package types

import (
	"context"
	"reflect"
)

// 链路追踪常量。
const (
	TraceIdName = "x-md-trace-id" // TraceID 请求头名称
)

// TraceIDFromContext 从结构化请求上下文读取 TraceID；缺失时返回空值，不生成 ID。
func TraceIDFromContext(ctx context.Context) string {
	if isNilTraceValue(ctx) {
		return ""
	}
	request, ok := FromRequestContext(ctx)
	if !ok || isNilTraceValue(request) {
		return ""
	}
	return request.TraceID()
}

// 接口中的 nil 指针不等于 nil，日志提取时应将其视为缺失值。
func isNilTraceValue(value interface{}) bool {
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
