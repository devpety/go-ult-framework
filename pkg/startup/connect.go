// Package startup 提供依赖初始化的期限和重试控制。
package startup

import (
	"context"
	"errors"
	"fmt"
	"io"
	"math"
	"time"
)

const (
	// DefaultTimeout 是所有已配置依赖共用的默认初始化总预算。
	DefaultTimeout        = 30 * time.Second
	defaultAttemptTimeout = 5 * time.Second
)

// Timeout 将毫秒配置转换为初始化期限，零值使用默认值。
func Timeout(milliseconds int64, fallback time.Duration) (time.Duration, error) {
	if milliseconds < 0 || milliseconds > math.MaxInt64/int64(time.Millisecond) {
		return 0, errors.New("初始化超时毫秒数超出允许范围")
	}
	if milliseconds == 0 {
		return fallback, nil
	}
	return time.Duration(milliseconds) * time.Millisecond, nil
}

// Policy 是单个连接的初始化策略；所有尝试仍受调用方总期限约束。
type Policy struct {
	MaxAttempts    int
	RetryDelay     time.Duration
	AttemptTimeout time.Duration
}

// NewPolicy 校验初始化策略，零值分别使用三次尝试、一秒间隔和五秒单次期限。
func NewPolicy(attempts, delaySeconds int, timeoutMS int64) (Policy, error) {
	if attempts < 0 || delaySeconds < 0 || int64(delaySeconds) > math.MaxInt64/int64(time.Second) {
		return Policy{}, errors.New("初始化尝试次数和重试间隔无效")
	}
	if attempts == 0 {
		attempts = 3
	}
	if delaySeconds == 0 {
		delaySeconds = 1
	}
	timeout, err := Timeout(timeoutMS, defaultAttemptTimeout)
	if err != nil {
		return Policy{}, err
	}
	return Policy{MaxAttempts: attempts, RetryDelay: time.Duration(delaySeconds) * time.Second, AttemptTimeout: timeout}, nil
}

// Connect 同步执行可取消的连接尝试；失败或迟到的连接在下一次尝试前关闭。
// 工厂可同时返回资源和错误，将半初始化资源交给此处统一回收。
func Connect[T interface {
	io.Closer
	comparable
}](ctx context.Context, policy Policy,
	open func(context.Context) (T, error), onRetry func(int, error)) (T, error) {
	var zero T
	var lastErr error
	if policy.MaxAttempts <= 0 || policy.RetryDelay < 0 || policy.AttemptTimeout <= 0 {
		return zero, errors.New("初始化重试策略无效")
	}
	for attempt := 1; attempt <= policy.MaxAttempts; attempt++ {
		if err := ctx.Err(); err != nil {
			return zero, errors.Join(err, lastErr)
		}
		attemptCtx, cancel := context.WithTimeout(ctx, policy.AttemptTimeout)
		resource, err := open(attemptCtx)
		if err == nil {
			err = attemptCtx.Err()
		}
		if err == nil && resource == zero {
			err = errors.New("初始化工厂返回空连接")
		}
		cancel()
		if err == nil {
			return resource, nil
		}
		if resource != zero {
			err = errors.Join(err, resource.Close())
		}
		lastErr = fmt.Errorf("第 %d/%d 次初始化失败: %w", attempt, policy.MaxAttempts, err)
		if err := ctx.Err(); err != nil {
			return zero, errors.Join(err, lastErr)
		}
		if attempt == policy.MaxAttempts {
			return zero, lastErr
		}
		if onRetry != nil {
			onRetry(attempt, lastErr)
		}
		timer := time.NewTimer(policy.RetryDelay)
		select {
		case <-ctx.Done():
			timer.Stop()
			return zero, errors.Join(ctx.Err(), lastErr)
		case <-timer.C:
		}
	}
	return zero, errors.New("初始化尝试次数必须大于零")
}
