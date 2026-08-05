/*
 * @Author: kamalyes 501893067@qq.com
 * @Date: 2025-12-23 21:45:00
 * @LastEditors: kamalyes 501893067@qq.com
 * @LastEditTime: 2025-12-23 21:53:06
 * @FilePath: \kronos-scheduler\scheduler\distributed_lock.go
 * @Description: 分布式锁装饰器 - 基于 go-cachex LockManager，支持任务互斥和分片
 *
 * Copyright (c) 2025 by kamalyes, All Rights Reserved.
 */

package scheduler

import (
	"context"
	"errors"
	"fmt"

	"github.com/kamalyes/go-cachex"
	"github.com/kamalyes/go-toolbox/pkg/mathx"
	"github.com/kamalyes/kronos-scheduler/job"
	"github.com/kamalyes/kronos-scheduler/logger"
)

// WithCachexLock 使用 go-cachex LockManager 的分布式锁装饰器
// 集成了看门狗机制，自动续期，确保任务在集群中只执行一次
//
// 行为：使用非阻塞 TryLock，同一调度时刻只有一个 Pod 抢锁成功并执行，
// 其他 Pod 抢锁失败后立即跳过本轮，不会阻塞等待（避免多 Pod 串行执行同一任务）
func WithCachexLock(lockMgr *cachex.LockManager, log Logger) JobWrapper {
	log = mathx.IF(log == nil, logger.NewNoOpLogger(), log)
	return func(j job.Job) job.Job {
		return &wrappedJob{
			name:        j.Name(),
			description: j.Description(),
			execute: func(ctx context.Context) error {
				lockKey := j.Name()

				// 使用 LockManager 获取锁(自动支持看门狗)
				lock := lockMgr.GetLock(lockKey)

				// 非阻塞 TryLock：抢锁失败立即跳过本轮调度
				// 避免阻塞式 Lock 导致多个 Pod 串行执行同一任务
				acquired, err := lock.TryLock(ctx)
				if err != nil {
					return fmt.Errorf("%w: %s: %v", ErrLockAcquireFailed, j.Name(), err)
				}
				if !acquired {
					// 锁被其他节点持有，跳过本次执行（非错误，正常竞争行为）
					log.InfoContext(ctx, "分布式锁被其他节点持有，跳过本轮: %s", j.Name())
					return nil
				}

				// 确保释放锁，记录释放异常（不影响任务执行结果）
				defer func() {
					if unlockErr := lock.Unlock(ctx); unlockErr != nil {
						// ErrLockNotFound/ErrLockNotOwned 表示锁已过期、被清理或所有权丢失，
						// 均属于看门狗续期失败后的正常情况，静默跳过避免噪音日志
						if errors.Is(unlockErr, cachex.ErrLockNotFound) || errors.Is(unlockErr, cachex.ErrLockNotOwned) {
							return
						}
						log.WarnContext(ctx, "分布式锁释放异常: %s, err=%v", j.Name(), unlockErr)
					}
				}()

				// 执行任务(LockManager 自动处理看门狗续期)
				return j.Execute(ctx)
			},
		}
	}
}

// contextKey 自定义上下文键类型，避免键冲突
type contextKey string

const (
	// shardIndexKey 分片索引键
	shardIndexKey contextKey = "shardIndex"
	// shardTotalKey 总分片数键
	shardTotalKey contextKey = "shardTotal"
)

// ShardIndexKey 获取分片索引上下文键(供外部使用)
func ShardIndexKey() contextKey {
	return shardIndexKey
}

// ShardTotalKey 获取总分片数上下文键(供外部使用)
func ShardTotalKey() contextKey {
	return shardTotalKey
}

// ShardingContext 分片上下文
type ShardingContext struct {
	ShardIndex int // 当前分片索引
	ShardTotal int // 总分片数
}

// WithSharding 分片装饰器 - 支持任务分片执行
func WithSharding(getShardContext func() *ShardingContext) JobWrapper {
	return func(j job.Job) job.Job {
		return &wrappedJob{
			name:        j.Name(),
			description: j.Description(),
			execute: func(ctx context.Context) error {
				shardCtx := getShardContext()
				if shardCtx == nil {
					// 无分片配置，正常执行
					return j.Execute(ctx)
				}

				// 将分片信息注入上下文
				ctx = context.WithValue(ctx, shardIndexKey, shardCtx.ShardIndex)
				ctx = context.WithValue(ctx, shardTotalKey, shardCtx.ShardTotal)

				return j.Execute(ctx)
			},
		}
	}
}

// GetShardIndex 从上下文获取分片索引
func GetShardIndex(ctx context.Context) (int, bool) {
	val := ctx.Value(shardIndexKey)
	if val == nil {
		return 0, false
	}
	index, ok := val.(int)
	return index, ok
}

// GetShardTotal 从上下文获取总分片数
func GetShardTotal(ctx context.Context) (int, bool) {
	val := ctx.Value(shardTotalKey)
	if val == nil {
		return 0, false
	}
	total, ok := val.(int)
	return total, ok
}
