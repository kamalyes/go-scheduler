# 快速开始

## 安装

```bash
go get github.com/kamalyes/go-scheduler
```

## 基础使用（内存存储）

### 1. 定义你的 Job

```go
package main

import (
    "context"
    "log"
    "time"

    "github.com/kamalyes/go-scheduler/job"
)

type CleanupJob struct {
    *job.BaseJob
}

func NewCleanupJob() *CleanupJob {
    return &CleanupJob{
        BaseJob: job.NewBaseJob("cleanup", "清理过期数据"),
    }
}

func (j *CleanupJob) Execute(ctx context.Context) error {
    log.Println("执行清理任务...")
    time.Sleep(2 * time.Second)
    log.Println("清理完成")
    return nil
}
```

### 2. 创建调度器

```go
package main

import (
    "context"
    "log"
    
    "github.com/kamalyes/go-config/pkg/jobs"
    "github.com/kamalyes/go-scheduler/scheduler"
    "github.com/kamalyes/go-scheduler/pubsub"
    "github.com/kamalyes/go-scheduler/logger"
)

func main() {
    // 创建本地 PubSub
    ps := pubsub.NewLocalPubSub()
    defer ps.Close()

    // 创建调度器
    sched := scheduler.NewCronScheduler(
        scheduler.WithPubSub(ps),
        scheduler.WithLoggerOption(logger.NewSchedulerLogger("MyScheduler")),
    )

    // 注册 Job
    err := sched.RegisterJob(NewCleanupJob(), jobs.TaskCfg{
        CronSpec:       "0 */5 * * * *", // 每5分钟执行
        Enabled:        true,
        OverlapPrevent: true,
        Timeout:        60,
    })
    if err != nil {
        log.Fatal(err)
    }

    // 启动调度器
    if err := sched.Start(); err != nil {
        log.Fatal(err)
    }
    defer sched.Stop()

    // 手动触发执行
    sched.TriggerManual(context.Background(), "cleanup")

    // 保持运行
    select {}
}
```

## 生产环境配置（数据库 + Redis）

### 1. 初始化依赖

```go
package main

import (
    "context"
    
    "github.com/kamalyes/go-scheduler/scheduler"
    "github.com/kamalyes/go-scheduler/repository"
    "github.com/kamalyes/go-scheduler/pubsub"
    "github.com/kamalyes/go-cachex"
    "github.com/redis/go-redis/v9"
    "gorm.io/driver/mysql"
    "gorm.io/gorm"
)

func main() {
    // 1. 初始化数据库
    db, err := gorm.Open(mysql.Open(
        "user:password@tcp(127.0.0.1:3306)/jobdb?charset=utf8mb4&parseTime=True",
    ), &gorm.Config{})
    if err != nil {
        panic(err)
    }

    // 2. 初始化 Redis
    redisClient := redis.NewClient(&redis.Options{
        Addr: "localhost:6379",
    })

    // 3. 创建 PubSub
    cachexPS := cachex.NewPubSub(redisClient)
    ps := pubsub.NewRedisPubSub(cachexPS)
    defer ps.Close()

    // 4. 创建仓储
    jobRepo, _ := repository.NewDatabaseJobConfigRepository(db)
    snapshotRepo, _ := repository.NewExecutionSnapshotRepository(db, nil)
    cacheRepo := repository.NewRedisCacheRepository(redisClient)

    // 5. 创建调度器缓存
    schedulerCache := repository.NewSchedulerCache(redisClient)

    // 6. 创建节点注册器（分布式模式）
    nodeRegistry := scheduler.NewNodeRegistry(
        schedulerCache,
        logger.NewSchedulerLogger("Scheduler"),
        "node-1",
        "192.168.1.100",
    )

    // 7. 创建调度器
    sched := scheduler.NewCronScheduler(
        scheduler.WithRepositories(jobRepo, snapshotRepo, cacheRepo),
        scheduler.WithSchedulerCache(schedulerCache),
        scheduler.WithPubSub(ps),
        scheduler.WithDistributed(nodeRegistry),
        scheduler.WithMaxConcurrency(10),
    )

    // 注册 Job 并启动
    // ...
}
```

## 函数式 Job

快速创建简单任务：

```go
import "github.com/kamalyes/go-scheduler/job"

// 方式 1: 使用函数
printJob := job.NewFuncJob("print", "打印任务", func(ctx context.Context) error {
    fmt.Println("Hello from Job!")
    return nil
})

// 方式 2: 使用闭包
counter := 0
countJob := job.NewFuncJob("counter", "计数任务", func(ctx context.Context) error {
    counter++
    fmt.Printf("执行次数: %d\n", counter)
    return nil
})

// 注册
sched.RegisterJob(printJob, jobs.TaskCfg{
    CronSpec: "0 * * * * *", // 每分钟执行
    Enabled:  true,
})
```

## 使用 go-config 配置

### 1. 定义配置文件

`config.yaml`:

```yaml
jobs:
  enabled: true
  time_zone: "Asia/Shanghai"
  max_concurrent_jobs: 10
  max_retries: 3
  retry_interval: 5
  tasks:
    cleanup:
      enabled: true
      cron_spec: "0 */5 * * * *"
      description: "清理过期数据"
      timeout: 60
      overlap_prevent: true
    
    backup:
      enabled: true
      cron_spec: "0 0 2 * * *"
      description: "每天凌晨2点备份"
      timeout: 300
      max_retries: 3
```

### 2. 加载配置

```go
package main

import (
    "github.com/kamalyes/go-config"
    "github.com/kamalyes/go-config/pkg/jobs"
    "github.com/kamalyes/go-scheduler/scheduler"
)

func main() {
    // 加载配置
    cfg := config.Load("config.yaml")
    jobsConfig := cfg.GetJobsConfig()

    // 创建调度器并加载配置
    sched := scheduler.NewCronScheduler()
    sched.LoadFromConfig(jobsConfig)

    // 注册 Job 实现
    jobMap := map[string]job.Job{
        "cleanup": NewCleanupJob(),
        "backup":  NewBackupJob(),
    }

    // 使用 JobRegistry 批量注册
    registry := scheduler.NewJobRegistry(sched, jobsConfig)
    if err := registry.RegisterJobsFromConfig(jobMap); err != nil {
        panic(err)
    }

    // 启动
    sched.Start()
    defer sched.Stop()

    select {}
}
```

## 下一步

- [核心概念](./CONCEPTS.md) - 了解 Job、Schedule、装饰器等核心概念
- [高级特性](./ADVANCED.md) - 分布式、任务依赖、分片等高级功能
- [PubSub 使用](../PUBSUB_USAGE.md) - 发布订阅系统详解
- [API 参考](./API.md) - 完整的 API 文档
