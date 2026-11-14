package network

// NetworkConfig 网络配置
type NetworkConfig struct {
	Subnet  string `json:"subnet"`
	Gateway string `json:"gateway"`
	Bridge  string `json:"bridge"`
}

// PortMap 端口映射
type PortMap struct {
	HostPort      int    `json:"hostPort"`
	ContainerPort int    `json:"containerPort"`
	Protocol      string `json:"protocol"`
}

// SandboxNetwork 沙箱的网络配置
type SandboxNetwork struct {
	SandboxName string    `json:"sandboxName"`
	IP          string    `json:"ip"`
	VethHost    string    `json:"vethHost"`
	VethSandbox string    `json:"vethSandbox"`
	PortMaps    []PortMap `json:"portMaps"`
}

// NetworkPolicy 网络策略
type NetworkPolicy struct {
	Name      string   `json:"name"`
	Sandbox   string   `json:"sandbox"`
	AllowFrom []string `json:"allowFrom"`
	AllowTo   []string `json:"allowTo"`
}

// DefaultConfig 默认网络配置
func DefaultConfig() NetworkConfig {
	return NetworkConfig{
		Subnet:  "10.0.0.0/24",
		Gateway: "10.0.0.1",
		Bridge:  "cbr0",
	}
}
