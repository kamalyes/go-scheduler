# PubSub 使用指南

- 独立的 `pubsub.PubSub` 接口
- 调度器通过配置注入 PubSub 客户端
- 支持本地（LocalPubSub）和分布式（RedisPubSub）两种实现

## 基础使用

### 1. 单机模式 - 使用 LocalPubSub

```go
package main

import (
    "context"
    "github.com/kamalyes/go-scheduler/scheduler"
    "github.com/kamalyes/go-scheduler/pubsub"
)

func main() {
    // 创建本地 PubSub（基于 channel）
    ps := pubsub.NewLocalPubSub()
    defer ps.Close()

    // 创建调度器
    sched := scheduler.NewCronScheduler(
        scheduler.WithPubSub(ps),
    )

    // 启动调度器
    sched.Start()
    defer sched.Stop()
}
```

### 2. 分布式模式 - 使用 RedisPubSub

```go
package main

import (
    "context"
    "github.com/kamalyes/go-scheduler/scheduler"
    "github.com/kamalyes/go-scheduler/pubsub"
    "github.com/kamalyes/go-cachex"
    "github.com/redis/go-redis/v9"
)

func main() {
    // 创建 Redis 客户端
    redisClient := redis.NewClient(&redis.Options{
        Addr: "localhost:6379",
    })

    // 创建 go-cachex PubSub
    cachexPS := cachex.NewPubSub(redisClient)

    // 创建 RedisPubSub
    ps := pubsub.NewRedisPubSub(cachexPS)
    defer ps.Close()

    // 创建调度器（分布式模式）
    sched := scheduler.NewCronScheduler(
        scheduler.WithPubSub(ps),
        scheduler.WithDistributed(nodeRegistry),
    )

    // 启动调度器
    sched.Start()
    defer sched.Stop()
}
```

## 支持的事件类型

### 1. 任务配置更新事件

**发布：**
```go
err := ps.PublishConfigUpdate(ctx, "my-job")
```

**订阅：**
```go
ps.SubscribeConfigUpdate(ctx, func(jobName string) {
    fmt.Printf("任务 %s 的配置已更新\n", jobName)
})
```

### 2. CronSpec 更新事件

**发布：**
```go
err := ps.PublishCronSpecUpdate(ctx, "my-job", "0 */5 * * * *")
```

**订阅：**
```go
ps.SubscribeCronSpecUpdate(ctx, func(jobName, cronSpec string) {
    fmt.Printf("任务 %s 的 Cron 表达式已更新为: %s\n", jobName, cronSpec)
})
```

### 3. 手动执行事件

**发布：**
```go
err := ps.PublishManualExecute(ctx, "my-job")
```

**订阅：**
```go
ps.SubscribeManualExecute(ctx, func(jobName string) {
    fmt.Printf("手动执行任务: %s\n", jobName)
})
```

### 4. 定时执行事件

**发布：**
```go
executeTime := time.Now().Add(5 * time.Minute)
err := ps.PublishScheduledExecute(ctx, "my-job", executeTime, 3)
```

**订阅：**
```go
ps.SubscribeScheduledExecute(ctx, func(jobName string, executeTime time.Time, repeatCount int) {
    fmt.Printf("定时执行任务: %s, 时间=%v, 次数=%d\n", jobName, executeTime, repeatCount)
})
```

### 5. 任务删除事件

**发布：**
```go
err := ps.PublishJobDeleted(ctx, "my-job")
```

**订阅：**
```go
ps.SubscribeJobDeleted(ctx, func(jobName string) {
    fmt.Printf("任务 %s 已被删除\n", jobName)
})
```

## 分布式场景

在分布式部署中，多个调度器实例通过 RedisPubSub 同步状态：

```
┌──────────────┐         ┌──────────────┐         ┌──────────────┐
│  Scheduler 1 │◄────────┤  Redis Pub/Sub ├────────►│  Scheduler 2 │
└──────────────┘         └──────────────┘         └──────────────┘
       │                                                   │
       │                                                   │
       ▼                                                   ▼
  本地任务注册                                        本地任务注册
```

### 事件流程示例

