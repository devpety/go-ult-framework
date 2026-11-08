// Package db 提供数据库连接封装，基于 GORM 实现。
// 支持多种数据库驱动，提供连接池管理、日志记录和插件扩展功能。
package db

import (
	"context"
	"fmt"
	"math"
	"time"
	"ult/config/autoload"
	"ult/pkg/logger"
	"ult/pkg/startup"

	"github.com/devpety/go-utils/v3/db/gorm"
	"go.uber.org/zap"
	gorm_logger "gorm.io/gorm/logger"
)

var _ Db = (*db)(nil)

// Predicate 查询条件谓词类型。
type Predicate string

// Db 数据库接口，定义数据库连接的基本操作。
type Db interface {
	Get() gorm.Client                      // 获取 GORM 客户端
	Close() error                          // 关闭数据库连接
	Ping() error                           // 测试数据库连接
	PingContext(ctx context.Context) error // 使用调用方期限测试数据库连接
}

// db 数据库实例，封装 GORM 客户端和日志记录器。
type db struct {
	client gorm.Client
	logger *logger.Logger
}

// Ping 测试数据库连接是否正常。
//
// 返回:
//   - error: 连接测试错误，nil 表示连接正常
func (db *db) Ping() error {
	return db.PingContext(context.Background())
}

// PingContext 使用请求上下文执行连接探测。
func (db *db) PingContext(ctx context.Context) error { return db.Get().SqlDB().PingContext(ctx) }

// NewDb 在调用方总期限内重试，单次连接的取消与资源清理由 go-utils 负责。
func NewDb(ctx context.Context, name string, config autoload.DB, log *logger.Logger) (Db, error) {
	policy, err := startup.NewPolicy(config.MaxRetries, config.RetryDelay, config.ConnectTimeoutMS)
	if err != nil {
		return nil, fmt.Errorf("数据库连接 %s 初始化策略无效: %w", name, err)
	}
	
	maxLifetime, err := connectionMaxLifetime(config.MaxLifeTime)
	if err != nil {
		return nil, fmt.Errorf("数据库连接 %s 配置无效: %w", name, err)
	}
	resource := &db{logger: log}
	client, err := startup.Connect(ctx, policy, func(attemptCtx context.Context) (gorm.Client, error) {
		client, err := gorm.NewClient(attemptCtx,
			gorm.WithDsn(config.Dsn),
			gorm.WithDriver(config.Driver),
			gorm.WithDbName(config.DbName),
			gorm.WithHost(config.Host),
			gorm.WithUserName(config.UserName),
			gorm.WithPassword(config.Password),
			gorm.WithCharset(config.Charset),
			gorm.WithPort(config.Port),
			gorm.WithPrefix(config.Prefix),
			gorm.WithMaxIdleConn(config.MaxIdleConn),
			gorm.WithMaxOpenConn(config.MaxOpenConn),
			gorm.WithMaxLifeTime(maxLifetime),
			gorm.WithParseTime(config.ParseTime),
			gorm.WithLoc(config.Loc),
			gorm.WithLogger(resource.WithLogger()))
		if err != nil {
			return nil, err
		}
		return client, nil
	}, func(attempt int, err error) {
		log.UseApp(ctx).Warn(fmt.Sprintf("数据库连接 %s 第 %d 次初始化失败，等待重试", name, attempt), zap.Error(err))
	})
	if err != nil {
		return nil, fmt.Errorf("初始化数据库连接 %s 失败: %w", name, err)
	}
	resource.client = client
	return resource, nil
}

// connectionMaxLifetime 将配置中的秒转换为时长；零值保持不限制寿命的语义。
func connectionMaxLifetime(seconds int64) (time.Duration, error) {
	if seconds < 0 || seconds > math.MaxInt64/int64(time.Second) {
		return 0, fmt.Errorf("max_life_time 必须在 0 到 %d 秒之间", math.MaxInt64/int64(time.Second))
	}
	return time.Duration(seconds) * time.Second, nil
}

// Get 获取 GORM 客户端实例。
//
// 返回:
//   - gorm.Client: GORM 客户端
func (db *db) Get() gorm.Client {
	return db.client
}

// Close 关闭数据库连接。
//
// 返回:
//   - error: 关闭连接时的错误
func (db *db) Close() error {
	return db.Get().Close()
}

// WithLogger 配置 GORM 日志记录器。
//
// 返回:
//   - gorm_logger.Interface: GORM 日志接口实例
func (db *db) WithLogger() gorm_logger.Interface {
	return NewLogger(
		db.logger,
		WithLoggerLevel(gorm_logger.Info),
		WithLoggerSlowThreshold(time.Second*1),
		WithLoggerIgnoreRecordNotFoundError(true))
}
