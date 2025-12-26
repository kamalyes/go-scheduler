# 核心概念

## Job 接口

所有任务都需要实现 `job.Job` 接口：

```go
type Job interface {
    Name() string                       // 任务唯一标识
    Description() string                // 任务描述
    Execute(ctx context.Context) error // 执行逻辑
}
```

### BaseJob

框架提供了基础实现 `BaseJob`，你只需组合它并实现 `Execute` 方法：

```go
type MyJob struct {
    *job.BaseJob
    config MyConfig
}

func NewMyJob(config MyConfig) *MyJob {
    return &MyJob{
        BaseJob: job.NewBaseJob("my-job", "我的任务"),
        config:  config,
    }
}

func (j *MyJob) Execute(ctx context.Context) error {
    // 你的业务逻辑
    return nil
}
```

### 扩展接口

框架提供了多个可选接口，用于扩展 Job 的能力：

#### ConfigurableJob - 支持动态配置

```go
type ConfigurableJob interface {
    Job
    LoadConfig(ctx context.Context, config interface{}) error
}
```

#### RetryableJob - 自定义重试逻辑

```go
type RetryableJob interface {
    Job
    ShouldRetry(err error) bool
}
```

#### ShardingJob - 支持分片执行

```go
type ShardingJob interface {
    Job
    FetchShardingItems(ctx context.Context, sharding ShardingContext) ([]interface{}, error)
    ProcessShardingItem(ctx context.Context, item interface{}, sharding ShardingContext) error
}
```

## Schedule 调度规范

### Cron 表达式

支持 5 字段和 6 字段（带秒）格式：

```
# 6 字段格式（秒 分 时 日 月 周）
秒   分   时   日   月   周
*    *    *    *    *    *

# 5 字段格式（分 时 日 月 周）
分   时   日   月   周
*    *    *    *    *
```

**示例：**

```go
"0 */5 * * * *"    // 每5分钟执行
"0 0 2 * * *"      // 每天凌晨2点执行
"0 0 0 1 * *"      // 每月1日零点执行
"0 30 9 * * 1-5"   // 工作日上午9:30执行
```

### 预定义调度

```go
@yearly    // 0 0 0 1 1 *    每年执行
@annually  // 同 @yearly
@monthly   // 0 0 0 1 * *    每月执行
@weekly    // 0 0 0 * * 0    每周执行
@daily     // 0 0 0 * * *    每天执行
@midnight  // 同 @daily
@hourly    // 0 0 * * * *    每小时执行
```

### 固定间隔调度

```go
schedule := scheduler.Every(5 * time.Minute) // 每5分钟
```

## 任务配置 (TaskCfg)

```go
type TaskCfg struct {
    // 基础配置
    Enabled     bool   `yaml:"enabled"`      // 是否启用
    CronSpec    string `yaml:"cron_spec"`    // Cron 表达式
    Description string `yaml:"description"`  // 任务描述

    // 执行控制
    Timeout        int  `yaml:"timeout"`         // 超时时间（秒）
    OverlapPrevent bool `yaml:"overlap_prevent"` // 防止重叠执行
    ImmediateStart bool `yaml:"immediate_start"` // 启动时立即执行

    // 重试配置
    MaxRetries    int `yaml:"max_retries"`    // 最大重试次数
    RetryInterval int `yaml:"retry_interval"` // 重试间隔（秒）
    RetryJitter   int `yaml:"retry_jitter"`   // 重试抖动（秒）

    // 优先级和依赖
    Priority     int      `yaml:"priority"`     // 优先级（数值越大越高）
    Dependencies []string `yaml:"dependencies"` // 依赖的任务列表

    // 分片配置
    ShardingEnabled bool `yaml:"sharding_enabled"` // 是否启用分片
    ShardingTotal   int  `yaml:"sharding_total"`   // 总分片数
}
```

## 装饰器模式

装饰器用于为 Job 添加额外功能，无需修改原始代码。

### 内置装饰器

#### 1. Recover - 恢复 panic

```go
chain := scheduler.NewChain(
    scheduler.Recover(logger),
)
```

#### 2. SkipIfStillRunning - 跳过正在运行的任务

```go
chain := scheduler.NewChain(
    scheduler.SkipIfStillRunning(logger),
)
```

#### 3. DelayIfStillRunning - 延迟等待

