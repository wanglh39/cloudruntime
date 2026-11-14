package network

import (
	"fmt"
	"net"
	"sync"
)

// IPAM 主机本地 IP 地址管理器
// 从固定子网中顺序分配 IP，回收后可重新分配
type IPAM struct {
	mu        sync.Mutex
	subnet    *net.IPNet
	gateway   net.IP
	allocated map[string]bool // IP string -> allocated
	next      int             // next offset from subnet base
}

// NewIPAM 创建 IPAM 实例
// subnet 格式如 "10.0.0.0/24"，gateway 如 "10.0.0.1"
func NewIPAM(subnet, gateway string) (*IPAM, error) {
	_, ipNet, err := net.ParseCIDR(subnet)
	if err != nil {
		return nil, fmt.Errorf("invalid subnet %s: %w", subnet, err)
	}

	gw := net.ParseIP(gateway)
	if gw == nil {
		return nil, fmt.Errorf("invalid gateway %s", gateway)
	}

	ipam := &IPAM{
		subnet:    ipNet,
		gateway:   gw,
		allocated: make(map[string]bool),
		next:      2, // 从 .2 开始（.1 是 gateway）
	}

	// 标记 gateway 为已分配
	ipam.allocated[gw.String()] = true

	return ipam, nil
}

// Allocate 分配一个可用的 IP 地址
func (i *IPAM) Allocate() (string, error) {
	i.mu.Lock()
	defer i.mu.Unlock()

	// 从 next 开始扫描可用 IP
	base := i.subnet.IP.To4()
	if base == nil {
		return "", fmt.Errorf("subnet is not IPv4")
	}

	// 先扫描已释放的 IP（低地址优先复用，减少碎片）
	for offset := 2; offset < i.next; offset++ {
		ip := net.IPv4(base[0], base[1], base[2], byte(offset))
		ipStr := ip.String()

		if !i.allocated[ipStr] {
			i.allocated[ipStr] = true
			return ipStr, nil
		}
	}

	// 再扫描新 IP
	for offset := i.next; offset < 255; offset++ {
		ip := net.IPv4(base[0], base[1], base[2], byte(offset))
		ipStr := ip.String()

		if !i.allocated[ipStr] {
			i.allocated[ipStr] = true
			i.next = offset + 1
			return ipStr, nil
		}
	}

	return "", fmt.Errorf("no available IP in subnet %s", i.subnet.String())
}

// Release 释放一个 IP 地址
func (i *IPAM) Release(ip string) {
	i.mu.Lock()
	defer i.mu.Unlock()
	delete(i.allocated, ip)
}

// IsAllocated 检查 IP 是否已分配
func (i *IPAM) IsAllocated(ip string) bool {
	i.mu.Lock()
	defer i.mu.Unlock()
	return i.allocated[ip]
}

// Gateway 返回网关地址
func (i *IPAM) Gateway() string {
	return i.gateway.String()
}

// Subnet 返回子网 CIDR
func (i *IPAM) Subnet() string {
	return i.subnet.String()
}
