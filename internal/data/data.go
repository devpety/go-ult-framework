// Package data 提供数据层实现。
// 封装数据库和 Redis 连接，提供统一的数据访问接口。
package data

import (
	"context"
	"fmt"
	"maps"
	"slices"
	"sync"

	"ult/config"
	"ult/config/autoload"
	"ult/internal/app"
	"ult/internal/data/dbquery"
	"ult/pkg/cache"
	"ult/pkg/db"
	"ult/pkg/repositories"

	"github.com/devpety/go-utils/v3/cache/redis"
	"github.com/google/wire"
	"gorm.io/gorm"
)

// ProviderSet Wire 依赖注入提供者集合。
var ProviderSet = wire.NewSet(NewData, NewDataRepo)

var _ Data = (*dataImpl)(nil)

// Data 数据接口，定义数据访问操作。
type Data interface {
	DB() *dbquery.Query                   // 获取 GORM 查询器
	Redis() redis.Client                  // 获取 Redis 客户端
	GormDB() *gorm.DB                     // 获取 GORM DB 实例
	WithContext(ctx context.Context) Data // 创建带上下文的数据实例
	DataRepo() *DataRepo                  // 获取数据仓库
}

// dataImpl 数据实例实现。
type dataImpl struct {
	db       *dbquery.Query  // GORM 查询器
	gorm     *gorm.DB        // GORM DB 实例
	redis    redis.Client    // Redis 客户端
	ctx      context.Context // 上下文
	dataRepo *DataRepo       // 数据仓库
}

// DataRepo 数据仓库结构体，包含数据库和 Redis 仓库。
type DataRepo struct {
	DbRepo     repositories.DbRepo    // 数据库仓库
	RedisRepo  repositories.RedisRepo // Redis 仓库
	DBNames    []string               // 配置要求存在的连接，不能将丢失连接当作未配置
	RedisNames []string               // 配置要求存在的 Redis 连接
}

// NewData 创建新的数据实例。
// 初始化数据库和 Redis 连接，返回清理函数。
//
// 参数:
//   - c: 应用配置
//   - tools: 应用工具包
//
// 返回:
//   - Data: 数据实例
//   - func(): 清理函数
//   - error: 配置或连接初始化错误
func NewData(ctx context.Context, c *config.Config, tools *app.Tools) (Data, func(), error) {
	if c == nil {
		return nil, nil, fmt.Errorf("数据连接配置不能为空")
	}
	if err := validateDefaultConnections(c.DB, c.Redis); err != nil {
		return nil, nil, err
	}
	repo, err := repositories.NewDataRepo(ctx, tools.Logger(), c)
	if err != nil {
		return nil, nil, err
	}
	connections := &DataRepo{
		DbRepo:     repo.DbRepo(),
		RedisRepo:  repo.RedisRepo(),
		DBNames:    slices.Sorted(maps.Keys(c.DB)),
		RedisNames: slices.Sorted(maps.Keys(c.Redis)),
	}
	instance, cleanup := newDataInstance(connections, tools)
	return instance, cleanup, nil
}

func newDataInstance(connections *DataRepo, tools *app.Tools) (Data, func()) {
	ctx := context.Background()
	data := &dataImpl{dataRepo: connections.snapshot()}
	// 清理持有独立集合，不再依赖对外提供的数据仓库对象。
	var closeDB map[string]db.Db
	var closeRedis map[string]cache.Redis
	if data.dataRepo.DbRepo != nil {
		closeDB = maps.Clone(data.dataRepo.DbRepo.All())
	}
	if data.dataRepo.RedisRepo != nil {
		closeRedis = maps.Clone(data.dataRepo.RedisRepo.All())
	}

	data.gorm = newDefaultDb(data.dataRepo.DbRepo)
	if data.gorm != nil {
		data.db = dbquery.Use(data.gorm)
	}

	data.redis = newDefaultRedis(data.dataRepo.RedisRepo)

	data.ctx = context.Background()

	cleanup := sync.OnceFunc(func() {
		for dbName, dbRepo := range closeDB {
			if dbRepo != nil {
				if err := dbRepo.Close(); err != nil {
					tools.Logger().UseApp(ctx).Error(fmt.Sprintf("关闭数据库连接 %s 失败: %v", dbName, err))
				}
			}
			tools.Logger().UseApp(ctx).Info(fmt.Sprintf("关闭数据库连接: %s", dbName))
		}
		for redisName, redisRepo := range closeRedis {
			if redisRepo != nil {
				if err := redisRepo.Close(); err != nil {
					tools.Logger().UseApp(ctx).Error(fmt.Sprintf("关闭 Redis 连接 %s 失败: %v", redisName, err))
				}
			}
			tools.Logger().UseApp(ctx).Info(fmt.Sprintf("关闭 Redis 连接: %s", redisName))
		}
	})

	return data, cleanup
}

