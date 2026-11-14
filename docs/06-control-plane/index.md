# 模块 6：控制面与 API（Control Plane & API）

## 一、核心问题

**如何用一套统一的接口管理前五个模块的所有资源？**

前五个模块各自解决了隔离、调度、网络、存储、函数运行时的问题。但它们目前是分散的——用户需要分别调用不同的命令来管理不同类型的资源。

控制面的职责就是：**提供一个统一的声明式 API，让用户用同一套接口管理所有资源，并通过控制器循环自动驱动系统向期望状态收敛。**

这是整个 Cloud Runtime 的"大脑"——它不直接执行任何操作，而是观察状态、做出决策、驱动数据面执行。

---

## 二、声明式 API

### 命令式 vs 声明式

**命令式 API**：
```
POST /sandboxes/create {name: "web", cpu: 500}
POST /sandboxes/web/start
POST /sandboxes/web/network/connect
POST /sandboxes/web/storage/bind {volume: "data"}
```
用户需要告诉系统每一步**怎么做**。

**声明式 API**：
```
PUT /resources/web {
  kind: "Sandbox",
  spec: {
    cpu: 500,
    network: {connect: true},
    storage: {volumes: ["data"]}
  }
}
```
用户只声明**期望状态**，系统自己决定怎么做。

### 为什么声明式更好？

1. **幂等性**：同样的声明执行多次，结果一样。网络中断后重试不会产生副作用。
2. **自愈**：系统持续检查实际状态是否匹配期望状态。如果有人手动删了沙箱，控制器会自动重建。
3. **可审计**：所有期望状态都有版本记录（resourceVersion），可以回滚到任意历史版本。
4. **可组合**：多个资源的声明可以互相引用（标签选择器），系统自动解析依赖关系。

### RESTful 资源模型

所有资源遵循统一的 CRUD + Watch 接口：

```
GET    /api/v1/<resource>           — 列出所有资源
GET    /api/v1/<resource>/<name>     — 查询单个资源
POST   /api/v1/<resource>            — 创建资源
PUT    /api/v1/<resource>/<name>     — 更新资源（全量替换）
PATCH  /api/v1/<resource>/<name>     — 部分更新
DELETE /api/v1/<resource>/<name>     — 删除资源
GET    /api/v1/<resource>?watch=true — Watch（长连接事件推送）
```

---

## 三、控制器循环（Reconcile Loop）

### 核心机制

控制器循环是控制面的心脏。它的逻辑极其简单但极其强大：

```go
for {
    desired := getDesiredState()    // 从 API Server 读期望状态
    actual := getActualState()      // 从数据面读实际状态
    if desired != actual {
        actions := computeActions(desired, actual)
        execute(actions)            // 驱动数据面执行
    }
    sleep(shortInterval)
}
```

### 为什么这个简单循环如此强大？

1. **不需要复杂的状态机**：控制器不需要知道"当前在什么状态、下一步该做什么"。它只需要比较期望和实际，然后行动。
2. **自动处理任何偏差**：无论是用户修改了声明、还是数据面出了故障、还是手动操作改变了实际状态——控制器都会发现偏差并修正。
3. **天然分布式友好**：多个控制器可以并行运行，每个负责不同资源类型，互不干扰。

### Kubernetes 的控制器架构

```
┌──────────────────────────────────────────┐
│          Controller Manager              │
│  ┌────────────────┐  ┌────────────────┐  │
│  │ SandboxCtrl    │  │ NetworkCtrl    │  │
│  │ (沙箱控制器)   │  │ (网络控制器)   │  │
│  └────────────────┘  └────────────────┘  │
│  ┌────────────────┐  ┌────────────────┐  │
│  │ StorageCtrl    │  │ FaaSCtrl       │  │
│  │ (存储控制器)   │  │ (函数控制器)   │  │
│  └────────────────┘  └────────────────┘  │
│  ┌────────────────┐                      │
│  │ SchedulerCtrl  │                      │
│  │ (调度控制器)   │                      │
│  └────────────────┘                      │
└──────────────────────────────────────────┘
         │
         ▼
┌──────────────────┐
│   API Server     │ ← 所有控制器通过 API Server 通信
│   (etcd 存储)    │
└──────────────────┘
```

每个控制器：
- Watch 自己负责的资源类型的变化
- 将期望状态与实际状态比较
- 驱动数据面执行操作

---

## 四、Watch 机制

### 为什么需要 Watch？

