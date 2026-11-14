# 模块 3：网络抽象（Networking）

## 一、核心问题

**隔离环境中的进程如何与外界通信？**

模块 1 的 namespace 隔离给了每个沙箱一个独立的网络栈——独立的网卡、路由表、iptables 规则。但隔离之后，沙箱就成了孤岛：没有 IP 地址，无法访问外部网络，其他沙箱也无法访问它。

网络抽象层要解决的就是：**在隔离的基础上重建连通性**。

### 真实场景

你创建了 3 个沙箱：
- sandbox-a：一个 Web 服务器，需要监听 8080 端口
- sandbox-b：一个数据库，需要被 sandbox-a 访问
- sandbox-c：一个 API 服务，需要访问外部互联网

每个沙箱都在自己的 Network namespace 中，它们需要：
1. 各自有一个 IP 地址
2. sandbox-a 能通过 IP 访问 sandbox-b
3. sandbox-c 能通过 NAT 访问外部互联网
4. 外部用户能通过端口映射访问 sandbox-a 的 8080

---

## 二、Linux 虚拟网络基础

### 核心虚拟设备

| 设备 | 作用 | 类比 |
|------|------|------|
| **veth pair** | 虚拟以太网线，连接两个 namespace | 一根网线两头 |
| **bridge** | 虚拟交换机，连接多个 veth | 一台交换机 |
| **tun/tap** | 虚拟网卡，用户态处理网络包 | 软件实现的网卡 |
| **vxlan** | 隧道协议，跨节点二层网络 | VPN 隧道 |

### veth pair：连接两个世界

veth pair 是最基础的网络连接原语。它是一对虚拟网卡，从一端写入的数据包会从另一端读出。

```
Namespace A          Namespace B
┌──────────┐         ┌──────────┐
│  veth-a  │←──────→│  veth-b  │
│ 10.0.0.2 │  veth   │ 10.0.0.3 │
└──────────┘  pair   └──────────┘
```

创建 veth pair 并放入不同 namespace：
```bash
# 创建 veth pair
ip link add veth-a type veth peer name veth-b

# 将 veth-b 移入 sandbox 的 network namespace
ip link set veth-b netns <sandbox-pid>

# 在 sandbox 内配置 IP
nsenter -t <sandbox-pid> -n ip addr add 10.0.0.3/24 dev veth-b
nsenter -t <sandbox-pid> -n ip link set veth-b up
```

### bridge：虚拟交换机

bridge（网桥）是 Linux 内核实现的二层交换机。多个 veth pair 的一端连接到 bridge 上，就像多台机器插到同一台交换机：

```
         ┌───────────┐
         │   bridge  │  (10.0.0.1/24)
         │  (cbr0)   │
         └──┬──┬──┬──┘
            │  │  │
      ┌─────┘  │  └─────┐
      │        │        │
  ┌───┴──┐ ┌──┴───┐ ┌──┴───┐
  │veth-a│ │veth-b│ │veth-c│
  │10.0.0.2│ │10.0.0.3│ │10.0.0.4│
  └──────┘ └──────┘ └──────┘
  Sandbox A  Sandbox B  Sandbox C
```

所有连接到同一 bridge 的沙箱可以在二层互相通信。

### NAT：访问外部网络

沙箱默认只能和同一 bridge 上的其他沙箱通信。要访问外部互联网，需要 NAT（Network Address Translation）：

```
Sandbox (10.0.0.2) → 请求 8.8.8.8
    │
    ├── 到达 bridge (10.0.0.1)
    ├── iptables MASQUERADE 规则：将源 IP 从 10.0.0.2 改为宿主机 IP
    ├── 宿主机发出请求
    ├── 响应返回宿主机
    ├── iptables DNAT：将目标 IP 从宿主机 IP 改回 10.0.0.2
    └── 响应到达 Sandbox
```

### 端口映射：外部访问沙箱

外部用户无法直接访问沙箱的私有 IP。需要端口映射（DNAT）：

```
外部请求 → 宿主机:8080
    │
    ├── iptables DNAT 规则：目标改为 10.0.0.2:8080
    └── 请求到达 Sandbox A
```

---

## 三、IPAM：IP 地址管理

### 问题

每个沙箱需要一个 IP 地址。谁来分配？如何避免冲突？

### 方案

| 方案 | 描述 | 适用场景 |
|------|------|---------|
| **主机本地 IPAM** | 每个节点独立分配，使用不重叠的子网 | 单节点、简单场景 |
| **集中式 IPAM** | 中央服务统一分配，保证全局唯一 | 多节点、生产环境 |
| **分布式 IPAM** | 基于 etcd/consul 的分布式分配 | 大规模集群 |

**我们的实现**：主机本地 IPAM，使用固定子网 `10.0.0.0/24`，顺序分配 IP。

### CIDR 与子网

```
10.0.0.0/24
├── 网络地址：10.0.0.0（不可用）
├── 可用地址：10.0.0.1 ~ 10.0.0.254
├── 广播地址：10.0.0.255（不可用）
└── 网关地址：10.0.0.1（bridge 使用）
```

---

## 四、跨节点网络：Overlay

单节点网络用 bridge 就够了。但多节点场景下，不同节点上的沙箱需要通信，这就需要 overlay 网络。

### VXLAN

VXLAN（Virtual eXtensible LAN）是最常用的 overlay 协议：

```
节点 A                                节点 B
┌──────────────┐                     ┌──────────────┐
│ Sandbox:10.0.0.2│                  │ Sandbox:10.0.0.5│
│      ↓        │                     │      ↓        │
│   bridge      │                     │   bridge      │
│      ↓        │                     │      ↓        │
│   vxlan0      │←── UDP 隧道 ──→│   vxlan0      │
│      ↓        │   封装/解封装     │      ↓        │
│  eth0 (物理)  │←── 物理网络 ──→│  eth0 (物理)  │
└──────────────┘                     └──────────────┘
```

