// Package middleware 提供基于 HTTP 框架的中间件管理系统。
package middleware

import (
	"fmt"
	nethttp "net/http"
	"net/url"
	"slices"
	"strings"
	"ult/pkg/types"

	utilsMiddleware "github.com/devpety/go-utils/v3/middleware"
	"github.com/gin-gonic/gin"
	"github.com/rs/cors"
)

// CORSConfig CORS 中间件配置。
type CORSConfig struct {
	// Enabled 是否启用 CORS 中间件
	Enabled bool

	// AllowedOrigins 允许的域名列表
	// 例如: ["https://example.com", "http://localhost:3000"] 或 ["*"]
	AllowedOrigins []string

	// AllowedMethods 允许的 HTTP 方法
	// 默认包含常用的 HTTP 方法
	AllowedMethods []string

	// AllowedHeaders 允许的请求头
	AllowedHeaders []string

	// AllowCredentials 是否允许携带凭证（如 Cookie）
	AllowCredentials bool

	// OptionsPassthrough 是否让 OPTIONS 请求继续传递
	OptionsPassthrough bool
}

// CORS CORS 中间件。
// 处理跨域资源共享（Cross-Origin Resource Sharing）。
type CORS struct {
	config      *CORSConfig
	corsHandler *cors.Cors
}

// NewCORS 创建 CORS 中间件实例。
//
// 参数:
//   - config: CORS 配置
//
// 返回:
//   - *CORS: CORS 中间件实例
//   - error: 来源或凭证策略无效
func NewCORS(config *CORSConfig) (*CORS, error) {
	if config == nil {
		return nil, fmt.Errorf("CORS 配置不能为空")
	}
	cloned := *config
	cloned.AllowedOrigins = slices.Clone(config.AllowedOrigins)
	cloned.AllowedMethods = slices.Clone(config.AllowedMethods)
	cloned.AllowedHeaders = slices.Clone(config.AllowedHeaders)
	config = &cloned
	if !config.Enabled {
		return &CORS{config: config}, nil
	}
	if len(config.AllowedOrigins) == 0 {
		return nil, fmt.Errorf("启用 CORS 时必须声明 allowed_origins")
	}
	for index, origin := range config.AllowedOrigins {
		origin = strings.TrimSpace(origin)
		if origin == "*" {
			if config.AllowCredentials {
				return nil, fmt.Errorf("CORS 携带凭证时不能使用通配来源")
			}
		} else {
			parsed, err := url.Parse(origin)
			if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Hostname() == "" ||
				parsed.User != nil || parsed.Path != "" || parsed.RawQuery != "" || parsed.ForceQuery || parsed.Fragment != "" || strings.Contains(origin, "*") {
				return nil, fmt.Errorf("CORS 来源 %q 必须是完整的 HTTP 或 HTTPS Origin，不能包含路径或通配符", origin)
			}
		}
		config.AllowedOrigins[index] = origin
	}
	// 设置默认值
	if len(config.AllowedMethods) == 0 {
		config.AllowedMethods = []string{
			nethttp.MethodHead,
			nethttp.MethodGet,
			nethttp.MethodPost,
			nethttp.MethodPut,
			nethttp.MethodPatch,
			nethttp.MethodDelete,
		}
	}
	if len(config.AllowedHeaders) == 0 {
		config.AllowedHeaders = []string{"Content-Type", "Authorization", types.TraceIdName}
	}

	// 在初始化时创建 CORS 处理器，避免每次请求重复创建
	corsHandler := cors.New(cors.Options{
		AllowedOrigins:     config.AllowedOrigins,
		AllowedMethods:     config.AllowedMethods,
		AllowedHeaders:     config.AllowedHeaders,
		AllowCredentials:   config.AllowCredentials,
		OptionsPassthrough: config.OptionsPassthrough,
	})

	return &CORS{
		config:      config,
		corsHandler: corsHandler,
	}, nil
}

// Name 返回中间件名称。
func (c *CORS) Name() string {
	return types.CorsMiddlewareName
}

// Priority 返回中间件优先级。
// 与 Recovery 同级，依靠框架先注册 Recovery 的稳定排序，在正文读取前处理跨域。
func (c *CORS) Priority() utilsMiddleware.Priority {
	return utilsMiddleware.PriorityHighest
}

// Enabled 返回是否启用。
func (c *CORS) Enabled() bool {
	return c.config.Enabled
}

// Dependencies 返回中间件依赖列表。
// CORS 中间件无依赖。
func (c *CORS) Dependencies() []string {
	return []string{}
}

// Handler 返回中间件处理函数。
func (c *CORS) Handler() HandlerFunc {
	return c.handler()
}

// handler 返回中间件处理函数。
// 使用缓存的 CORS 处理器，避免每次请求重复创建。
func (c *CORS) handler() HandlerFunc {
	return func(ctx *gin.Context) {
		// 如果没有配置允许的域名或处理器未创建，跳过 CORS 处理
		if c.corsHandler == nil {
			ctx.Next()
			return
		}

		continued := false
		c.corsHandler.ServeHTTP(ctx.Writer, ctx.Request, func(nethttp.ResponseWriter, *nethttp.Request) {
			continued = true
			ctx.Next()
		})
		if !continued {
			ctx.Abort()
			// Gin 延迟提交状态码，预检必须在统一响应收尾前完成写入。
			ctx.Writer.WriteHeaderNow()
		}
	}
}

// DefaultCORSConfig 返回默认 CORS 配置。
// 采用安全配置，默认不允许任何域名，强制用户显式配置。
//
// 返回:
//   - *CORSConfig: 默认 CORS 配置
func DefaultCORSConfig() *CORSConfig {
	return &CORSConfig{
		Enabled:        false,
		AllowedOrigins: []string{}, // 默认不允许任何域名，强制用户配置
		AllowedMethods: []string{
			nethttp.MethodHead,
			nethttp.MethodGet,
			nethttp.MethodPost,
			nethttp.MethodPut,
			nethttp.MethodPatch,
			nethttp.MethodDelete,
		},
		AllowCredentials:   false,
		OptionsPassthrough: false,
	}
}