轮询（Polling）的问题：
- 大量无效请求（大多数轮询时状态没变化）
- 延迟（轮询间隔内的变化无法及时感知）
- 浪费带宽和 CPU

Watch 的优势：
- 事件驱动，状态变化时立即推送
- 长连接，减少握手开销
- 增量更新，只推送变化的部分

### Watch 的实现

```
客户端                          API Server
  │                                │
  │── GET /api/v1/sandboxes?watch=true ──→│
  │                                │
  │←── HTTP 200 (chunked) ─────────│
  │←── Event: ADDED sandbox-1 ─────│
  │←── Event: MODIFIED sandbox-1 ──│
  │←── Event: DELETED sandbox-2 ───│
  │                                │
  │ (长连接保持)                    │
  │                                │
```

Watch 使用 HTTP chunked transfer encoding 保持长连接，每次资源变化时推送一个事件。

### 事件类型

| 事件 | 含义 | 触发条件 |
|------|------|---------|
| **ADDED** | 资源被创建 | 用户 POST 新资源 |
| **MODIFIED** | 资源被修改 | 用户 PUT/PATCH 资源 |
| **DELETED** | 资源被删除 | 用户 DELETE 资源 |

---

## 五、Spec / Status 模型

### 双字段模型

每个资源都有两个核心字段：

```json
{
  "kind": "Sandbox",
  "metadata": {
    "name": "web-server",
    "resourceVersion": "42",
    "labels": {"app": "web"}
  },
  "spec": {
    "cpu": 500,
    "command": ["/bin/web-server"]
  },
  "status": {
    "phase": "running",
    "pid": 12345,
    "ip": "10.0.0.2"
  }
}
```

- **Spec**：用户声明的期望状态（用户写入，控制器读取）
- **Status**：系统观察到的实际状态（控制器写入，用户读取）

**关键规则**：用户只能写 Spec，控制器只能写 Status。这个分离保证了：
- 用户不会意外覆盖实际状态信息
- 控制器不会意外修改用户的期望状态
- 任何偏差都清晰可见（Spec 说 X，Status 说 Y）

---

## 六、我们的实现设计

### 架构

```
┌──────────────────────────────────────────────┐
│              APIServer                       │
│  ┌────────────┐  ┌────────────────────────┐  │
│  │ Router     │  │ ResourceStore          │  │
│  │ (HTTP路由) │  │ (内存存储 + 版本控制)  │  │
│  └────────────┘  └────────────────────────┘  │
│  ┌────────────────────────────────────────┐  │
│  │ WatchHub (事件推送)                    │  │
│  └────────────────────────────────────────┘  │
├──────────────────────────────────────────────┤
│              ControllerManager               │
│  ┌────────────────┐  ┌────────────────────┐  │
│  │ SandboxCtrl    │  │ NetworkCtrl        │  │
│  └────────────────┘  └────────────────────┘  │
│  ┌────────────────┐  ┌────────────────────┐  │
│  │ StorageCtrl    │  │ FaaSCtrl           │  │
│  └────────────────┘  └────────────────────┘  │
└──────────────────────────────────────────────┘
```

### API 端点

```
GET    /api/v1/sandboxes           — 列出所有沙箱
GET    /api/v1/sandboxes/<name>    — 查询沙箱
POST   /api/v1/sandboxes           — 创建沙箱
PUT    /api/v1/sandboxes/<name>    — 更新沙箱
DELETE /api/v1/sandboxes/<name>    — 删除沙箱
GET    /api/v1/sandboxes?watch=true — Watch 沙箱变化

（同样模式适用于 nodes, volumes, functions, policies）
```

### 控制器实现

每个控制器实现 `Reconciler` 接口：

```go
type Reconciler interface {
    Reconcile(name string) error
}
```

控制器通过 Watch API 感知资源变化，然后执行 Reconcile 驱动数据面。

---

## 七、工业对比

| | Kubernetes | 我们的实现 |
|---|-----------|-----------|
| API Server | etcd + apiserver | 内存存储 + HTTP |
| Watch | etcd watch + chunked HTTP | channel + chunked HTTP |
| 控制器 | Controller Manager + 多个控制器 | ControllerManager + 5 个控制器 |
| Spec/Status | 严格分离 | 严格分离 |
| 声明式 | 全量声明式 | 全量声明式 |
| RBAC | 完整 RBAC | 无（教学简化） |
| Admission | Mutating + Validating webhook | 无（教学简化） |

---

**下一步**：阅读代码实现，理解 API Server、Watch、控制器循环的具体实现。