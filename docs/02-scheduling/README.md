# 模块 2：调度与编排（Scheduling & Orchestration）

## 一、核心问题

**给定一组待运行的任务和一组可用节点，如何决定每个任务在哪运行？**

这是 Cloud Runtime 控制面的核心职责。调度器是连接"用户意图"和"物理资源"的桥梁。

### 真实场景

你有 3 台机器：
- Node-A：4 CPU, 8GB 内存
- Node-B：2 CPU, 4GB 内存
- Node-C：4 CPU, 16GB 内存

用户提交了 5 个任务：
- Task-1：需要 1 CPU, 2GB
- Task-2：需要 2 CPU, 4GB
- Task-3：需要 0.5 CPU, 1GB
- Task-4：需要 1 CPU, 8GB
- Task-5：需要 2 CPU, 2GB

怎么分配？这不是简单的"找个够大的节点放上去"——每个选择都会影响后续任务的可行性和系统的整体效率。

---

## 二、调度的本质：一个约束满足问题

调度本质上是一个 **约束满足问题（Constraint Satisfaction Problem）**：

```
给定：
  - 一组任务 T = {t1, t2, ..., tn}，每个任务有资源需求
  - 一组节点 N = {n1, n2, ..., nm}，每个节点有资源容量
  - 一组约束 C = {c1, c2, ...}（亲和性、反亲和性、优先级等）

求：
  - 一个映射 f: T → N ∪ {unschedulable}
  - 使得所有约束满足，且某个目标函数最优
```

### 两类约束

| 约束类型 | 例子 | 说明 |
|---------|------|------|
| **硬约束** | 资源容量、节点选择器 | 必须满足，否则任务无法调度 |
| **软约束** | 亲和性偏好、负载均衡 | 尽量满足，不满足也能调度 |

### 目标函数

不同的调度策略优化不同的目标：

- **Bin-packing**：最小化使用节点数 → 省电、省资源
- **Spread**：最大化任务分散度 → 高可用、容错
- **公平性**：每个用户/团队获得公平的资源份额
- **优先级**：高优先级任务优先获得资源

---

## 三、调度策略详解

### 1. Bin-packing（紧凑放置）

**思想**：像装箱一样，尽量把任务塞进已经使用的节点，填满一个再开下一个。

```
Node-A: [Task-1, Task-3, Task-5]  ← 填满
Node-B: [Task-2]                  ← 正在使用
Node-C: []                        ← 空闲，不开
```

**优点**：
- 空闲节点可以关机省电
- 资源碎片化程度低

**缺点**：
- 节点故障影响面大（一个节点上任务太多）
- 不利于高可用

**适用场景**：成本敏感的批处理任务、开发环境

### 2. Spread（分散放置）

**思想**：尽量把任务分散到不同节点，避免单点故障。

```
Node-A: [Task-1]
Node-B: [Task-2]
Node-C: [Task-3]
```

**优点**：
- 单节点故障影响最小
- 负载天然均衡

**缺点**：
- 所有节点都需要保持运行
- 可能产生资源碎片

**适用场景**：生产环境、高可用服务

### 3. 亲和性/反亲和性（Affinity/Anti-Affinity）

**思想**：任务可以声明"我想和谁在一起"或"我不想和谁在一起"。

- **亲和性**：数据库和缓存放同一节点（减少网络延迟）
- **反亲和性**：同一服务的多个副本不放同一节点（避免同时故障）

### 4. 优先级与抢占（Priority & Preemption）

**思想**：当资源不足时，高优先级任务可以"抢占"低优先级任务的资源。

```
高优先级任务提交 → 资源不足 → 驱逐低优先级任务 → 高优先级任务运行
```

Kubernetes 的实现：
- PriorityClass 定义优先级（0-1000000000）
- 抢占时考虑"谁被驱逐后总损失最小"
- 优雅终止期（grace period）让被驱逐任务有机会保存状态

---

## 四、调度器的架构模式

### 模式 1：集中式调度器（Monolithic Scheduler）

```
           ┌─────────────┐
           │  Scheduler  │ ← 所有调度决策在一个进程
           └─────────────┘
            /     |     \
       Node-A  Node-B  Node-C
```

**代表**：Kubernetes（默认调度器）、Nomad

**优点**：简单，全局视野，决策一致
**缺点**：单点瓶颈，扩展性差

### 模式 2：两层调度器（Two-Level Scheduler）

```
           ┌─────────────┐
           │  Meta       │ ← 第一层：粗粒度分配
           │  Scheduler  │
           └─────────────┘
            /     |     \
     ┌──────┐ ┌──────┐ ┌──────┐
     │ Node │ │ Node │ │ Node │ ← 第二层：节点内细粒度调度
     │ Sched│ │ Sched│ │ Sched│
     └──────┘ └──────┘ └──────┘
```

**代表**：Mesos

**优点**：可扩展，每个节点自治
**缺点**：全局视野受限，可能资源分配不均

### 模式 3：共享状态调度器（Shared State Scheduler）

```
     ┌──────────┐  ┌──────────┐  ┌──────────┐
     │ Sched-1  │  │ Sched-2  │  │ Sched-3  │ ← 多个调度器并行
     └──────────┘  └──────────┘  └──────────┘
          │             │             │
          └─────────────┴─────────────┘
                    │
           ┌───────────────┐
           │ Shared State  │ ← 乐观并发控制
           │ (etcd)        │
           └───────────────┘
```

