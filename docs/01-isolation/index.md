# 模块 1：隔离与沙箱（Isolation & Sandbox）

## 一、核心问题

**如何在同一台物理机上安全地运行多个互不信任的程序？**

这是 Cloud Runtime 的第一块基石。如果没有隔离，所有用户代码共享同一个 OS 环境，一个程序的崩溃或恶意行为会影响所有人。

### 真实场景

想象你运营一个 FaaS 平台，用户 A 和用户 B 的函数运行在同一台机器上：
- 用户 A 的函数有 bug，吃光了所有内存 → 用户 B 的函数被 OOM killed
- 用户 A 的函数读取 `/etc/passwd` → 安全漏洞
- 用户 A 的函数监听了 8080 端口 → 用户 B 的函数无法绑定同一端口
- 用户 A 的函数 fork bomb → 整台机器瘫痪

隔离的目标就是让每个函数**以为自己在独占一台机器**，同时实际上共享物理资源。

---

## 二、Linux Namespace：隔离的内核基础

Namespace 是 Linux 内核提供的隔离机制。它让进程看到一个**独立的系统资源视图**。

### 六种 Namespace

| Namespace | 隔离什么 | 系统调用标志 | 通俗解释 |
|-----------|---------|-------------|---------|
| **PID** | 进程 ID | `CLONE_NEWPID` | 沙箱内的进程看不到外面的进程，自己是 PID 1 |
| **Network** | 网络栈 | `CLONE_NEWNET` | 沙箱有独立的网卡、路由表、iptables 规则 |
| **Mount** | 文件系统挂载点 | `CLONE_NEWNS` | 沙箱有独立的文件系统视图 |
| **UTS** | hostname/domain | `CLONE_NEWUTS` | 沙箱有独立的主机名 |
| **IPC** | 进程间通信 | `CLONE_NEWIPC` | 沙箱有独立的消息队列、共享内存 |
| **User** | 用户/组 ID | `CLONE_NEWUSER` | 沙箱内的 root 映射到外面的普通用户 |

### 为什么是这六种？

这不是随便选的。每一种 namespace 隔离的都是一个**可能产生冲突的系统资源维度**：

- **PID**：进程之间会互相发送信号（kill），需要隔离
- **Network**：端口绑定冲突、路由干扰，需要隔离
- **Mount**：文件系统路径冲突，需要隔离
- **UTS**：hostname 用于服务发现，需要隔离
- **IPC**：共享内存/消息队列可能被其他进程读写，需要隔离
- **User**：root 权限太危险，需要将沙箱内的 root 映射为外部普通用户

### Namespace 的创建方式

```go
// 方式 1：clone — 创建新进程时指定 namespace
syscall.Clone(syscall.CLONE_NEWPID|syscall.CLONE_NEWNET, ...)

// 方式 2：unshare — 让当前进程进入新 namespace
syscall.Unshare(syscall.CLONE_NEWPID|syscall.CLONE_NEWNET)

// 方式 3：setns — 让进程加入已有的 namespace
fd, _ := os.Open("/proc/1234/ns/pid")
syscall.Setns(int(fd.Fd()), syscall.CLONE_NEWPID)
```

**我们用 `clone` 方式**，因为它最清晰地展示了"创建一个隔离进程"的过程。

### Namespace 的局限性

Namespace 只提供**视图隔离**，不提供**安全隔离**：
- 所有沙箱仍然共享同一个 Linux 内核
- 内核漏洞（如 dirty pipe）可以突破 namespace 隔离
- 沙箱内的 root 仍然是真正的 root（除非用 User namespace）

这就是为什么工业界还有更强的隔离方案：
- **gVisor**：用用户态内核拦截系统调用，沙箱不直接接触内核
- **Firecracker**：用 KVM 创建真正的 microVM，每个沙箱有独立内核
- **Kata Containers**：结合 VM + 容器接口

---

## 三、Cgroups：资源限制的内核基础

Namespace 解决了"看得到什么"的问题，cgroups 解决了"能用多少"的问题。

### Cgroups v2 核心概念

