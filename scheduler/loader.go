/*
 * @Author: kamalyes 501893067@qq.com
 * @Date: 2025-12-25 17:25:00
 * @LastEditors: kamalyes 501893067@qq.com
 * @LastEditTime: 2025-12-25 17:01:59
 * @FilePath: \go-scheduler\scheduler\loader.go
 * @Description: 配置加载器 - 启动时加载配置的策略
 *
 * Copyright (c) 2025 by kamalyes, All Rights Reserved.
 */

package scheduler

import (
	"context"
	"fmt"

	"github.com/kamalyes/go-config/pkg/jobs"
	"github.com/kamalyes/go-scheduler/models"
)

// LoadStrategy 配置加载策略
type LoadStrategy string

const (
	// LoadStrategyLocalFirst 优先使用本地配置，DB/Redis 不存在时保存本地配置
	LoadStrategyLocalFirst LoadStrategy = "local_first"

	// LoadStrategyRemoteFirst 优先使用 DB/Redis 配置，存在时覆盖本地配置
	LoadStrategyRemoteFirst LoadStrategy = "remote_first"

	// LoadStrategyLocalOnly 只使用本地配置，不加载也不保存到 DB/Redis
	LoadStrategyLocalOnly LoadStrategy = "local_only"

	// LoadStrategyRemoteOnly 只使用 DB/Redis 配置，本地配置不生效
	LoadStrategyRemoteOnly LoadStrategy = "remote_only"
)

// Loader 配置加载器
type Loader struct {
	scheduler *CronScheduler
	strategy  LoadStrategy
}

// newLoader 创建配置加载器
func newLoader(scheduler *CronScheduler, strategy LoadStrategy) *Loader {
	if strategy == "" {
		strategy = LoadStrategyLocalFirst // 默认策略
	}
	return &Loader{
		scheduler: scheduler,
		strategy:  strategy,
	}
}

// LoadConfigs 加载配置
func (l *Loader) LoadConfigs(ctx context.Context) error {
	switch l.strategy {
	case LoadStrategyLocalOnly:
		return l.loadLocalOnly(ctx)
	case LoadStrategyRemoteOnly:
		return l.loadRemoteOnly(ctx)
	case LoadStrategyLocalFirst:
		return l.loadLocalFirst(ctx)
	case LoadStrategyRemoteFirst:
		return l.loadRemoteFirst(ctx)
	default:
		return fmt.Errorf("%w: %s", ErrUnsupportedLoadStrategy, l.strategy)
	}
}

// loadLocalOnly 只使用本地配置
func (l *Loader) loadLocalOnly(ctx context.Context) error {
	if l.scheduler.globalConfig == nil {
		return nil
	}

	l.scheduler.logger.Infof("📋 使用本地配置: 任务数=%d", len(l.scheduler.globalConfig.Tasks))
	return nil
}

// loadRemoteOnly 只使用远程配置
func (l *Loader) loadRemoteOnly(ctx context.Context) error {
	if l.scheduler.jobRepo == nil {
		return ErrJobRepoNotConfigured
	}

	// 从数据库加载所有配置
	configs, err := l.scheduler.jobRepo.ListAll(ctx)
	if err != nil {
		return fmt.Errorf("%w: %w", ErrLoadConfigFromDB, err)
	}

	l.scheduler.logger.Infof("📋 从数据库加载配置: 任务数=%d", len(configs))

	// 转换为 globalConfig
	tasksMap := make(map[string]jobs.TaskCfg)
	for _, config := range configs {
		taskCfg, err := config.ToTaskCfg()
		if err != nil || taskCfg == nil {
			l.scheduler.logger.Warnf("⚠️ 转换配置失败: %s", config.JobName)
			continue
		}
		tasksMap[config.JobName] = *taskCfg
	}

	// 替换 globalConfig
	if l.scheduler.globalConfig == nil {
		l.scheduler.globalConfig = &jobs.Jobs{
			Enabled: true,
			Tasks:   tasksMap,
		}
	} else {
		l.scheduler.globalConfig.Tasks = tasksMap
	}

	return nil
}

