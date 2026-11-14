//go:build linux
// +build linux

package network

import (
	"fmt"
	"os/exec"
)

// setupNetwork 在 Linux 上创建 veth pair 并连接到 bridge
func (m *NetworkManager) setupNetwork(sandboxName, ip, vethHost, vethSandbox string) error {
	// 1. 创建 bridge（如果不存在）
	if err := runCmd("ip", "link", "add", "name", m.config.Bridge, "type", "bridge"); err != nil {
		// bridge 可能已存在，忽略错误
	}

	// 2. 设置 bridge IP 并启用
	runCmd("ip", "addr", "add", m.config.Gateway+"/24", "dev", m.config.Bridge)
	runCmd("ip", "link", "set", m.config.Bridge, "up")

	// 3. 创建 veth pair
	if err := runCmd("ip", "link", "add", vethHost, "type", "veth", "peer", "name", vethSandbox); err != nil {
		return fmt.Errorf("failed to create veth pair: %w", err)
	}

	// 4. 将 vethHost 连接到 bridge
	if err := runCmd("ip", "link", "set", vethHost, "master", m.config.Bridge); err != nil {
		return fmt.Errorf("failed to attach veth to bridge: %w", err)
	}

	// 5. 启用 vethHost
	if err := runCmd("ip", "link", "set", vethHost, "up"); err != nil {
		return fmt.Errorf("failed to bring up veth host: %w", err)
	}

	// 6. 将 vethSandbox 移入沙箱的 network namespace
	// 注意：需要 sandbox 的 PID，这里简化处理
	// 实际实现中需要通过 /proc/<pid>/ns/net 获取 namespace
	// 教学版：vethSandbox 留在宿主机，仅做演示

	return nil
}

// cleanupNetwork 清理 veth pair 和 bridge 连接
func (m *NetworkManager) cleanupNetwork(sandboxName, vethHost string) {
	// 删除 veth pair（删除一端会自动删除另一端）
	runCmd("ip", "link", "del", vethHost)
}

// setupPortMapping 配置 iptables 端口映射（DNAT）
func (m *NetworkManager) setupPortMapping(containerIP string, portMaps []PortMap) error {
	for _, pm := range portMaps {
		proto := pm.Protocol
		if proto == "" {
			proto = "tcp"
		}

		// DNAT：将宿主机端口的流量转发到容器端口
		rule := fmt.Sprintf("-t nat -A PREROUTING -p %s --dport %d -j DNAT --to-destination %s:%d",
			proto, pm.HostPort, containerIP, pm.ContainerPort)
		runCmd("iptables", stringSlice("iptables", "-t", "nat", "-A", "PREROUTING", "-p", proto, "--dport", fmt.Sprintf("%d", pm.HostPort), "-j", "DNAT", "--to-destination", fmt.Sprintf("%s:%d", containerIP, pm.ContainerPort))...)

		// 允许本地回环访问
		rule2 := fmt.Sprintf("-t nat -A OUTPUT -p %s --dport %d -j DNAT --to-destination %s:%d",
			proto, pm.HostPort, containerIP, pm.ContainerPort)
		_ = rule2
		runCmd("iptables", "-t", "nat", "-A", "OUTPUT", "-p", proto, "--dport", fmt.Sprintf("%d", pm.HostPort), "-j", "DNAT", "--to-destination", fmt.Sprintf("%s:%d", containerIP, pm.ContainerPort))
	}
	return nil
}

// cleanupPortMapping 清理 iptables 端口映射规则
func (m *NetworkManager) cleanupPortMapping(containerIP string, portMaps []PortMap) {
	for _, pm := range portMaps {
		proto := pm.Protocol
		if proto == "" {
			proto = "tcp"
		}
		runCmd("iptables", "-t", "nat", "-D", "PREROUTING", "-p", proto, "--dport", fmt.Sprintf("%d", pm.HostPort), "-j", "DNAT", "--to-destination", fmt.Sprintf("%s:%d", containerIP, pm.ContainerPort))
		runCmd("iptables", "-t", "nat", "-D", "OUTPUT", "-p", proto, "--dport", fmt.Sprintf("%d", pm.HostPort), "-j", "DNAT", "--to-destination", fmt.Sprintf("%s:%d", containerIP, pm.ContainerPort))
	}
}

// applyPolicy 应用网络策略（iptables 规则）
func (m *NetworkManager) applyPolicy(policy *NetworkPolicy) {
	sn, ok := m.sandboxes[policy.Sandbox]
	if !ok {
		return
	}

	// 默认拒绝所有入站流量
	runCmd("iptables", "-A", "FORWARD", "-d", sn.IP, "-j", "DROP")

	// 允许 AllowFrom 列表中的源
	for _, fromName := range policy.AllowFrom {
		if fromName == "*" {
			// 允许所有
			runCmd("iptables", "-I", "FORWARD", "-d", sn.IP, "-j", "ACCEPT")
			break
		}
		fromSn, ok := m.sandboxes[fromName]
		if !ok {
			continue
		}
		runCmd("iptables", "-I", "FORWARD", "-s", fromSn.IP, "-d", sn.IP, "-j", "ACCEPT")
	}
}

// removePolicyRules 移除网络策略对应的 iptables 规则
func (m *NetworkManager) removePolicyRules(policy *NetworkPolicy) {
	sn, ok := m.sandboxes[policy.Sandbox]
	if !ok {
		return
	}

	runCmd("iptables", "-D", "FORWARD", "-d", sn.IP, "-j", "DROP")

	for _, fromName := range policy.AllowFrom {
		if fromName == "*" {
			runCmd("iptables", "-D", "FORWARD", "-d", sn.IP, "-j", "ACCEPT")
			break
		}
		fromSn, ok := m.sandboxes[fromName]
		if !ok {
			continue
		}
		runCmd("iptables", "-D", "FORWARD", "-s", fromSn.IP, "-d", sn.IP, "-j", "ACCEPT")
	}
}

func runCmd(name string, args ...string) error {
	cmd := exec.Command(name, args...)
	return cmd.Run()
}

func stringSlice(s ...string) []string {
	return s
}
