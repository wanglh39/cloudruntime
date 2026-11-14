package runtime

// SandboxSpec 定义沙箱的期望状态（声明式 API 的体现）
type SandboxSpec struct {
	Name     string   `json:"name"`
	Command  []string `json:"command"`
	CPULimit int      `json:"cpuLimit"` // millicores, e.g. 500 = 0.5 CPU
	MemLimit int64    `json:"memLimit"` // bytes
	Env      []string `json:"env"`
}

// SandboxStatus 定义沙箱的实际状态（控制面观察的对象）
type SandboxStatus struct {
	Name   string `json:"name"`
	State  string `json:"state"` // pending, running, stopped, failed
	PID    int    `json:"pid"`
	IP     string `json:"ip"`
	CPUUse int    `json:"cpuUse"` // actual CPU usage in millicores
	MemUse int64  `json:"memUse"` // actual memory usage in bytes
}

// Sandbox 是 Spec + Status + Metadata 的组合（一切皆资源的体现）
type Sandbox struct {
	Spec   SandboxSpec   `json:"spec"`
	Status SandboxStatus `json:"status"`
}
