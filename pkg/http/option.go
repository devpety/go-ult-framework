// Package http 提供 HTTP 服务器实现，基于 Gin 框架封装。
package http

import (
	"fmt"
	"time"

	pkgmiddleware "ult/pkg/http/middleware"
	"ult/pkg/proposal"
)

// Option HTTP 服务器选项函数类型。
type Option func(opt *option)

// option HTTP 服务器内部选项配置。
type option struct {
	openBrowser       string                     // 启动时自动打开的浏览器 URL
	timeout           time.Duration              // 优雅关闭超时时间
	middlewares       []pkgmiddleware.Middleware // 自定义中间件列表
	routes            []func(RouterGroup)
	pprof             bool
	pprofEnvironment  string
	alertNotify       proposal.NotifyHandler
	maxBodyBytes      int64
	readHeaderTimeout time.Duration
	readTimeout       time.Duration
	writeTimeout      time.Duration
	idleTimeout       time.Duration
}

const defaultMaxBodyBytes int64 = 2 << 20

// serverOptions 构建 HTTP 服务器选项配置。
// 验证所有选项是否有效，返回配置对象。
// 如果选项无效，返回错误。
func serverOptions(opts ...Option) (*option, error) {
	options := &option{timeout: 5 * time.Second, maxBodyBytes: defaultMaxBodyBytes,
		readHeaderTimeout: 5 * time.Second, readTimeout: 15 * time.Second,
		writeTimeout: 30 * time.Second, idleTimeout: 60 * time.Second}
	for _, opt := range opts {
		if opt == nil {
			return nil, fmt.Errorf("HTTP 服务器选项不能为空")
		}
		opt(options)
	}
	for _, register := range options.routes {
		if register == nil {
			return nil, fmt.Errorf("路由注册函数不能为空")
		}
	}
	if options.maxBodyBytes <= 0 {
		return nil, fmt.Errorf("max_body_bytes 必须大于 0")
	}
	for _, limit := range []struct {
		name  string
		value time.Duration
	}{
		{"read_header_timeout", options.readHeaderTimeout}, {"read_timeout", options.readTimeout},
		{"write_timeout", options.writeTimeout}, {"idle_timeout", options.idleTimeout},
	} {
		if limit.value <= 0 {
			return nil, fmt.Errorf("%s 必须大于 0", limit.name)
		}
	}
	if options.readHeaderTimeout > options.readTimeout {
		return nil, fmt.Errorf("read_header_timeout 不能大于 read_timeout")
	}
	return options, nil
}

// WithAlertNotify 设置内置恢复中间件的告警出口，处理 panic 和显式标记的业务错误。
func WithAlertNotify(notify proposal.NotifyHandler) Option {
	return func(opt *option) { opt.alertNotify = notify }
}

// WithMaxBodyBytes 设置请求体上限（字节），必须大于零。
func WithMaxBodyBytes(limit int64) Option { return func(opt *option) { opt.maxBodyBytes = limit } }

// WithReadHeaderTimeout 设置读取请求头的超时，必须大于零。
func WithReadHeaderTimeout(timeout time.Duration) Option {
	return func(opt *option) { opt.readHeaderTimeout = timeout }
}

// WithReadTimeout 设置整个请求读取超时（包含请求头和正文），必须大于零。
func WithReadTimeout(timeout time.Duration) Option {
	return func(opt *option) { opt.readTimeout = timeout }
}

// WithWriteTimeout 设置响应写入超时，必须大于零。
func WithWriteTimeout(timeout time.Duration) Option {
	return func(opt *option) { opt.writeTimeout = timeout }
}

// WithIdleTimeout 设置长连接空闲超时，必须大于零。
func WithIdleTimeout(timeout time.Duration) Option {
	return func(opt *option) { opt.idleTimeout = timeout }
}

// EnableOpenBrowser 启动时自动打开浏览器选项。
//
// 参数:
//   - uri: 要打开的 URL
//
// 返回:
//   - Option: 选项函数
func EnableOpenBrowser(uri string) Option {
	return func(opt *option) {
		opt.openBrowser = uri
	}
}

// WithTimeout 设置优雅关闭超时时间选项。
//
// 参数:
//   - ts: 超时时间
//
// 返回:
//   - Option: 选项函数
func WithTimeout(ts time.Duration) Option {
	return func(opt *option) {
		opt.timeout = ts
	}
}

// WithMiddleware 添加自定义中间件选项。
// 支持新的中间件管理系统，提供优先级控制和灵活配置。
//
// 参数:
//   - m: 中间件列表
//
// 返回:
//   - Option: 选项函数
func WithMiddleware(m ...pkgmiddleware.Middleware) Option {
	return func(opt *option) {
		opt.middlewares = append(opt.middlewares, m...)
	}
}

// WithRoutes 在全局中间件安装后同步注册路由；可多次调用并按选项顺序执行。
// 回调返回后不得异步继续配置，服务器构建结束后所有路由组都会关闭注册。
func WithRoutes(register func(RouterGroup)) Option {
	return func(opt *option) { opt.routes = append(opt.routes, register) }
}

// WithPProf 在构建阶段注册性能分析路由，生产环境沿用禁用规则。
func WithPProf(environment string) Option {
	return func(opt *option) {
		opt.pprof = true
		opt.pprofEnvironment = environment
	}
}