**代表**：Omega（Google 论文）

**优点**：并行调度，高吞吐
**缺点**：冲突处理复杂

### 我们的选择

**集中式调度器**——最适合教学，也是 Kubernetes 的核心模式。我们实现：
- 一个 Scheduler 进程
- 事件驱动的调度循环
- 支持 bin-packing 和 spread 两种策略
- 资源请求/限制模型

---

## 五、资源模型

### Request vs Limit

这是 Cloud Runtime 资源模型最重要的区分：

| | Request | Limit |
|---|---------|-------|
| 含义 | 任务**保证**能获得的资源 | 任务**最多**能使用的资源 |
| 调度用途 | 用于调度决策 | 用于运行时限制（cgroups） |
| 类比 | 餐厅预订的座位数 | 餐厅最多能坐的人数 |

```
Task: { request: 0.5 CPU, limit: 2 CPU }
含义：调度时保证分配 0.5 CPU，运行时最多能用 2 CPU（可突发）
```

**为什么需要两个值？**

1. **Request 用于调度**：调度器根据 request 计算节点是否还有足够资源
2. **Limit 用于隔离**：cgroups 根据 limit 限制实际使用，防止一个任务吃光资源
3. **突发能力**：request < limit 允许任务在节点空闲时临时使用更多资源

### 资源碎片问题

```
Node: 4 CPU total, 已分配 3.5 CPU (request)
新任务 request: 1 CPU → 无法调度（只剩 0.5 CPU 可用）
但实际使用可能只有 2 CPU（limit 可以更高）
→ 浪费了 2 CPU 的实际算力
```

这是调度器的经典难题。工业界的解决方案：
- **节点重平衡**：定期将任务从碎片化节点迁移
- **调度插件**：Kubernetes 的 NodeResourcesFit 插件支持 MostAllocated（bin-pack）和 LeastAllocated（spread）策略

---

## 六、事件驱动的调度循环

调度器不是"每次有新任务就全量重新调度"，而是**事件驱动**的增量处理：

```go
for {
    select {
    case event := <-eventCh:
        switch event.Type {
        case TaskAdded:
            // 新任务提交 → 尝试调度
            scheduleTask(event.Task)
        case TaskDeleted:
            // 任务完成 → 释放资源
            releaseResources(event.Task)
        case NodeAdded:
            // 新节点加入 → 可能可以调度之前 pending 的任务
            trySchedulePending()
        case NodeDeleted:
            // 节点移除 → 重新调度上面的任务
            rescheduleTasks(event.Node)
        }
    }
}
```

### 为什么是事件驱动而不是轮询？

- **效率**：只在状态变化时处理，不浪费 CPU
- **及时性**：新任务提交后立即调度，不需要等下一个轮询周期
- **可组合**：多个事件源可以汇聚到同一个 channel

---

## 七、我们的实现设计

### 架构

```
┌──────────────────────────────────────┐
│            Scheduler                  │
│  ┌────────────────────────────────┐  │
│  │      Event Loop (channel)      │  │
│  └────────────────────────────────┘  │
│  ┌────────────┐  ┌────────────────┐  │
│  │  Filter    │→ │    Score       │  │
│  │ (硬约束过滤) │  │ (软约束打分)   │  │
│  └────────────┘  └────────────────┘  │
│  ┌────────────────────────────────┐  │
│  │      Binding (绑定决策)        │  │
│  └────────────────────────────────┘  │
│  ┌────────────┐  ┌────────────────┐  │
│  │ NodeStore  │  │  TaskStore     │  │
│  └────────────┘  └────────────────┘  │
└──────────────────────────────────────┘
```

### 调度流程

1. **Filter**：过滤掉不满足硬约束的节点（资源不足、标签不匹配）
2. **Score**：对剩余节点按策略打分（bin-packing 或 spread）
3. **Bind**：选择得分最高的节点，将任务绑定到该节点

### 关键数据结构

```go
// Task 待调度的任务
type Task struct {
    Spec   TaskSpec
    Status TaskStatus
}

// TaskSpec 包含资源需求和调度约束
type TaskSpec struct {
    Name         string
    CPURequest   int        // millicores
    MemRequest   int64      // bytes
    NodeSelector map[string]string
    Policy       SchedulingPolicy  // binpack or spread
}

// TaskStatus 调度状态
type TaskStatus struct {
    NodeName string  // 被调度到哪个节点
    Phase    string  // pending, scheduled, running, failed
}
```

---

## 八、工业对比

| 调度器 | 架构 | 默认策略 | 特色 |
|--------|------|---------|------|
| **Kubernetes Scheduler** | 集中式 | spread | 插件化框架，支持扩展 |
| **Nomad** | 集中式 | binpack | 简单轻量，支持非容器任务 |
| **Mesos** | 两层 | 取决于 Framework | DRF 公平调度算法 |
| **Borg** | 集中式 | mixed | Google 内部，混合策略 |

### Kubernetes 调度器演进

1. **v1.0-v1.14**：单体调度器，硬编码调度逻辑
2. **v1.15+**：调度框架（Scheduling Framework），插件化扩展点
3. **v1.19+**：Volcano 等批处理调度器，支持 gang scheduling

Kubernetes 调度器的 Filter→Score→Bind 三阶段就是我们要实现的核心流程。

---

**下一步**：阅读代码实现，理解 Filter/Score/Bind 三阶段调度流程。