```go
chain := scheduler.NewChain(
    scheduler.DelayIfStillRunning(logger),
)
```

#### 4. WithTimeout - 超时控制

```go
chain := scheduler.NewChain(
    scheduler.WithTimeout(30 * time.Second),
)
```

#### 5. WithMetrics - 指标收集

```go
metrics := breaker.NewMetricsCollector()
chain := scheduler.NewChain(
    scheduler.WithMetrics(metrics),
)
```

#### 6. WithHooks - 执行钩子

```go
chain := scheduler.NewChain(
    scheduler.WithHooks(
        func(ctx context.Context, jobName string) { /* before */ },
        func(ctx context.Context, jobName string, err error) { /* after */ },
    ),
)
```

### 自定义装饰器

```go
func MyCustomDecorator(logger logger.Logger) scheduler.JobWrapper {
    return func(j job.Job) job.Job {
        return &wrappedJob{
            name:        j.Name(),
            description: j.Description(),
            execute: func(ctx context.Context) error {
                // 前置处理
                logger.Info("开始执行: %s", j.Name())
                
                // 执行原始任务
                err := j.Execute(ctx)
                
                // 后置处理
                if err != nil {
                    logger.Error("执行失败: %s, error=%v", j.Name(), err)
                } else {
                    logger.Info("执行成功: %s", j.Name())
                }
                
                return err
            },
        }
    }
}

// 使用
sched := scheduler.NewCronScheduler(
    scheduler.WithChain(
        scheduler.NewChain(
            MyCustomDecorator(logger),
            scheduler.Recover(logger),
        ),
    ),
)
```

## 任务保护器 (JobProtector)

提供熔断和限流功能：

```go
protector := scheduler.NewJobProtector("my-job").
    WithCircuitBreaker(breaker.Config{
        MaxFailures:  3,              // 最大失败次数
        ResetTimeout: 30 * time.Second, // 重置时间
    }).
    WithMetrics(metrics)

// 应用到调度器
sched.SetJobProtector("my-job", protector)
```

### 熔断状态

- **Closed（关闭）**：正常执行
- **Open（打开）**：失败次数超过阈值，拒绝执行
- **HalfOpen（半开）**：尝试恢复，允许少量请求通过

## 执行模式

### 1. Cron 调度

最常用的定时执行模式：

```go
sched.RegisterJob(job, jobs.TaskCfg{
    CronSpec: "0 */5 * * * *",
    Enabled:  true,
})
```

### 2. 手动触发

立即执行一次：

```go
err := sched.TriggerManual(ctx, "job-name")
```

### 3. 定时执行（XXL-JOB 风格）

指定时间执行指定次数：

```go
executeTime := time.Now().Add(5 * time.Minute)
err := sched.TriggerScheduled(ctx, "job-name", executeTime, 3)
// 5分钟后执行3次，每次间隔1秒
```

### 4. 固定间隔

不使用 Cron 表达式，直接指定时间间隔：

```go
schedule := scheduler.Every(5 * time.Minute)
entry := &scheduler.Entry{
    JobName:  "my-job",
    Schedule: schedule,
    Job:      myJob,
}
```

## 存储层

### 内存存储

适用于开发和测试：

```go
repo := repository.NewMemoryJobConfigRepository()
```

### 数据库存储

生产环境推荐：

```go
db, _ := gorm.Open(mysql.Open(dsn), &gorm.Config{})
repo, _ := repository.NewDatabaseJobConfigRepository(db)
```

### 缓存层

提供性能优化：

```go
cacheRepo := repository.NewRedisCacheRepository(redisClient)
```

## 事件系统

详见 [PubSub 使用指南](../PUBSUB_USAGE.md)

### 事件类型

- **配置更新** - `EventJobConfigUpdate`
- **CronSpec 更新** - `EventJobCronSpecUpdate`
- **手动执行** - `EventJobManualExecute`
- **定时执行** - `EventJobScheduledExecute`
- **任务删除** - `EventJobDeleted`
- **节点注册** - `EventNodeRegistered`
- **节点心跳** - `EventNodeHeartbeat`

## 下一步

- [快速开始](./QUICKSTART.md) - 开始使用框架
- [高级特性](./ADVANCED.md) - 深入了解高级功能
- [API 参考](./API.md) - 查看完整 API