1. **Scheduler 1** 更新任务配置：
```go
sched1.UpdateCronSpec(ctx, "my-job", "0 */10 * * * *")
```

2. **自动发布事件**到 Redis：
```
Event: job:cronspec:update
Payload: {"job_name": "my-job", "cron_spec": "0 */10 * * * *"}
```

3. **Scheduler 2** 自动接收并处理：
```go
// 内部自动调用
reloadJobConfig("my-job")
```

## 自定义 PubSub 实现

你可以实现自己的 PubSub 后端：

```go
package custom

import (
    "context"
    "time"
    "github.com/kamalyes/go-scheduler/pubsub"
)

type MyPubSub struct {
    // 你的实现
}

func (p *MyPubSub) PublishConfigUpdate(ctx context.Context, jobName string) error {
    // 你的逻辑
    return nil
}

func (p *MyPubSub) SubscribeConfigUpdate(ctx context.Context, handler func(jobName string)) error {
    // 你的逻辑
    return nil
}

// ... 实现其他接口方法
```

然后注入到调度器：

```go
myPS := custom.NewMyPubSub()
sched := scheduler.NewCronScheduler(
    scheduler.WithPubSub(myPS),
)
```

## 测试

### Mock PubSub

```go
type MockPubSub struct {
    publishedEvents []string
}

func (m *MockPubSub) PublishConfigUpdate(ctx context.Context, jobName string) error {
    m.publishedEvents = append(m.publishedEvents, "config:"+jobName)
    return nil
}

// ... 实现其他方法

// 测试
func TestSchedulerWithMockPubSub(t *testing.T) {
    mockPS := &MockPubSub{}
    sched := scheduler.NewCronScheduler(
        scheduler.WithPubSub(mockPS),
    )
    
    // 执行操作
    sched.UpdateCronSpec(ctx, "test-job", "0 * * * * *")
    
    // 验证事件
    assert.Contains(t, mockPS.publishedEvents, "config:test-job")
}
```

## 最佳实践

### 1. 错误处理

PubSub 操作应该是非阻塞的，即使失败也不应中断主流程：

```go
if err := ps.PublishConfigUpdate(ctx, jobName); err != nil {
    logger.Warnf("发布事件失败: %v", err) // 记录警告，不返回错误
}
```

### 2. 优雅关闭

确保在程序退出时关闭 PubSub：

```go
defer ps.Close()
```

### 3. 订阅重连

RedisPubSub 会自动处理连接断开和重连，但建议添加监控：

```go
go func() {
    ticker := time.NewTicker(30 * time.Second)
    defer ticker.Stop()
    
    for range ticker.C {
        // 检查 PubSub 连接状态
    }
}()
```

### 4. 事件去重

在分布式环境中，可能会收到重复事件，建议添加去重逻辑：

```go
ps.SubscribeConfigUpdate(ctx, func(jobName string) {
    if isDuplicate(jobName) {
        return
    }
    processUpdate(jobName)
})
```

## 性能优化

### 1. 批量发布

```go
jobs := []string{"job1", "job2", "job3"}
for _, jobName := range jobs {
    ps.PublishConfigUpdate(ctx, jobName)
}
```

### 2. 异步处理

订阅处理器默认是异步的，避免阻塞：

```go
ps.SubscribeConfigUpdate(ctx, func(jobName string) {
    go handleUpdate(jobName) // 在 goroutine 中处理
})
```

## 故障排查

### 问题：事件未收到

**检查清单：**
1. 确认 PubSub 客户端已正确配置
2. 验证 Redis 连接正常（RedisPubSub）
3. 检查订阅是否在发布之前
4. 查看日志是否有错误信息

### 问题：重复事件

**解决方案：**
- 添加事件去重逻辑
- 使用事务 ID 跟踪事件
- 检查是否有多个订阅者

## 总结

重构后的 PubSub 模块提供了：

✅ **解耦**：调度器不再直接依赖 SchedulerCache 的 PubSub  
✅ **灵活**：支持多种 PubSub 实现  
✅ **可测试**：易于 Mock 和单元测试  
✅ **扩展性**：方便添加新的事件类型  
✅ **向后兼容**：保持 API 一致性

