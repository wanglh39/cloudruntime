//go:build !linux
// +build !linux

package network

import (
	"fmt"
	"runtime"
)

func (m *NetworkManager) setupNetwork(sandboxName, ip, vethHost, vethSandbox string) error {
	return fmt.Errorf("network setup requires Linux, current OS: %s; use WSL2 or a Linux VM", runtime.GOOS)
}

func (m *NetworkManager) cleanupNetwork(sandboxName, vethHost string) {
}

func (m *NetworkManager) setupPortMapping(containerIP string, portMaps []PortMap) error {
	return fmt.Errorf("port mapping requires Linux iptables")
}

func (m *NetworkManager) cleanupPortMapping(containerIP string, portMaps []PortMap) {
}

func (m *NetworkManager) applyPolicy(policy *NetworkPolicy) {
}

func (m *NetworkManager) removePolicyRules(policy *NetworkPolicy) {
}