func validateDefaultConnections(dbConfigs map[string]autoload.DB, redisConfigs map[string]autoload.Redis) error {
	if len(dbConfigs) > 0 {
		if _, ok := dbConfigs[repositories.DbConnectionDefaultName]; !ok {
			return fmt.Errorf("已启用数据库，但缺少 default 连接")
		}
	}
	if len(redisConfigs) > 0 {
		if _, ok := redisConfigs[repositories.RedisConnectionDefaultName]; !ok {
			return fmt.Errorf("已启用 Redis，但缺少 default 连接")
		}
	}
	return nil
}

// NewDataRepo 从数据实例创建数据仓库。
//
// 参数:
//   - data: 数据实例
//
// 返回:
//   - *DataRepo: 数据仓库
func NewDataRepo(data Data) *DataRepo {
	return data.DataRepo()
}

// DB 获取 GORM 查询器。
//
// 返回:
//   - *dbquery.Query: GORM 查询器
func (d *dataImpl) DB() *dbquery.Query {
	return d.db
}

// Redis 获取 Redis 客户端。
//
// 返回:
//   - redis.Client: Redis 客户端
func (d *dataImpl) Redis() redis.Client {
	return d.redis
}

// GormDB 获取 GORM DB 实例。
//
// 返回:
//   - *gorm.DB: GORM DB 实例
func (d *dataImpl) GormDB() *gorm.DB {
	return d.gorm
}

// DataRepo 返回仓库视图的独立副本，连接实例仍由数据层统一管理。
//
// 返回:
//   - *DataRepo: 数据仓库
func (d *dataImpl) DataRepo() *DataRepo {
	return d.dataRepo.snapshot()
}

// snapshot 创建数据仓库的独立副本，连接实例仍由数据层统一管理。
//
// 返回:
//   - *DataRepo: 数据仓库
func (repo *DataRepo) snapshot() *DataRepo {
	snapshot := *repo
	snapshot.DBNames = slices.Clone(repo.DBNames)
	snapshot.RedisNames = slices.Clone(repo.RedisNames)
	return &snapshot
}

// WithContext 创建带上下文的数据实例。
//
// 参数:
//   - ctx: 上下文
//
// 返回:
//   - Data: 带上下文的数据实例
func (d *dataImpl) WithContext(ctx context.Context) Data {
	var db *dbquery.Query
	var gormDB *gorm.DB
	if d.gorm != nil {
		gormDB = d.gorm.WithContext(ctx)
		// gorm/gen 初始化查询器时会重置 Context，需重新绑定带上下文的 DB。
		db = dbquery.Use(gormDB).ReplaceDB(gormDB)
	}
	return &dataImpl{
		db:       db,
		gorm:     gormDB,
		redis:    d.redis,
		ctx:      ctx,
		dataRepo: d.dataRepo,
	}
}

// newDefaultDb 获取默认数据库连接的 GORM DB 实例。
//
// 参数:
//   - dbRepo: 数据库仓库
//
// 返回:
//   - *gorm.DB: GORM DB 实例
func newDefaultDb(dbRepo repositories.DbRepo) *gorm.DB {
	if dbRepo == nil {
		return nil
	}
	dbConn := dbRepo.DB(repositories.DbConnectionDefaultName)
	if dbConn == nil {
		return nil
	}
	return dbConn.Get().DB()
}

// newDefaultRedis 获取默认 Redis 连接的客户端实例。
//
// 参数:
//   - redisRepo: Redis 仓库
//
// 返回:
//   - redis.Client: Redis 客户端
func newDefaultRedis(redisRepo repositories.RedisRepo) redis.Client {
	if redisRepo == nil {
		return nil
	}
	redisConn := redisRepo.Redis(repositories.RedisConnectionDefaultName)
	if redisConn == nil {
		return nil
	}
	return redisConn.Get()
}
