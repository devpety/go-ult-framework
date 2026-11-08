// Package repositories 提供数据仓库抽象层。
package repositories

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"slices"
	"ult/config"
	"ult/config/autoload"
	"ult/pkg/cache"
	"ult/pkg/db"
	"ult/pkg/logger"
	"ult/pkg/startup"

	"go.uber.org/zap"
)

var _ DataRepo = (*dataRepo)(nil)

// DataRepo 数据仓库接口，定义数据库和 Redis 连接管理操作。
type DataRepo interface {
	DB(name string) db.Db          // 获取指定名称的数据库连接
	DbRepo() DbRepo                // 获取数据库仓库
	Redis(name string) cache.Redis // 获取指定名称的 Redis 连接
	RedisRepo() RedisRepo          // 获取 Redis 仓库
}

// dataRepo 数据仓库实例，管理数据库和 Redis 连接。
type dataRepo struct {
	db    *dbRepo    // 数据库仓库
	redis *redisRepo // Redis 仓库
}

// NewDataRepo 根据配置创建连接；任一已配置连接失败时回收已创建资源并返回错误。
func NewDataRepo(ctx context.Context, logger *logger.Logger, conf *config.Config) (DataRepo, error) {
	if conf == nil {
		return nil, fmt.Errorf("数据连接配置不能为空")
	}
	timeout, err := startup.Timeout(conf.App.StartupTimeoutMS, startup.DefaultTimeout)
	if err != nil {
		return nil, fmt.Errorf("依赖初始化总期限无效: %w", err)
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	return newDataRepo(ctx, logger, conf.DB, conf.Redis, db.NewDb, cache.NewRedis)
}

// newDataRepo 创建数据仓库实例，根据配置打开数据库和 Redis 连接。
// 如果任一连接打开失败，回收已创建资源并返回错误。
func newDataRepo(ctx context.Context, log *logger.Logger, dbConfigs map[string]autoload.DB, redisConfigs map[string]autoload.Redis,
	openDB func(context.Context, string, autoload.DB, *logger.Logger) (db.Db, error),
	openRedis func(context.Context, string, autoload.Redis, *logger.Logger) (cache.Redis, error)) (DataRepo, error) {
	repo := &dataRepo{
		db:    &dbRepo{resource: make(map[string]db.Db, len(dbConfigs))},
		redis: &redisRepo{resource: make(map[string]cache.Redis, len(redisConfigs))},
	}
	fail := func(err error) (DataRepo, error) { return nil, errors.Join(err, repo.closeConnections()) }
	if err := ctx.Err(); err != nil {
		return fail(err)
	}
	if len(dbConfigs) == 0 {
		log.UseApp(ctx).Warn("目前 Db.repo 未配置连接")
	}
	// 固定创建顺序，便于复现失败及验证资源回收。
	for _, name := range slices.Sorted(maps.Keys(dbConfigs)) {
		if err := ctx.Err(); err != nil {
			return fail(err)
		}
		connection, err := openDB(ctx, name, dbConfigs[name], log)
		if connection != nil {
			repo.db.resource[name] = connection
		}
		if err != nil {
			log.UseApp(ctx).Error(fmt.Sprintf("初始化 Db.repo %s 失败", name), zap.Error(err))
			return fail(fmt.Errorf("初始化 Db.repo %s 失败: %w", name, err))
		}
		if connection == nil {
			return fail(fmt.Errorf("初始化 Db.repo %s 返回空连接", name))
		}
		log.UseApp(ctx).Info(fmt.Sprintf("初始化 Db.repo %s 成功", name))
	}
	if len(redisConfigs) == 0 {
		log.UseApp(ctx).Warn("目前 Redis.repo 未配置连接")
	}
	for _, name := range slices.Sorted(maps.Keys(redisConfigs)) {
		if err := ctx.Err(); err != nil {
			return fail(err)
		}
		connection, err := openRedis(ctx, name, redisConfigs[name], log)
		if connection != nil {
			repo.redis.resource[name] = connection
		}
		if err != nil {
			log.UseApp(ctx).Error(fmt.Sprintf("初始化 Redis.repo %s 失败", name), zap.Error(err))
			return fail(fmt.Errorf("初始化 Redis.repo %s 失败: %w", name, err))
		}
		if connection == nil {
			return fail(fmt.Errorf("初始化 Redis.repo %s 返回空连接", name))
		}
		log.UseApp(ctx).Info(fmt.Sprintf("初始化 Redis.repo %s 成功", name))
	}
	if err := ctx.Err(); err != nil {
		return fail(err)
	}
	return repo, nil
}

// closeConnections 关闭所有已打开的数据库连接和 Redis 连接。
// 返回:
//   - error: 关闭连接时的错误，合并所有失败的关闭操作
func (repo *dataRepo) closeConnections() error {
	var failures []error
	for _, name := range slices.Sorted(maps.Keys(repo.redis.resource)) {
		if err := repo.redis.resource[name].Close(); err != nil {
			failures = append(failures, fmt.Errorf("关闭 Redis 连接 %s 失败: %w", name, err))
		}
	}
	for _, name := range slices.Sorted(maps.Keys(repo.db.resource)) {
		if err := repo.db.resource[name].Close(); err != nil {
			failures = append(failures, fmt.Errorf("关闭数据库连接 %s 失败: %w", name, err))
		}
	}
	return errors.Join(failures...)
}

// DB 获取指定名称的数据库连接。
//
// 参数:
//   - name: 连接名称
//
// 返回:
//   - db.Db: 数据库连接实例
func (repo *dataRepo) DB(name string) db.Db {
	if repo.db == nil {
		return nil
	}
	return repo.db.resource[name]
}

// DbRepo 获取数据库仓库。
//
// 返回:
//   - DbRepo: 数据库仓库实例
func (repo *dataRepo) DbRepo() DbRepo {
	return repo.db
}

// Redis 获取指定名称的 Redis 连接。
//
// 参数:
//   - name: 连接名称
//
// 返回:
//   - cache.Redis: Redis 连接实例
func (repo *dataRepo) Redis(name string) cache.Redis {
	if repo.redis == nil {
		return nil
	}
	return repo.redis.resource[name]
}

// RedisRepo 获取 Redis 仓库。
//
// 返回:
//   - RedisRepo: Redis 仓库实例
func (repo *dataRepo) RedisRepo() RedisRepo {
	return repo.redis
}
