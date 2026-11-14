package scheduler

// SchedulingPolicy 定义调度策略
type SchedulingPolicy string

const (
	PolicyBinPack SchedulingPolicy = "binpack"
	PolicySpread  SchedulingPolicy = "spread"
)

// NodeSpec 定义节点的资源容量
type NodeSpec struct {
	Name   string            `json:"name"`
	CPUCap int               `json:"cpuCap"`
	MemCap int64             `json:"memCap"`
	Labels map[string]string `json:"labels"`
}

// NodeStatus 定义节点的实际资源使用
type NodeStatus struct {
	Name      string `json:"name"`
	CPUUsed   int    `json:"cpuUsed"`
	MemUsed   int64  `json:"memUsed"`
	TaskCount int    `json:"taskCount"`
}

// Node 是节点的完整表示
type Node struct {
	Spec   NodeSpec   `json:"spec"`
	Status NodeStatus `json:"status"`
}

// TaskSpec 定义任务的资源需求和调度约束
type TaskSpec struct {
	Name         string            `json:"name"`
	CPURequest   int               `json:"cpuRequest"`
	MemRequest   int64             `json:"memRequest"`
	NodeSelector map[string]string `json:"nodeSelector"`
	Policy       SchedulingPolicy  `json:"policy"`
}

// TaskStatus 定义任务的调度状态
type TaskStatus struct {
	Name     string `json:"name"`
	NodeName string `json:"nodeName"`
	Phase    string `json:"phase"`
	Message  string `json:"message"`
}

// Task 是任务的完整表示
type Task struct {
	Spec   TaskSpec   `json:"spec"`
	Status TaskStatus `json:"status"`
}

// EventType 定义调度事件类型
type EventType string

const (
	EventTaskAdded   EventType = "task_added"
	EventTaskDeleted EventType = "task_deleted"
	EventNodeAdded   EventType = "node_added"
	EventNodeDeleted EventType = "node_deleted"
)

// ScheduleEvent 是调度器接收的事件
type ScheduleEvent struct {
	Type EventType `json:"type"`
	Task *Task     `json:"task,omitempty"`
	Node *Node     `json:"node,omitempty"`
}
