package network

import (
	"fmt"
	"sync"
)

// NetworkManager 管理沙箱的网络配置
// 负责 IP 分配、veth pair 创建、bridge 连接、端口映射、网络策略
type NetworkManager struct {
	mu        sync.Mutex
	config    NetworkConfig
	ipam      *IPAM
	sandboxes map[string]*SandboxNetwork
	policies  map[string]*NetworkPolicy
}

func NewNetworkManager(config NetworkConfig) (*NetworkManager, error) {
	ipam, err := NewIPAM(config.Subnet, config.Gateway)
	if err != nil {
		return nil, err
	}

	return &NetworkManager{
		config:    config,
		ipam:      ipam,
		sandboxes: make(map[string]*SandboxNetwork),
		policies:  make(map[string]*NetworkPolicy),
	}, nil
}

// Connect 为沙箱配置网络
// 1. 分配 IP
// 2. 创建 veth pair
// 3. 将 veth 一端连接到 bridge
// 4. 将 veth 另一端放入沙箱的 network namespace
// 5. 配置端口映射
func (m *NetworkManager) Connect(sandboxName string, portMaps []PortMap) (*SandboxNetwork, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, exists := m.sandboxes[sandboxName]; exists {
		return nil, fmt.Errorf("sandbox %s already connected", sandboxName)
	}

	// 1. 分配 IP
	ip, err := m.ipam.Allocate()
	if err != nil {
		return nil, fmt.Errorf("IP allocation failed: %w", err)
	}

	// 2. 创建 veth pair 名称
	vethHost := "veth" + sandboxName[:4] + "h"
	vethSandbox := "veth" + sandboxName[:4] + "s"

	// 3. 执行平台相关的网络配置
	if err := m.setupNetwork(sandboxName, ip, vethHost, vethSandbox); err != nil {
		m.ipam.Release(ip)
		return nil, fmt.Errorf("network setup failed: %w", err)
	}

	// 4. 配置端口映射
	if err := m.setupPortMapping(ip, portMaps); err != nil {
		m.cleanupNetwork(sandboxName, vethHost)
		m.ipam.Release(ip)
		return nil, fmt.Errorf("port mapping failed: %w", err)
	}

	sn := &SandboxNetwork{
		SandboxName: sandboxName,
		IP:          ip,
		VethHost:    vethHost,
		VethSandbox: vethSandbox,
		PortMaps:    portMaps,
	}

	m.sandboxes[sandboxName] = sn
	return sn, nil
}

// Disconnect 清理沙箱的网络配置
func (m *NetworkManager) Disconnect(sandboxName string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	sn, ok := m.sandboxes[sandboxName]
	if !ok {
		return fmt.Errorf("sandbox %s not connected", sandboxName)
	}

	// 清理端口映射
	m.cleanupPortMapping(sn.IP, sn.PortMaps)

	// 清理 veth pair 和 bridge 连接
	m.cleanupNetwork(sandboxName, sn.VethHost)

	// 释放 IP
	m.ipam.Release(sn.IP)

	delete(m.sandboxes, sandboxName)
	return nil
}

// GetSandboxNetwork 查询沙箱的网络配置
func (m *NetworkManager) GetSandboxNetwork(sandboxName string) (*SandboxNetwork, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	sn, ok := m.sandboxes[sandboxName]
	if !ok {
		return nil, fmt.Errorf("sandbox %s not found", sandboxName)
	}
	return sn, nil
}

// ListConnected 列出所有已连接网络的沙箱
func (m *NetworkManager) ListConnected() []*SandboxNetwork {
	m.mu.Lock()
	defer m.mu.Unlock()

	result := make([]*SandboxNetwork, 0, len(m.sandboxes))
	for _, sn := range m.sandboxes {
		result = append(result, sn)
	}
	return result
}

// AddPolicy 添加网络策略
func (m *NetworkManager) AddPolicy(policy *NetworkPolicy) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.policies[policy.Name] = policy
	m.applyPolicy(policy)
}

// RemovePolicy 移除网络策略
func (m *NetworkManager) RemovePolicy(name string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	policy, ok := m.policies[name]
	if !ok {
		return
	}

	m.removePolicyRules(policy)
	delete(m.policies, name)
}

// ListPolicies 列出所有网络策略
func (m *NetworkManager) ListPolicies() []*NetworkPolicy {
	m.mu.Lock()
	defer m.mu.Unlock()

	result := make([]*NetworkPolicy, 0, len(m.policies))
	for _, p := range m.policies {
		result = append(result, p)
	}
	return result
}

// IsAllowed 检查源沙箱是否被允许访问目标沙箱
// 如果目标沙箱有策略，则检查 AllowFrom 列表；没有策略则默认允许
func (m *NetworkManager) IsAllowed(fromSandbox, toSandbox string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()

	for _, policy := range m.policies {
		if policy.Sandbox == toSandbox {
			for _, allowed := range policy.AllowFrom {
				if allowed == fromSandbox || allowed == "*" {
					return true
				}
			}
			return false
		}
	}
	return true
}

// Config 返回网络配置
func (m *NetworkManager) Config() NetworkConfig {
	return m.config
}
