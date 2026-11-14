# 模块 5：函数运行时（FaaS Runtime）

## 一、核心问题

**如何实现毫秒级的函数冷启动，让用户代码"随叫随到"？**

传统容器启动需要秒级时间（拉镜像、创建 namespace、初始化运行时）。但 FaaS 的承诺是：用户提交一段代码，系统能在毫秒级内响应事件并执行。这之间的差距就是冷启动问题。

### 真实场景

用户提交了一个 Python 函数，处理 HTTP 请求：
1. 首次请求到达 → 需要启动函数实例 → 冷启动延迟（可能 100ms~5s）
2. 后续请求到达 → 复用已有实例 → 热启动延迟（~1ms）
3. 一段时间无请求 → 实例被回收 → 下次请求又需要冷启动

FaaS 引擎的核心挑战就是：**如何最小化冷启动频率和延迟**。

---

## 二、函数生命周期

### 完整生命周期

```
                    ┌─────────┐
                    │ Created │ ← 函数定义已注册
                    └────┬────┘
                         │ 首次事件触发
                         ▼
                    ┌─────────┐
                    │ Pending │ ← 等待实例启动
                    └────┬────┘
                         │ 实例就绪
                         ▼
    ┌──────────────┐ ┌─────────┐
    │   Frozen     │←│ Running │ ← 正在处理请求
    │ (空闲冻结)   │→└─────────┘
    └──────────────┘      │
         │  ↑             │ 长时间无请求
         │  │             ▼
         │  │        ┌─────────┐
         │  └────────│ Idle    │ ← 等待新请求
         │           └────┬────┘
         │                │ 超时
         ▼                ▼
    ┌─────────┐      ┌─────────┐
    │ Stopped │      │ Deleted │ ← 实例被回收
    └─────────┘      └─────────┘
```

### 冷启动 vs 热启动

| | 冷启动 | 热启动 |
|---|--------|--------|
| **触发条件** | 首次请求 / 实例被回收后 | 已有实例在运行 |
| **延迟** | 100ms ~ 5s | ~1ms |
| **原因** | 需要创建沙箱、加载代码、初始化运行时 | 直接复用已有进程 |
| **用户体验** | 明显延迟 | 几乎无感 |

### 冷启动的组成部分

```
冷启动总延迟 = 沙箱创建 + 代码加载 + 运行时初始化 + 函数初始化
     ↑              ↑            ↑              ↑
  ~50ms         ~100ms       ~200ms         ~50ms
  (namespace    (从镜像层     (Python/Node   (用户代码的
   +cgroups)    加载文件)     启动时间)      init 逻辑)
```

---

## 三、冷启动优化策略

### 1. 预热池（Warm Pool）

**核心思想**：预先启动一批沙箱实例，保持"待命"状态。请求到达时直接分配，跳过沙箱创建步骤。

```
预热池: [instance-1(ready)] [instance-2(ready)] [instance-3(ready)]
                                          │
                              请求到达 → 分配 instance-1
                                          │
预热池: [instance-2(ready)] [instance-3(ready)] [instance-4(creating)]
```

**AWS Lambda 的实现**：Firecracker microVM 预热池，每个 microVM 启动只需 125ms。

### 2. 快照恢复（Snapshot Restore）

**核心思想**：将已初始化的函数实例内存状态保存为快照。冷启动时直接恢复快照，跳过初始化。

```
首次启动: 创建沙箱 → 加载代码 → 初始化运行时 → 执行 init → [保存快照]
后续冷启动: 创建沙箱 → [恢复快照] → 直接就绪
```

**CRIU（Checkpoint/Restore In Userspace）** 是 Linux 的进程快照工具，可以将运行中的进程冻结为磁盘文件，然后从文件恢复。

### 3. 微 VM（MicroVM）

**核心思想**：用极轻量的 VM 替代传统容器，兼顾隔离性和启动速度。

| | 传统 VM | 容器 | MicroVM |
|---|---------|------|---------|
| 隔离性 | 强 | 弱 | 强 |
| 启动时间 | 10-30s | 1-2s | 125ms |
| 内存开销 | 500MB+ | ~0 | 5MB |
| 内核 | 独立 | 共享 | 独立(精简) |

**Firecracker** 是 AWS 开源的 MicroVM 方案，Rust 实现，专为 FaaS 设计。

### 4. 渐进式初始化

**核心思想**：不等待所有初始化完成就接受请求，按需加载依赖。

```
传统: init() → load_all_deps() → connect_db() → ready()
渐进: ready() → (请求到达时才) load_dep() → connect_db()
```

### 我们的实现

**预热池策略**——最直观、最容易理解、也最贴近工业实践。我们实现：
- 可配置的预热池大小
- 请求到达时从预热池分配实例
- 空闲实例超时回收
- 池自动补充

---

## 四、事件驱动模型

### 事件源

FaaS 的核心特征是**事件驱动**——函数不是常驻服务，而是由事件触发：