// loadLocalFirst 优先本地配置
func (l *Loader) loadLocalFirst(ctx context.Context) error {
	if l.scheduler.globalConfig == nil || len(l.scheduler.globalConfig.Tasks) == 0 {
		// 本地没有配置，尝试从远程加载
		return l.loadRemoteOnly(ctx)
	}

	// 本地有配置，检查远程是否存在
	if l.scheduler.jobRepo != nil {
		for taskName, taskCfg := range l.scheduler.globalConfig.Tasks {
			// 检查远程是否已存在
			_, err := l.scheduler.jobRepo.GetByJobName(ctx, taskName)
			if err != nil {
				// 远程不存在，保存本地配置到远程
				dbConfig := models.FromTaskCfg(taskName, &taskCfg)
				if _, saveErr := l.scheduler.jobRepo.EnsureConfigExists(ctx, dbConfig); saveErr != nil {
					l.scheduler.logger.Warnf("⚠️ 保存配置到数据库失败: %s, error=%v", taskName, saveErr)
				} else {
					l.scheduler.logger.Infof("📤 本地配置已同步到数据库: %s", taskName)
				}

				// 同步到缓存
				if l.scheduler.schedulerCache != nil {
					l.scheduler.schedulerCache.SetTaskConfig(ctx, taskName, taskCfg)
				}
			}
		}
	}

	l.scheduler.logger.Infof("📋 使用本地配置: 任务数=%d", len(l.scheduler.globalConfig.Tasks))
	return nil
}

// loadRemoteFirst 优先远程配置
func (l *Loader) loadRemoteFirst(ctx context.Context) error {
	if l.scheduler.jobRepo == nil {
		// 没有远程仓库，使用本地配置
		return l.loadLocalOnly(ctx)
	}

	// 尝试从远程加载
	configs, err := l.scheduler.jobRepo.ListAll(ctx)
	if err != nil || len(configs) == 0 {
		// 远程没有配置，使用本地配置并保存
		if l.scheduler.globalConfig != nil && len(l.scheduler.globalConfig.Tasks) > 0 {
			l.scheduler.logger.Infof("📋 远程配置为空，使用本地配置并同步")
			return l.loadLocalFirst(ctx)
		}
		return nil
	}

	// 远程有配置，覆盖本地
	l.scheduler.logger.Infof("📋 使用远程配置覆盖本地: 任务数=%d", len(configs))

	tasksMap := make(map[string]jobs.TaskCfg)
	for _, config := range configs {
		taskCfg, convErr := config.ToTaskCfg()
		if convErr != nil || taskCfg == nil {
			l.scheduler.logger.Warnf("⚠️ 转换配置失败: %s", config.JobName)
			continue
		}
		tasksMap[config.JobName] = *taskCfg

		// 更新到缓存
		if l.scheduler.schedulerCache != nil {
			l.scheduler.schedulerCache.SetTaskConfig(ctx, config.JobName, *taskCfg)
		}
	}

	// 替换或合并 globalConfig
	if l.scheduler.globalConfig == nil {
		l.scheduler.globalConfig = &jobs.Jobs{
			Enabled: true,
			Tasks:   tasksMap,
		}
	} else {
		// 合并配置（远程优先）
		for name, cfg := range tasksMap {
			l.scheduler.globalConfig.Tasks[name] = cfg
		}
	}

	return nil
}

// ReloadJobConfig 重新加载单个任务配置
func (l *Loader) ReloadJobConfig(ctx context.Context, jobName string) error {
	if l.scheduler.jobRepo == nil {
		return ErrJobRepoNotConfigured
	}

	// 从数据库加载
	config, err := l.scheduler.jobRepo.GetByJobName(ctx, jobName)
	if err != nil {
		return fmt.Errorf("%w: %w", ErrLoadConfigFromDB, err)
	}

	taskCfg, err := config.ToTaskCfg()
	if err != nil || taskCfg == nil {
		return fmt.Errorf("%w: %w", ErrConvertConfig, err)
	}

	// 更新 globalConfig
	if l.scheduler.globalConfig != nil {
		l.scheduler.globalConfig.Tasks[jobName] = *taskCfg
	}

	// 更新缓存
	if l.scheduler.schedulerCache != nil {
		l.scheduler.schedulerCache.SetTaskConfig(ctx, jobName, *taskCfg)
	}

	l.scheduler.logger.Infof("✅ 配置已重载: %s", jobName)
	return nil
}
