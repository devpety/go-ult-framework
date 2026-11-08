// Package cache 提供 Redis 缓存连接封装。
// 基于 go-utils/redis 实现，支持连接池管理、连接重试和配置化连接。
package cache

import (
	"context"
	"fmt"
	"math"
	"net"
	"strconv"
	"time"
	"ult/config/autoload"
	"ult/pkg/logger"
	"ult/pkg/startup"

	utils_redis "github.com/devpety/go-utils/v3/cache/redis"
	"go.uber.org/zap"
)

var _ Redis = (*redis)(nil)

// Redis Redis 客户端接口，定义缓存连接的基本操作。
type Redis interface {
	Get() utils_redis.Client        // 获取 Redis 客户端
	Close() error                   // 关闭 Redis 连接
	Ping(ctx context.Context) error // 测试 Redis 连接
}

// redis Redis 客户端实例，封装 go-utils Redis 客户端。
type redis struct {
	client utils_redis.Client
	logger *logger.Logger
}

// NewRedis 创建新的 Redis 连接实例。
// 初始化尝试次数及间隔与命令重试独立配置，仍受调用方总期限约束。
//
// 参数:
//   - name: Redis 连接名称，用于日志标识
//   - config: Redis 配置（地址、端口、密码、连接池等）
//   - logger: 日志记录器实例
//
// 返回:
//   - Redis: Redis 客户端实例
//   - error: 连接创建错误
func NewRedis(ctx context.Context, name string, config autoload.Redis, logger *logger.Logger) (Redis, error) {
	opts, err := redisOptions(config)
	if err != nil {
		return nil, fmt.Errorf("Redis 连接 %s 配置无效: %w", name, err)
	}
	policy, err := startup.NewPolicy(config.StartupMaxAttempts, config.StartupRetryDelay, config.ConnectTimeoutMS)
	if err != nil {
		return nil, fmt.Errorf("Redis 连接 %s 初始化策略无效: %w", name, err)
	}
	client, err := startup.Connect(ctx, policy, func(attemptCtx context.Context) (utils_redis.Client, error) {
		return utils_redis.NewClient(attemptCtx, opts)
	}, func(attempt int, err error) {
		logger.UseApp(ctx).Warn(fmt.Sprintf("Redis 连接 %s 第 %d 次初始化失败，等待重试", name, attempt), zap.Error(err))
	})
	if err != nil {
		return nil, fmt.Errorf("初始化 Redis 连接 %s 失败: %w", name, err)
	}
	return &redis{client: client, logger: logger}, nil
}

// redisOptions 在建立连接前转换和校验时长，保留底层库的默认值及负数开关。
func redisOptions(config autoload.Redis) (*utils_redis.Options, error) {
	opts := new(utils_redis.Options)
	if config.MaxRetries < -1 {
		return nil, fmt.Errorf("max_retries 不能小于 -1")
	}
	if config.Network == "unix" {
		opts.Addr = config.Addr
	} else {
		opts.Addr = net.JoinHostPort(config.Addr, strconv.Itoa(config.Port))
	}
	opts.Network = config.Network
	opts.Username = config.Username
	opts.Password = config.Password
	opts.DB = config.DB
	// 探测和业务命令均遵守调用方的期限，避免 socket 超时覆盖健康检查预算。
	opts.ContextTimeoutEnabled = true
	opts.MaxRetries = config.MaxRetries
	if opts.MaxRetries == 0 {
		// 底层零值会自动重试三次；框架默认不重放结果不确定的写命令。
		opts.MaxRetries = -1
	}
	opts.MinIdleConns = config.MinIdleConns
	opts.PoolFIFO = config.PoolFIFO
	opts.PoolSize = config.PoolSize

	fields := []struct {
		name   string
		value  int64
		unit   time.Duration
		min    int64
		target *time.Duration
	}{
		{"dial_timeout", config.DialTimeout, time.Millisecond, 0, &opts.DialTimeout},
		{"read_timeout", config.ReadTimeout, time.Millisecond, -1, &opts.ReadTimeout},
		{"write_timeout", config.WriteTimeout, time.Millisecond, -1, &opts.WriteTimeout},
		{"pool_timeout", config.PoolTimeout, time.Millisecond, 0, &opts.PoolTimeout},
		{"min_retry_backoff", config.MinRetryBackoff, time.Millisecond, -1, &opts.MinRetryBackoff},
		{"max_retry_backoff", config.MaxRetryBackoff, time.Millisecond, -1, &opts.MaxRetryBackoff},
		{"max_conn_age", config.MaxConnAge, time.Second, -1, &opts.ConnMaxLifetime},
		{"idle_timeout", config.IdleTimeout, time.Second, -1, &opts.ConnMaxIdleTime},
	}
	for _, field := range fields {
		if field.value < field.min || field.value > math.MaxInt64/int64(field.unit) {
			return nil, fmt.Errorf("%s 超出允许范围 [%d, %d]", field.name, field.min, math.MaxInt64/int64(field.unit))
		}
		if field.value < 0 {
			// -1 是底层库按原值识别的开关，不能乘以时间单位。
			*field.target = time.Duration(field.value)
		} else {
			*field.target = time.Duration(field.value) * field.unit
		}
	}

	// 使用与当前 go-redis 版本一致的有效值校验，避免零值默认或禁用组合绕过上下界。
	minBackoff, maxBackoff := opts.MinRetryBackoff, opts.MaxRetryBackoff
	if minBackoff == 0 {
		minBackoff = 8 * time.Millisecond
	} else if minBackoff == -1 {
		minBackoff = 0
	}
	if maxBackoff == 0 {
		maxBackoff = 512 * time.Millisecond
	} else if maxBackoff == -1 {
		maxBackoff = 0
	}
	// 任一端为 -1 时底层会禁用退避，保留该开关语义。
	if minBackoff > 0 && maxBackoff > 0 && minBackoff > maxBackoff {
		return nil, fmt.Errorf("min_retry_backoff 的有效值 %s 不能超过 max_retry_backoff 的有效值 %s", minBackoff, maxBackoff)
	}
	return opts, nil
}

// Get 获取 Redis 客户端实例。
//
// 返回:
//   - utils_redis.Client: Redis 客户端
func (rds *redis) Get() utils_redis.Client {
	return rds.client
}

// Close 关闭 Redis 连接。
//
// 返回:
//   - error: 关闭连接时的错误
func (rds *redis) Close() error {
	return rds.Get().Close()
}

// Ping 测试 Redis 连接是否正常。
//
// 参数:
//   - ctx: 上下文
//
// 返回:
//   - error: 连接测试错误，nil 表示连接正常
func (rds *redis) Ping(ctx context.Context) error {
	return rds.Get().Ping(ctx).Err()
}
