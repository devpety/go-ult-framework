// Package action 提供带有明确租约和取消边界的 Redis 分布式锁。
//
// 典型使用：
//
//	lock, err := NewLock(client, "order:123", 10*time.Second)
//	if err != nil {
//		return err
//	}
//
//	acquired, err := lock.TryLock(ctx, 100*time.Millisecond, 2*time.Second)
//	if err != nil {
//		return err // Redis 故障或 ctx 已取消
//	}
//	if !acquired {
//		// 锁被占用，按业务策略重试或放弃
//		return nil
//	}
//
//	defer lock.Unlock(context.Background()) // 幂等释放，避免忘记
//
//	// 临界区处理，必须在租约内完成；长任务可中途自查是否仍持有：
//	if held, _ := lock.IsHeldByMe(ctx); !held {
//		return errors.New("锁已失效")
//	}
package action

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/devpety/go-utils/v3/cache/redis"
	"github.com/google/uuid"
)

var (
	// ErrLockInUse 表示同一实例存在并发操作，调用方应串行管理租约。
	ErrLockInUse = errors.New("当前锁实例正在执行其他操作")
	// ErrLockAlreadyHeld 表示当前实例尚未释放上一次获取的租约。
	ErrLockAlreadyHeld = errors.New("当前锁实例已持有锁")
	// ErrLockNotOwned 表示租约已经失效，调用方不能继续受保护的操作。
	ErrLockNotOwned = errors.New("锁已过期或所有权已转移")
	// ErrLockLeaseExpired 表示网络往返已经耗尽本次获取的租约预算。
	ErrLockLeaseExpired = errors.New("获取结果返回时锁租约已耗尽")
)

const unlockTimeout = 2 * time.Second

// 同一令牌的命令重放只读取剩余租约，不续期；其他令牌不能获取已有锁。
const acquireScript = `
local current = redis.call("GET", KEYS[1])
if current and current ~= ARGV[1] then
    return 0
end
if not current then
    redis.call("SET", KEYS[1], ARGV[1], "PX", ARGV[2])
end
local remaining = redis.call("PTTL", KEYS[1])
if remaining <= 0 then
    return -1
end
return remaining
`

const unlockScript = `
local current = redis.call("GET", KEYS[1])
if not current then
    return 1
end
if current == ARGV[1] then
    return redis.call("DEL", KEYS[1])
end
return 0
`

// Lock 管理一次独占租约，不自动续期；临界区必须在租约内完成。
// 同一实例不能重复获取或并发变更所有权，每次获取使用独立令牌。
type Lock struct {
	client    redis.Client
	key       string
	duration  time.Duration
	mu        sync.Mutex
	value     string
	expiresAt time.Time
}

// NewLock 创建锁；租约至少为一毫秒，并按 Redis 的毫秒精度向下取整。
func NewLock(client redis.Client, key string, duration time.Duration) (*Lock, error) {
	if client == nil {
		return nil, errors.New("Redis 客户端不能为空")
	}
	if key == "" {
		return nil, errors.New("锁键名不能为空")
	}
	if duration < time.Millisecond {
		return nil, errors.New("锁租约不能小于一毫秒")
	}
	return &Lock{client: client, key: key, duration: duration.Truncate(time.Millisecond)}, nil
}

// Lock 执行一次获取；false、nil 表示锁被占用，Redis 故障和取消均返回错误。
func (lock *Lock) Lock(ctx context.Context) (bool, error) {
	if err := ctx.Err(); err != nil {
		return false, err
	}
	if !lock.mu.TryLock() {
		return false, ErrLockInUse
	}
	defer lock.mu.Unlock()
	if lock.value != "" {
		return false, ErrLockAlreadyHeld
	}

	value := uuid.NewString()
	started := time.Now()
	remainingMS, err := lock.client.Eval(ctx, acquireScript, []string{lock.key}, value, lock.duration.Milliseconds()).Int64()
	if err != nil {
		return false, fmt.Errorf("获取 Redis 锁失败: %w", err)
	}
	// 不将迟到的成功结果授予调用方；不确定写入由正租约自动回收。
	if err := ctx.Err(); err != nil {
		return false, err
	}
	if remainingMS == 0 {
		return false, nil
	}
	if remainingMS < 0 {
		return false, ErrLockLeaseExpired
	}
	// 将剩余租约再扣除整个往返耗时，保守覆盖底层重试及响应延迟。
	expiresAt := started.Add(time.Duration(min(remainingMS, lock.duration.Milliseconds())) * time.Millisecond)
	if !time.Now().Before(expiresAt) {
		return false, ErrLockLeaseExpired
	}
	lock.value = value
	lock.expiresAt = expiresAt
	return true, nil
}

// TryLock 在调用方期限和 timeout 中较早的期限内重试，取消可中断重试等待。
func (lock *Lock) TryLock(ctx context.Context, retryInterval, timeout time.Duration) (bool, error) {
	if retryInterval <= 0 || timeout <= 0 {
		return false, errors.New("重试间隔和获取超时必须大于零")
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	for {
		acquired, err := lock.Lock(ctx)
		if err != nil || acquired {
			return acquired, err
		}
		timer := time.NewTimer(retryInterval)
		select {
		case <-ctx.Done():
			timer.Stop()
			return false, ctx.Err()
		case <-timer.C:
		}
	}
}

// Unlock 使用独立的两秒清理期限，保留上下文值但不继承请求取消。
// 未持有锁或键已不存在时幂等返回；Redis 故障保留令牌以便重试，其他令牌持有时返回所有权丢失。
func (lock *Lock) Unlock(ctx context.Context) error {
	if !lock.mu.TryLock() {
		return ErrLockInUse
	}
	defer lock.mu.Unlock()
	if lock.value == "" {
		return nil
	}
	cleanupCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), unlockTimeout)
	defer cancel()
	deleted, err := lock.client.Eval(cleanupCtx, unlockScript, []string{lock.key}, lock.value).Int64()
	if err != nil {
		return fmt.Errorf("释放 Redis 锁失败: %w", err)
	}
	lock.value = ""
	lock.expiresAt = time.Time{}
	if deleted == 0 {
		return ErrLockNotOwned
	}
	return nil
}

// IsLocked 返回键是否存在的即时快照，不能代替原子获取。
func (lock *Lock) IsLocked(ctx context.Context) (bool, error) {
	count, err := lock.client.Exists(ctx, lock.key).Result()
	if err != nil {
		return false, fmt.Errorf("检查 Redis 锁失败: %w", err)
	}
	return count > 0, nil
}

// IsHeldByMe 返回当前实例是否仍持有租约的即时快照。
func (lock *Lock) IsHeldByMe(ctx context.Context) (bool, error) {
	if !lock.mu.TryLock() {
		return false, ErrLockInUse
	}
	defer lock.mu.Unlock()
	if lock.value == "" || !time.Now().Before(lock.expiresAt) {
		return false, nil
	}
	const script = `if redis.call("GET", KEYS[1]) == ARGV[1] then return 1 end return 0`
	owned, err := lock.client.Eval(ctx, script, []string{lock.key}, lock.value).Int64()
	if err != nil {
		return false, fmt.Errorf("检查 Redis 锁所有权失败: %w", err)
	}
	return owned == 1 && time.Now().Before(lock.expiresAt), nil
}
