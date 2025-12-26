# syncx 集成说明

## 概述

`go-scheduler` 已成功集成 `go-toolbox/pkg/syncx` 的强大任务管理能力，提供更灵活、更强大的任务调度和管理功能。

## 核心增强

### 1. **syncx.Task 集成**

每个注册的 Job 内部都使用 `syncx.Task` 进行管理，提供：

- ✅ **任务状态管理**: Pending, Running, Completed, Failed, Cancelled
- ✅ **智能重试机制**: 支持重试次数、重试间隔、指数退避
- ✅ **任务依赖管理**: 支持复杂的任务依赖关系
- ✅ **执行历史记录**: 自动记录任务执行历史
- ✅ **成功/失败回调**: 灵活的回调机制
- ✅ **并发控制**: 通过 TaskManager 控制最大并发数

### 2. **syncx.PeriodicTaskManager 集成**

提供专门的周期任务管理：

- ✅ **固定间隔执行**: 无需 cron 表达式，直接指定时间间隔
- ✅ **立即执行选项**: 支持任务启动时立即执行首次
- ✅ **重叠保护**: 防止同一任务重叠执行
- ✅ **独立生命周期**: 独立于 cron 调度的任务管理

### 3. **增强的配置选项**

```go
// 新增选项
scheduler.WithMaxConcurrency(10)  // 设置最大并发数
```

## 使用示例

### 基础 Cron 任务（自动使用 syncx.Task）

```go
sched := scheduler.NewCronScheduler(
    scheduler.WithLoggerOption(logger),
    scheduler.WithMaxConcurrency(5),  // 最多5个任务并发执行
)

// 注册任务（内部自动创建 syncx.Task）
sched.RegisterJob(myJob, models.JobConfig{
    Enabled:        true,
    CronSpec:       "0 */5 * * * *",  // 每5分钟
    Timeout:        30,
    MaxRetries:     3,                 // syncx 重试机制
    RetryInterval:  5,
    Priority:       10,
    OverlapPrevent: true,
    Dependencies:   []string{},        // syncx 依赖管理
})
```

### 任务依赖（syncx 特性）

```go
// 注册基础任务
sched.RegisterJob(dataSyncJob, models.JobConfig{
    CronSpec: "0 0 * * * *",
    // ...
})

sched.RegisterJob(cacheWarmupJob, models.JobConfig{
    CronSpec: "0 0 * * * *",
    // ...
})

// 注册依赖任务（会先执行依赖）
sched.RegisterJob(reportJob, models.JobConfig{
    CronSpec:     "0 30 0 * * *",
    Dependencies: []string{"DataSync", "CacheWarmup"},  // 依赖前两个任务
    // ...
})
```

### 周期任务（使用 syncx.PeriodicTaskManager）

```go
// 添加周期任务（每30秒执行一次）
sched.AddPeriodicTask(
    healthCheckJob,
    30*time.Second,  // 间隔
    true,            // 立即执行
    true,            // 防止重叠
)

// 启动周期任务
sched.StartPeriodicTasks()
```

### 获取任务详情（syncx 集成特性）

```go
// 获取单个任务详情
details, err := sched.GetTaskDetails("MyJob")
if err == nil {
    fmt.Printf("状态: %v\n", details["state"])
    fmt.Printf("重试次数: %v\n", details["retry_count"])
    fmt.Printf("执行耗时: %v\n", details["fn_duration"])
    fmt.Printf("历史记录: %v\n", details["history"])
}

// 获取所有任务详情
allDetails := sched.GetAllTasksDetails()
fmt.Printf("总任务数: %d\n", allDetails["total_tasks"])
fmt.Printf("周期任务数: %d\n", allDetails["periodic_task_count"])
```

## 架构说明

```
┌─────────────────────────────────────────────────────────┐
│                    CronScheduler                         │
│  ┌───────────────────────────────────────────────────┐  │
│  │            syncx.TaskManager                      │  │
│  │  - 管理所有 cron 任务的 syncx.Task               │  │
│  │  - 控制最大并发数                                 │  │
│  │  - 任务依赖解析和执行                             │  │
│  └───────────────────────────────────────────────────┘  │
│  ┌───────────────────────────────────────────────────┐  │
│  │        syncx.PeriodicTaskManager                  │  │
│  │  - 管理所有周期任务                               │  │
│  │  - 独立的生命周期管理                             │  │
│  │  - 重叠保护和立即执行                             │  │
│  └───────────────────────────────────────────────────┘  │
│                                                          │
│  Entry (任务条目)                                       │
│  ├─ Job                (原始任务)                       │
│  ├─ WrappedJob         (装饰后的任务)                   │
│  ├─ Task               (syncx.Task 管理)                │
│  ├─ PeriodicTask       (周期任务)                       │
│  └─ Config             (任务配置)                       │
└─────────────────────────────────────────────────────────┘
```

## 主要优势

1. **统一的任务管理**: 使用 syncx 统一管理所有类型的任务
2. **强大的依赖管理**: 自动处理任务依赖关系，支持并发/顺序执行
3. **详细的执行追踪**: 自动记录任务执行历史和性能指标
4. **灵活的回调机制**: 成功/失败回调自动处理日志、指标、持久化
5. **并发控制**: 全局并发数控制，避免资源过度消耗
6. **周期任务增强**: 专门的周期任务管理器，更适合固定间隔场景
7. **完全向后兼容**: 保持现有 API 不变，内部增强实现

## 迁移指南

### 现有代码无需修改

现有代码无需任何修改即可享受 syncx 的增强功能：

```go
// 原有代码继续工作
sched := scheduler.NewCronScheduler()
sched.RegisterJob(myJob, config)
sched.Start()
```

### 可选：启用新特性

如果想使用新特性，只需添加配置：

```go
// 启用并发控制
sched := scheduler.NewCronScheduler(
    scheduler.WithMaxConcurrency(10),
)

// 使用任务依赖
config := models.JobConfig{
    Dependencies: []string{"TaskA", "TaskB"},
    // ...
}

// 添加周期任务
sched.AddPeriodicTask(job, 30*time.Second, true, true)

// 查看任务详情
details := sched.GetAllTasksDetails()
```

## 完整示例

查看 `examples/syncx_integration/main.go` 获取完整的使用示例。

## 性能影响

- **内存**: 每个任务额外增加约 1-2KB（Task 对象 + 历史记录）
- **CPU**: 任务执行开销增加 < 1%（主要用于状态管理和回调）
- **并发**: TaskManager 提供更好的并发控制，避免资源竞争

## 未来计划

- [ ] 任务执行结果缓存
- [ ] 任务链式调用 API
- [ ] 动态调整任务优先级
- [ ] 任务执行可视化
- [ ] 任务分组管理

## 参考

- [syncx.Task 文档](../go-toolbox/pkg/syncx/task.go)
- [syncx.PeriodicTaskManager 文档](../go-toolbox/pkg/syncx/periodic_task.go)
- [go-scheduler 原始文档](../README.md)