```
/sys/fs/cgroup/
├── cpu.max          # CPU 限制：格式 "quota period"，如 "50000 100000" = 50%
├── memory.max       # 内存限制：字节数，如 536870912 = 512MB
├── io.max           # IO 限制
├── pids.max         # 进程数限制
└── sandbox-1/       # 子 cgroup（每个沙箱一个）
    ├── cpu.max
    ├── memory.max
    ├── cgroup.procs  # 属于这个 cgroup 的进程列表
    └── ...
```

### 为什么需要 cgroups？

没有 cgroups，即使 namespace 隔离了视图，进程仍然可以：
- 吃光 CPU（fork bomb 或死循环）
- 吃光内存（内存泄漏）
- 吃光磁盘 IO（大量写入）
- 创建无限进程

cgroups 让我们能够说："这个沙箱最多用 0.5 CPU、512MB 内存、100 个进程"。

### Cgroups v1 vs v2

| | v1 | v2 |
|---|---|---|
| 结构 | 每个控制器独立层级 | 统一层级 |
| 路径 | `/sys/fs/cgroup/cpu/`, `/sys/fs/cgroup/memory/` | `/sys/fs/cgroup/` |
| 状态 | 已弃用 | 当前标准 |

我们使用 cgroups v2（Linux 内核 4.15+ 默认启用）。

---

## 四、我们的实现设计

### 架构

```
SandboxManager
    │
    ├── Create(spec) → Sandbox
    │     ├── 创建 cgroup 目录
    │     ├── 设置资源限制
    │     ├── clone() 创建隔离进程
    │     │     ├── CLONE_NEWPID  (PID namespace)
    │     │     ├── CLONE_NEWNET  (Network namespace)
    │     │     ├── CLONE_NEWNS   (Mount namespace)
    │     │     ├── CLONE_NEWUTS  (UTS namespace)
    │     │     ├── CLONE_NEWIPC  (IPC namespace)
    │     │     └── CLONE_NEWUSER (User namespace)
    │     └── 将进程 PID 写入 cgroup.procs
    │
    ├── Stop(name) → error
    │     └── 发送 SIGTERM/SIGKILL
    │
    ├── Delete(name) → error
    │     ├── 杀死残留进程
    │     └── 删除 cgroup 目录
    │
    └── List() → []Sandbox
          └── 遍历管理的沙箱列表
```

### 关键设计决策

1. **用 `cmd.SysProcAttr` 设置 namespace 标志**：Go 标准库提供了 `syscall.SysProcAttr` 结构体，可以在 `exec.Cmd` 中直接设置 clone flags，比手动调用 `syscall.Clone` 更安全。

2. **User namespace 做 UID 映射**：沙箱内看到的 root (UID 0) 实际上映射到外部的当前用户，避免真正的 root 权限。

3. **Cgroups v2 统一层级**：每个沙箱在 `/sys/fs/cgroup/cloudruntime/<name>/` 下创建子组。

4. **Mount namespace + pivot_root**：在沙箱内切换根文件系统，让沙箱只能看到自己的文件系统。

---

## 五、工业对比

| 方案 | 隔离强度 | 启动时间 | 内存开销 | 实现语言 | 适用场景 |
|------|---------|---------|---------|---------|---------|
| **runc** | 弱（namespace） | ~100ms | ~0 | Go | 开发环境、可信工作负载 |
| **gVisor** | 中（用户态内核） | ~200ms | ~50MB | Go | 不信任的工作负载 |
| **Firecracker** | 强（KVM microVM） | ~125ms | ~5MB | Rust | FaaS、多租户 |
| **Kata** | 强（VM + 容器） | ~1s | ~100MB | Go/Rust | 企业容器平台 |
| **我们的实现** | 弱（namespace） | ~50ms | ~0 | Go | 教学 |

我们的实现和 runc 最接近，但大幅简化：
- runc 支持 rootfs 配置、OCI runtime spec、terminal、hooks 等
- 我们只实现核心的 namespace + cgroups 隔离

---

## 六、前置条件

运行此模块需要 **Linux 环境**（Windows/macOS 无法使用 namespace/cgroups）。

如果你在 Windows 上，可以：
1. 使用 WSL2（Windows Subsystem for Linux）
2. 使用 Docker Desktop 的 Linux VM
3. 使用云上的 Linux 虚拟机

代码可以编译，但运行时会在非 Linux 环境给出明确提示。

---

**下一步**：阅读代码实现 `src/runtime/sandbox.go`，理解每个 namespace 标志和 cgroup 设置的具体作用。