VXLAN 将二层帧封装在 UDP 包中，通过物理网络传输。对沙箱来说，它们就像在同一二层网络中。

### 为什么需要 Overlay？

- **扁平地址空间**：所有沙箱使用同一子网，不需要路由
- **透明性**：沙箱不需要知道物理网络拓扑
- **灵活性**：IP 地址不绑定物理位置，沙箱迁移后 IP 不变

---

## 五、网络策略（NetworkPolicy）

### 问题

默认情况下，同一 bridge 上的所有沙箱可以互相通信。但在生产环境中，你需要：
- sandbox-a 可以访问 sandbox-b（数据库）
- sandbox-c **不能**访问 sandbox-b（安全隔离）

### Kubernetes NetworkPolicy 模型

```yaml
# 只允许带 app=web 标签的 Pod 访问带 app=db 标签的 Pod 的 5432 端口
apiVersion: networking.k8s.io/v1
kind: NetworkPolicy
spec:
  podSelector:
    matchLabels:
      app: db
  ingress:
  - from:
    - podSelector:
        matchLabels:
          app: web
    ports:
    - port: 5432
```

### 实现方式

| 方案 | 层级 | 性能 | 复杂度 |
|------|------|------|--------|
| **iptables** | 三/四层 | 好 | 中 |
| **eBPF** | 二/三/四层 | 极好 | 高 |
| **Calico** | 三层（BGP） | 好 | 中 |
| **Cilium** | eBPF | 极好 | 高 |

**我们的实现**：用 iptables 规则实现简单的网络策略（允许/拒绝列表）。

---

## 六、CNI：Container Network Interface

### 为什么需要 CNI？

Cloud Runtime 不应该硬编码网络方案。不同的部署场景需要不同的网络：
- 公有云：云厂商的 VPC 网络
- 私有云：Calico/Flannel overlay
- 裸金属：直接路由

CNI 是一个**插件接口标准**，定义了网络插件必须实现的操作：

```
CNI 插件接口：
  ADD    — 为容器配置网络
  DEL    — 清理容器网络
  CHECK  — 检查容器网络状态
  VERSION — 报告支持的 CNI 版本
```

调用方式：
```bash
# CNI 插件是一个可执行文件，通过环境变量传参
CNI_COMMAND=ADD CNI_CONTAINERID=sandbox1 CNI_NETNS=/proc/1234/ns/net \
  /opt/cni/bin/bridge < config.json
```

### CNI 配置

```json
{
  "name": "cloudruntime",
  "type": "bridge",
  "bridge": "cbr0",
  "ipam": {
    "type": "host-local",
    "subnet": "10.0.0.0/24",
    "gateway": "10.0.0.1"
  }
}
```

---

## 七、我们的实现设计

### 架构

```
┌──────────────────────────────────────────────┐
│              NetworkManager                   │
│  ┌────────────┐  ┌────────────────────────┐  │
│  │   IPAM     │  │  BridgeManager         │  │
│  │ (IP 分配)  │  │  (创建/管理 bridge)    │  │
│  └────────────┘  └────────────────────────┘  │
│  ┌────────────┐  ┌────────────────────────┐  │
│  │ VethManager│  │  PolicyManager         │  │
│  │ (veth pair)│  │  (网络策略)            │  │
│  └────────────┘  └────────────────────────┘  │
│  ┌────────────────────────────────────────┐  │
│  │ PortMapper (端口映射/NAT)              │  │
│  └────────────────────────────────────────┘  │
└──────────────────────────────────────────────┘
```

### 关键设计决策

1. **单节点 bridge 网络**：不实现 VXLAN overlay（教学简化），但文档中解释原理
2. **主机本地 IPAM**：固定子网 10.0.0.0/24，顺序分配
3. **iptables 网络策略**：用 iptables 规则实现简单的 allow/deny 列表
4. **端口映射**：支持将宿主机端口映射到沙箱端口
5. **跨平台**：Linux 实现真实网络操作，非 Linux 用模拟数据

### 数据模型

```go
// NetworkConfig 网络配置
type NetworkConfig struct {
    Subnet   string  // "10.0.0.0/24"
    Gateway  string  // "10.0.0.1"
    Bridge   string  // "cbr0"
}

// SandboxNetwork 沙箱的网络配置
type SandboxNetwork struct {
    SandboxName string
    IP          string  // 分配的 IP
    VethHost    string  // 宿主机侧 veth 名
    VethSandbox string  // 沙箱侧 veth 名
    PortMaps    []PortMap
}

// PortMap 端口映射
type PortMap struct {
    HostPort      int
    ContainerPort int
    Protocol      string  // tcp/udp
}

// NetworkPolicy 网络策略
type NetworkPolicy struct {
    Name      string
    Sandbox   string           // 目标沙箱
    AllowFrom []string         // 允许访问的源沙箱列表
    AllowTo   []string         // 允许访问的目标沙箱列表
}
```

---

## 八、工业对比

| 方案 | 类型 | Overlay | 策略 | 适用规模 |
|------|------|---------|------|---------|
| **Flannel** | CNI | VXLAN/Host-GW | 无 | 小中型 |
| **Calico** | CNI | BGP/IPIP | iptables/eBPF | 中大型 |
| **Cilium** | CNI | VXLAN/Geneve | eBPF | 大型 |
| **AWS VPC CNI** | CNI | 无（直接 VPC） | SG | AWS 环境 |
| **我们的实现** | 内置 | 无（单节点） | iptables | 教学 |

---

**下一步**：阅读代码实现，理解 veth pair 创建、IP 分配、bridge 连接的具体过程。