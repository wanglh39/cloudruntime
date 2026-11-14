package faas

import (
	"time"
)

// Function 函数定义
type Function struct {
	Name         string `json:"name"`
	Runtime      string `json:"runtime"`      // python, node, go
	Handler      string `json:"handler"`      // 入口函数名
	Code         string `json:"code"`         // 函数代码路径
	Timeout      int    `json:"timeout"`      // 执行超时（秒）
	MemoryLimit  int64  `json:"memoryLimit"`  // 内存限制
	MinInstances int    `json:"minInstances"` // 最小实例数
	MaxInstances int    `json:"maxInstances"` // 最大实例数
}

// InstanceStatus 实例状态
type InstanceStatus string

const (
	InstanceCreating InstanceStatus = "creating"
	InstanceReady    InstanceStatus = "ready"
	InstanceBusy     InstanceStatus = "busy"
	InstanceFrozen   InstanceStatus = "frozen"
	InstanceStopped  InstanceStatus = "stopped"
)

// Instance 函数实例
type Instance struct {
	ID           string         `json:"id"`
	FunctionName string         `json:"functionName"`
	Status       InstanceStatus `json:"status"`
	CreatedAt    time.Time      `json:"createdAt"`
	LastUsedAt   time.Time      `json:"lastUsedAt"`
	RequestCount int            `json:"requestCount"`
}

// EventSource 事件源类型
type EventSource string

const (
	SourceHTTP  EventSource = "http"
	SourceTimer EventSource = "timer"
	SourceQueue EventSource = "queue"
)

// Event 触发事件
type Event struct {
	Source       EventSource       `json:"source"`
	FunctionName string            `json:"functionName"`
	Payload      []byte            `json:"payload"`
	Headers      map[string]string `json:"headers"`
}

// InvokeResult 函数执行结果
type InvokeResult struct {
	Output    []byte        `json:"output"`
	Error     string        `json:"error"`
	Duration  time.Duration `json:"duration"`
	ColdStart bool          `json:"coldStart"`
}

// WarmPoolConfig 预热池配置
type WarmPoolConfig struct {
	MinReady       int           // 最少保持就绪的实例数
	MaxIdle        time.Duration // 实例空闲多久后回收
	ReplenishDelay time.Duration // 补充延迟
}
