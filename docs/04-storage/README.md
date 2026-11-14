# 模块 4：存储抽象（Storage）

## 一、核心问题

**容器的文件系统是临时的——容器销毁后数据就丢失了。如何持久化数据？**

模块 1 创建的沙箱使用 Mount namespace 隔离了文件系统视图，但这个文件系统是基于宿主机内存的临时层。沙箱停止后，所有写入的数据都会消失。

存储抽象层要解决三个问题：
1. **持久化**：数据在沙箱生命周期之外存活
2. **共享**：多个沙箱可以访问同一份数据
3. **分层**：高效的镜像分发和复用

### 真实场景

- 数据库沙箱：数据必须持久化，沙箱重启后数据还在
- Web 服务器沙箱：需要读取共享的静态文件
- 日志收集沙箱：多个沙箱的日志汇聚到一个共享卷
- 镜像分发：1000 个沙箱用同一个基础镜像，不需要复制 1000 份

---

## 二、容器文件系统：分层模型

### OverlayFS：分层文件系统的内核实现

容器镜像不是单个文件，而是**一组只读层（layer）**叠加而成：

```
┌──────────────────────────┐
│  Upper (读写层)           │ ← 沙箱的写入都到这里
├──────────────────────────┤
│  Layer 3: 安装 Python     │ ← 只读
├──────────────────────────┤
│  Layer 2: apt-get update  │ ← 只读
├──────────────────────────┤
│  Layer 1: Ubuntu base     │ ← 只读
├──────────────────────────┤
│  Lower (底层存储)          │ ← 只读
└──────────────────────────┘
```

**OverlayFS 的工作原理**：

- **Lower layers**：只读层，可以是多个，按顺序叠加
- **Upper layer**：读写层，所有修改（创建、修改、删除）都写入这一层
- **Merged view**：合并视图，用户看到的最终文件系统
- **Workdir**：overlayfs 内部使用的工作目录

```bash
# 创建 overlayfs 挂载
mount -t overlay overlay \
  -o lowerdir=/layers/l1:/layers/l2:/layers/l3,upperdir=/upper,workdir=/work \
  /merged
```

### 写时复制（Copy-on-Write）

当沙箱修改一个来自只读层的文件时：
1. 文件从 Lower layer 复制到 Upper layer
2. 修改发生在 Upper layer 的副本上
3. Lower layer 的原文件不变

这意味着：
- 1000 个沙箱共享同一个基础镜像（只读层），内存/磁盘开销只有 1 份
- 每个沙箱的修改只占自己 Upper layer 的空间
- 删除操作在 Upper layer 创建一个 whiteout 文件，遮蔽 Lower layer 的文件

### 为什么分层？

| 特性 | 单层文件系统 | 分层文件系统 |
|------|------------|------------|
| 镜像分发 | 每次传输完整镜像 | 只传输变化的层 |
| 磁盘占用 | 每个容器完整复制 | 共享只读层，只存差异 |
| 构建缓存 | 每步重建全部 | 变化的步骤才重建 |
| 回滚 | 需要完整备份 | 切换到旧层即可 |

---

## 三、Volume：持久化存储

### Volume vs Bind Mount

| | Bind Mount | Volume |
|---|-----------|--------|
| 位置 | 宿主机任意路径 | 由存储驱动管理的路径 |
| 生命周期 | 独立于容器 | 独立于容器 |
| 可移植性 | 依赖宿主机路径 | 跨主机可迁移 |
| 备份 | 手动 | 存储驱动支持 |

### Volume 生命周期

```
创建 Volume → 挂载到沙箱 → 沙箱读写数据 → 卸载 Volume → 沙箱销毁
                                    ↑                          │
                                    └──── 数据持久化 ──────────┘
```

Volume 的生命周期完全独立于沙箱：
- 沙箱销毁后 Volume 仍然存在
- 新沙箱可以挂载已有 Volume，获得之前的数据
- Volume 可以在多个沙箱之间共享（多挂载）

### Volume 的访问模式

| 模式 | 说明 | 适用场景 |
|------|------|---------|
| **ReadWriteOnce (RWO)** | 单沙箱读写 | 数据库 |
| **ReadOnlyMany (ROX)** | 多沙箱只读 | 共享配置文件 |
| **ReadWriteMany (RWX)** | 多沙箱读写 | 分布式文件系统 |

