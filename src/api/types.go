package api

import (
	"time"
)

// EventType 定义 Watch 事件类型
type EventType string

const (
	EventAdded    EventType = "ADDED"
	EventModified EventType = "MODIFIED"
	EventDeleted  EventType = "DELETED"
)

// WatchEvent 是 Watch 推送的事件
type WatchEvent struct {
	Type   EventType   `json:"type"`
	Object interface{} `json:"object"`
}

// ResourceMeta 资源元数据（所有资源共有）
type ResourceMeta struct {
	Name            string            `json:"name"`
	ResourceVersion int64             `json:"resourceVersion"`
	Labels          map[string]string `json:"labels"`
	CreatedAt       time.Time         `json:"createdAt"`
}

// ResourceKind 资源类型
type ResourceKind string

const (
	KindSandbox  ResourceKind = "Sandbox"
	KindNode     ResourceKind = "Node"
	KindVolume   ResourceKind = "Volume"
	KindFunction ResourceKind = "Function"
	KindPolicy   ResourceKind = "NetworkPolicy"
)

// Reconciler 控制器接口
type Reconciler interface {
	Reconcile(name string) error
}
