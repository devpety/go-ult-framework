// Package server 提供服务器层实现。
// 服务器层负责创建和配置 HTTP 服务器实例。
package server

import (
	"context"
	"fmt"
	"math"
	"net"
	"strconv"
	"time"

	"ult/config"
	"ult/config/autoload"
	"ult/internal/router"
	pkghttp "ult/pkg/http"
	pkgmiddleware "ult/pkg/http/middleware"
	"ult/pkg/logger"
	"ult/pkg/notify/recover/email"

	"github.com/devpety/go-utils/v3/http"
	"github.com/devpety/go-utils/v3/server/system"
)

// NewHTTPServer 创建并配置 HTTP 服务器实例。
// 设置服务器地址、CORS、pprof 等配置，注册路由。
//
// 参数:
//   - config: 应用配置
//   - appLogger: 日志实例
//   - environment: 运行环境，决定 pprof 是否注册
//   - httpRouter: 路由注册函数
//
// 返回:
//   - *pkghttp.HTTPServer: HTTP 服务器实例
//   - error: 配置或请求限制无效
func NewHTTPServer(
	config *config.Config,
	appLogger *logger.Logger,
	environment system.Environment,
	httpRouter router.HTTPRouter) (*pkghttp.HTTPServer, error) {
	if config == nil {
		return nil, fmt.Errorf("HTTP 配置不能为空")
	}
	options, err := httpLimitOptions(config.Server.Http)
	if err != nil {
		return nil, err
	}
	var addr = net.JoinHostPort(config.Server.Http.Host, strconv.Itoa(config.Server.Http.Port))
	// 创建 CORS 中间件
	corsConfig := config.Server.Http.Cors
	corsMiddleware, err := pkgmiddleware.NewCORS(&pkgmiddleware.CORSConfig{
		Enabled:          corsConfig.Enabled,
		AllowedOrigins:   corsConfig.AllowedOrigins,
		AllowedMethods:   corsConfig.AllowedMethods,
		AllowedHeaders:   corsConfig.AllowedHeaders,
		AllowCredentials: corsConfig.AllowCredentials,
	})
	if err != nil {
		return nil, err
	}
	if config.Notify.Recover.Email.Host != "" {
		options = append(options, pkghttp.WithAlertNotify(email.NotifyHandler(context.Background(), config.Notify, appLogger)))
	}

	options = append(options,
		pkghttp.WithMiddleware(corsMiddleware),
		pkghttp.WithRoutes(httpRouter),
		pkghttp.WithPProf(environment.Value()),
	)
	
	return pkghttp.NewServer(
		config,
		appLogger,
		[]http.ServerOption{
			http.WithServerNetwork(config.Server.Http.Network),
			http.WithServerAddress(addr),
		},
		options...,
	)
}

func httpLimitOptions(config autoload.Http) ([]pkghttp.Option, error) {
	if config.MaxBodyBytes < 0 {
		return nil, fmt.Errorf("max_body_bytes 不能为负数")
	}
	var options []pkghttp.Option
	if config.MaxBodyBytes > 0 {
		options = append(options, pkghttp.WithMaxBodyBytes(config.MaxBodyBytes))
	}
	for _, limit := range []struct {
		name   string
		value  int64
		option func(time.Duration) pkghttp.Option
	}{
		{"read_header_timeout_ms", config.ReadHeaderTimeoutMS, pkghttp.WithReadHeaderTimeout},
		{"read_timeout_ms", config.ReadTimeoutMS, pkghttp.WithReadTimeout},
		{"write_timeout_ms", config.WriteTimeoutMS, pkghttp.WithWriteTimeout},
		{"idle_timeout_ms", config.IdleTimeoutMS, pkghttp.WithIdleTimeout},
	} {
		if limit.value < 0 || limit.value > math.MaxInt64/int64(time.Millisecond) {
			return nil, fmt.Errorf("%s 超出允许范围", limit.name)
		}
		if limit.value > 0 {
			options = append(options, limit.option(time.Duration(limit.value)*time.Millisecond))
		}
	}
	return options, nil
}