---

## 四、存储类与动态供应

### 静态供应 vs 动态供应

**静态供应**：管理员预先创建好 Volume，用户挂载时指定名称。
**动态供应**：用户声明需求（"我要 10GB 存储"），系统自动创建。

```yaml
# 动态供应：用户声明 PVC
apiVersion: v1
kind: PersistentVolumeClaim
spec:
  accessModes: [ReadWriteOnce]
  storageClassName: fast-ssd
  resources:
    requests:
      storage: 10Gi
```

### StorageClass

StorageClass 定义了不同类型的存储：

| StorageClass | 后端 | 性能 | 成本 |
|-------------|------|------|------|
| fast-ssd | NVMe SSD | 极高 | 高 |
| standard | SATA SSD | 中 | 中 |
| bulk | HDD | 低 | 低 |
| network | NFS/CephFS | 取决于网络 | 取决于规模 |

---

## 五、CSI：Container Storage Interface

和 CNI 类似，CSI 是存储插件的标准接口：

```
CSI 插件接口：
  CreateVolume    — 创建卷
  DeleteVolume    — 删除卷
  ControllerPublish    — 将卷挂载到节点
  ControllerUnpublish  — 从节点卸载卷
  NodeStageVolume  — 在节点上准备卷
  NodePublishVolume — 将卷挂载到沙箱
  NodeUnpublishVolume — 从沙箱卸载卷
```

CSI 将存储操作分为 **控制面**（创建/删除/挂载到节点）和 **数据面**（在节点上挂载/卸载到沙箱），和 Cloud Runtime 的整体架构一致。

---

## 六、我们的实现设计

### 架构

```
┌──────────────────────────────────────────┐
│            StorageManager                │
│  ┌────────────┐  ┌────────────────────┐  │
│  │ VolumeMgr  │  │ LayerMgr           │  │
│  │ (卷管理)   │  │ (分层文件系统)     │  │
│  └────────────┘  └────────────────────┘  │
│  ┌────────────┐  ┌────────────────────┐  │
│  │ StorageCls │  │ SnapshotMgr        │  │
│  │ (存储类)   │  │ (快照)             │  │
│  └────────────┘  └────────────────────┘  │
└──────────────────────────────────────────┘
```

### 关键设计决策

1. **基于 overlayfs 的分层文件系统**：每个沙箱有 Upper/Work/Merged 目录
2. **主机本地 Volume**：Volume 数据存储在宿主机目录，通过 bind mount 挂载到沙箱
3. **三种访问模式**：RWO/ROX/RWX（RWX 通过共享目录实现）
4. **动态供应**：用户声明需求，StorageManager 自动创建 Volume
5. **快照支持**：Volume 可以创建快照，用于备份和恢复

### 数据模型

```go
// Volume 持久化卷
type Volume struct {
    Name         string
    SizeBytes    int64
    AccessMode   AccessMode    // RWO, ROX, RWX
    StorageClass string
    Path         string        // 宿主机上的路径
    Status       VolumeStatus  // available, bound, released
}

// Layer 文件系统层
type Layer struct {
    Name     string
    Path     string        // 层数据的路径
    Parent   string        // 父层名称（链式结构）
    ReadOnly bool
}

// Filesystem 沙箱的分层文件系统
type Filesystem struct {
    SandboxName string
    LowerDirs   []string    // 只读层路径列表
    UpperDir    string      // 读写层路径
    WorkDir     string      // overlayfs 工作目录
    MergedDir   string      // 合并视图路径
}
```

---

## 七、工业对比

| 方案 | 类型 | 分层 | 动态供应 | 快照 | 适用场景 |
|------|------|------|---------|------|---------|
| **overlayfs** | 内置 | ✅ | — | — | 本地容器 |
| **containerd snapshotter** | 内置 | ✅ | — | ✅ | 容器运行时 |
| **Longhorn** | CSI | — | ✅ | ✅ | K8s 分布式存储 |
| **Ceph RBD** | CSI | — | ✅ | ✅ | 块存储 |
| **NFS** | CSI | — | ✅ | — | 文件共享 |
| **我们的实现** | 内置 | ✅ | ✅ | ✅ | 教学 |

---

**下一步**：阅读代码实现，理解 overlayfs 分层、Volume 生命周期、动态供应的具体实现。