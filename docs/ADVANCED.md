# 高级特性

## 分布式调度

### 架构

```
┌─────────────┐     ┌─────────────┐     ┌─────────────┐
│ Scheduler 1 │     │ Scheduler 2 │     │ Scheduler 3 │
│  (Node-1)   │     │  (Node-2)   │     │  (Node-3)   │
└──────┬──────┘     └──────┬──────┘     └──────┬──────┘
       │                   │                   │
       └───────────────────┼───────────────────┘
                           │
                    ┌──────▼──────┐
                    │    Redis    │
                    │  Pub/Sub    │
                    │  + Lock     │
                    └─────────────┘
```

### 启用分布式模式

```go
// 1. 创建 Redis 客户端
redisClient := redis.NewClient(&redis.Options{
    Addr: "localhost:6379",
})

// 2. 创建调度器缓存
schedulerCache := repository.NewSchedulerCache(redisClient)

// 3. 创建 PubSub
cachexPS := cachex.NewPubSub(redisClient)
ps := pubsub.NewRedisPubSub(cachexPS)

// 4. 创建节点注册器
nodeRegistry := scheduler.NewNodeRegistry(
    schedulerCache,
    logger,
    "node-1",        // 节点 ID
    "192.168.1.100", // 节点主机
)

// 5. 创建调度器
sched := scheduler.NewCronScheduler(
    scheduler.WithSchedulerCache(schedulerCache),
    scheduler.WithPubSub(ps),
    scheduler.WithDistributed(nodeRegistry),
)
```

### 分布式锁

框架自动使用分布式锁确保任务在集群中只执行一次：

```go
// 框架内部自动处理
lock := lockManager.GetLock(jobName)
if err := lock.Lock(ctx); err != nil {
    // 锁被其他节点持有，跳过执行
    return nil
}
defer lock.Unlock(ctx)

// 执行任务
return job.Execute(ctx)
```

### 节点管理

#### 心跳机制

节点自动发送心跳，保持在线状态：

```go
// 默认配置
heartbeatInterval := 10 * time.Second // 心跳间隔
nodeTTL := 30 * time.Second           // 节点过期时间
```

#### 查看在线节点

```go
nodes, err := nodeRegistry.GetAllNodes(ctx)
for _, node := range nodes {
    fmt.Printf("节点: %s, 主机: %s, 状态: %s\n",
        node.NodeID, node.Host, node.Status)
}
```

## 任务依赖 (DAG)

### 定义依赖关系

```go
// 任务 A
sched.RegisterJob(jobA, jobs.TaskCfg{
    CronSpec: "0 0 1 * * *",
})

// 任务 B 依赖 A
sched.RegisterJob(jobB, jobs.TaskCfg{
    CronSpec:     "0 0 1 * * *",
    Dependencies: []string{"jobA"},
})

// 任务 C 依赖 A 和 B
sched.RegisterJob(jobC, jobs.TaskCfg{
    CronSpec:     "0 0 1 * * *",
    Dependencies: []string{"jobA", "jobB"},
})
```

### 执行顺序

```
     ┌───────┐
     │ Job A │
     └───┬───┘
         │
    ┌────┴────┐
    │         │
┌───▼───┐ ┌──▼────┐
│ Job B │ │ Job D │
└───┬───┘ └───────┘
    │
┌───▼───┐
│ Job C │
└───────┘
```

- Job A 先执行
- Job A 完成后，Job B 和 Job D 并行执行
- Job B 完成后，Job C 执行

## 任务分片

适用于大数据量处理场景。

### 实现 ShardingJob

```go
type DataSyncJob struct {
    *job.BaseJob
    db *gorm.DB
}

func (j *DataSyncJob) FetchShardingItems(
    ctx context.Context,
    sharding job.ShardingContext,
) ([]interface{}, error) {
    // 根据分片索引获取数据
    var items []DataItem
    offset := sharding.ShardIndex * 1000
    limit := 1000
    
    err := j.db.Offset(offset).Limit(limit).Find(&items).Error
    if err != nil {
        return nil, err
    }
    
    // 转换为 interface{} 切片
    result := make([]interface{}, len(items))
    for i, item := range items {
        result[i] = item
    }
    return result, nil
}

func (j *DataSyncJob) ProcessShardingItem(
    ctx context.Context,
    item interface{},
    sharding job.ShardingContext,
) error {
    dataItem := item.(DataItem)
    // 处理单个数据项
    return j.processData(dataItem)
}
```

### 配置分片

```go
sched.RegisterJob(dataSyncJob, jobs.TaskCfg{
    CronSpec:        "0 0 2 * * *",
    ShardingEnabled: true,
    ShardingTotal:   10, // 分10片
})
```

### 使用装饰器实现分片

```go
func getShardContext() *scheduler.ShardingContext {
    return &scheduler.ShardingContext{
        ShardIndex: 0,
        ShardTotal: 10,
    }
}

chain := scheduler.NewChain(
    scheduler.WithSharding(getShardContext),
)

sched := scheduler.NewCronScheduler(
    scheduler.WithChain(chain),
)
```

### 在任务中获取分片信息