| 事件源 | 触发方式 | 示例 |
|--------|---------|------|
| HTTP | HTTP 请求到达 | API Gateway → Function |
| Timer | 定时触发 | 每分钟执行一次清理 |
| Queue | 消息队列消息 | Kafka → Function |
| Storage | 文件变化 | S3 上传 → Function |
| Custom | 自定义事件 | 内部系统调用 |

### 事件 → 函数 映射

```
事件源 → 路由规则 → 函数 → 实例
HTTP /api/users → user-handler → instance-1
Timer 5min → cleanup → instance-2
Queue orders → order-processor → instance-3
```

### 我们的实现

支持 HTTP 和 Timer 两种事件源：
- **HTTP**：监听端口，请求到达时路由到对应函数
- **Timer**：定时触发函数执行

---

## 五、并发模型

### 单实例并发 vs 多实例扩展

| 策略 | 描述 | 适用场景 |
|------|------|---------|
| **单实例串行** | 一个实例一次只处理一个请求 | 状态敏感的函数 |
| **单实例并发** | 一个实例同时处理多个请求 | I/O 密集型函数 |
| **多实例扩展** | 请求量大时启动多个实例 | CPU 密集型函数 |

### Kubernetes 的做法（Knative）

Knative 使用 **自动扩缩容（Autoscaling）**：
- 默认基于并发请求数扩缩
- 支持 min/max 实例数配置
- 支持 scale-to-zero（无请求时实例数为 0）

```yaml
spec:
  template:
    spec:
      containerConcurrency: 10  # 每实例最大并发 10
  autoscaling:
    minScale: 0  # 最小实例数（0 = scale-to-zero）
    maxScale: 100  # 最大实例数
    target: 50  # 目标并发数
```

### 我们的实现

- 每个函数实例一次处理一个请求（单实例串行）
- 请求排队等待，超过队列长度则启动新实例
- 支持 min/max 实例数配置

---

## 六、我们的实现设计

### 架构

```
┌──────────────────────────────────────────────┐
│              FaaSEngine                      │
│  ┌────────────┐  ┌────────────────────────┐  │
│  │ FunctionRegistry │  │ WarmPool           │  │
│  │ (函数注册表)    │  │ (预热池管理)        │  │
│  └────────────┘  └────────────────────────┘  │
│  ┌────────────┐  ┌────────────────────────┐  │
│  │ EventRouter│  │ InstanceManager        │  │
│  │ (事件路由) │  │ (实例生命周期管理)     │  │
│  └────────────┘  └────────────────────────┘  │
│  ┌────────────────────────────────────────┐  │
│  │ Autoscaler (自动扩缩容)                │  │
│  └────────────────────────────────────────┘  │
└──────────────────────────────────────────────┘
```

### 数据模型

```go
// Function 函数定义
type Function struct {
    Name        string
    Runtime     string   // python, node, go
    Handler     string   // 入口函数名
    Code        string   // 函数代码路径
    Timeout     int      // 执行超时（秒）
    MemoryLimit int64    // 内存限制
    MinInstances int     // 最小实例数
    MaxInstances int     // 最大实例数
}

// Instance 函数实例
type Instance struct {
    ID          string
    FunctionName string
    Status      string   // creating, ready, busy, frozen, stopped
    CreatedAt   time.Time
    LastUsedAt  time.Time
    RequestCount int
}

// Event 触发事件
type Event struct {
    Source      string   // http, timer, queue
    FunctionName string
    Payload     []byte
    Headers     map[string]string
}

// InvokeResult 函数执行结果
type InvokeResult struct {
    Output     []byte
    Error      string
    Duration   time.Duration
    ColdStart  bool   // 是否经历了冷启动
}
```

### 关键设计决策

1. **模拟执行**：教学版不真正执行用户代码，而是模拟函数执行的延迟和生命周期
2. **预热池**：可配置大小，自动补充，空闲超时回收
3. **事件路由**：HTTP 事件和 Timer 事件
4. **自动扩缩容**：基于队列长度和并发请求数
5. **冷启动标记**：每次调用返回是否经历了冷启动，便于观察优化效果

---

## 七、工业对比

| 平台 | 冷启动优化 | 隔离方案 | 语言支持 | 特点 |
|------|-----------|---------|---------|------|
| **AWS Lambda** | Firecracker + 预热池 | MicroVM | 多语言 | 最成熟 |
| **Google Cloud Functions** | gVisor + 预热 | 用户态内核 | 多语言 | 安全性强 |
| **Azure Functions** | 预热池 | 容器 | 多语言 | .NET 生态 |
| **OpenFaaS** | 容器 | 容器 | 多语言 | 开源、K8s 原生 |
| **Knative** | 容器 + 自动扩缩 | 容器 | 多语言 | K8s 标准 |
| **我们的实现** | 预热池 | 模拟 | 模拟 | 教学 |

---

**下一步**：阅读代码实现，理解预热池、事件路由、自动扩缩容的具体实现。