```go
func (j *MyJob) Execute(ctx context.Context) error {
    // 获取分片信息
    shardIndex, ok := scheduler.GetShardIndex(ctx)
    if !ok {
        // 无分片配置
        return j.processAll()
    }
    
    shardTotal, _ := scheduler.GetShardTotal(ctx)
    
    // 根据分片处理数据
    return j.processShardData(shardIndex, shardTotal)
}
```

## 动态配置管理

### 运行时更新 CronSpec

```go
// 更新单个任务的 Cron 表达式
err := sched.UpdateCronSpec(ctx, "my-job", "0 */10 * * * *")
```

### 热重载配置

```go
// 从配置文件重新加载
newConfig := loadConfigFromFile("config.yaml")
err := sched.LoadFromConfig(newConfig)

// 分布式模式下自动同步到其他节点
```

### 配置加载策略

```go
// 优先使用本地配置
sched := scheduler.NewCronScheduler(
    scheduler.WithConfigLoadStrategy(scheduler.LoadStrategyLocalFirst),
)

// 优先使用远程配置
sched := scheduler.NewCronScheduler(
    scheduler.WithConfigLoadStrategy(scheduler.LoadStrategyRemoteFirst),
)

// 只使用本地配置
sched := scheduler.NewCronScheduler(
    scheduler.WithConfigLoadStrategy(scheduler.LoadStrategyLocalOnly),
)

// 只使用远程配置
sched := scheduler.NewCronScheduler(
    scheduler.WithConfigLoadStrategy(scheduler.LoadStrategyRemoteOnly),
)
```

## 执行快照

记录任务执行历史：

```go
// 查询执行记录
snapshots, err := snapshotRepo.List(ctx, repository.SnapshotFilter{
    JobID:     "my-job",
    Status:    models.StatusCompleted,
    StartTime: time.Now().Add(-24 * time.Hour),
    EndTime:   time.Now(),
})

// 获取统计信息
stats, err := snapshotRepo.GetStatistics(ctx, "my-job", startTime, endTime)
fmt.Printf("总执行次数: %d, 成功: %d, 失败: %d\n",
    stats.TotalCount, stats.SuccessCount, stats.FailureCount)
```

## 指标监控

### 收集指标

```go
metrics := breaker.NewMetricsCollector()
sched := scheduler.NewCronScheduler(
    scheduler.WithChain(
        scheduler.NewChain(
            scheduler.WithMetrics(metrics),
        ),
    ),
)
```

### 查看指标

```go
// 获取所有任务的指标
allMetrics := sched.GetMetrics()

// 获取单个任务的指标
jobMetrics := sched.GetJobMetrics("my-job")
fmt.Printf("执行次数: %d\n", jobMetrics["execution_count"])
fmt.Printf("成功率: %.2f%%\n", jobMetrics["success_rate"].(float64)*100)
fmt.Printf("平均耗时: %v\n", jobMetrics["avg_execution_time"])
```

## 任务优先级

```go
// 高优先级任务
sched.RegisterJob(urgentJob, jobs.TaskCfg{
    CronSpec: "0 * * * * *",
    Priority: 100,
})

// 普通优先级任务
sched.RegisterJob(normalJob, jobs.TaskCfg{
    CronSpec: "0 * * * * *",
    Priority: 50,
})

// 低优先级任务
sched.RegisterJob(backgroundJob, jobs.TaskCfg{
    CronSpec: "0 * * * * *",
    Priority: 10,
})
```

## 并发控制

### 全局并发限制

```go
sched := scheduler.NewCronScheduler(
    scheduler.WithMaxConcurrency(10), // 最多10个任务同时执行
)
```

### 任务级并发控制

```go
// 防止重叠执行
sched.RegisterJob(job, jobs.TaskCfg{
    CronSpec:       "0 * * * * *",
    OverlapPrevent: true, // 上次执行未完成，跳过本次
})
```

## 补偿机制

服务重启后自动补偿错过的任务：

```go
// 框架自动检测并补偿
// 1. 启动时加载上次执行记录
// 2. 计算错过的执行次数
// 3. 根据策略决定是否补偿
```

**补偿策略：**

- **立即补偿**：立即执行错过的任务
- **延迟补偿**：按原始调度时间补偿
- **跳过补偿**：忽略错过的任务

## 远程执行

通过 API 或消息触发远程节点执行：

```go
req := &scheduler.ExecuteRequest{
    MessageID:     uuid.New().String(),
    MessageType:   scheduler.MessageTypeExecute,
    JobName:       "my-job",
    RouteStrategy: scheduler.RouteStrategyRandom, // 随机节点
    Timeout:       60,
}

// 发布到 Redis
payload, _ := json.Marshal(req)
redis.Publish(ctx, "scheduler:execute:global", payload)
```

**路由策略：**

- `RouteStrategyFirst` - 第一个节点
- `RouteStrategyLast` - 最后一个节点
- `RouteStrategyRandom` - 随机节点
- `RouteStrategyRoundRobin` - 轮询
- `RouteStrategyBroadcast` - 广播（所有节点）
- `RouteStrategySharding` - 分片
- `RouteStrategySpecific` - 指定节点

## 下一步

- [PubSub 使用](../PUBSUB_USAGE.md) - 事件系统详解
- [API 参考](./API.md) - 查看完整 API
- [最佳实践](./BEST_PRACTICES.md) - 生产环境使用建议
