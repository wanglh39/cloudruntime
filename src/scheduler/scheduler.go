package scheduler

import (
	"fmt"
	"sort"
	"sync"
)

// Scheduler 是集中式调度器
// 实现 Filter → Score → Bind 三阶段调度流程
type Scheduler struct {
	mu    sync.Mutex
	nodes map[string]*Node
	tasks map[string]*Task
}

func NewScheduler() *Scheduler {
	return &Scheduler{
		nodes: make(map[string]*Node),
		tasks: make(map[string]*Task),
	}
}

// SubmitTask 提交一个待调度的任务（同步调度）
func (s *Scheduler) SubmitTask(task *Task) {
	s.mu.Lock()
	defer s.mu.Unlock()

	task.Status.Phase = "pending"
	s.tasks[task.Spec.Name] = task
	s.scheduleTask(task)
}

// AddNode 添加一个可用节点，并尝试调度 pending 任务
func (s *Scheduler) AddNode(node *Node) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.nodes[node.Spec.Name] = node
	s.trySchedulePending()
}

// RemoveNode 移除一个节点，并重新调度其上的任务
func (s *Scheduler) RemoveNode(name string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	node, ok := s.nodes[name]
	if !ok {
		return
	}
	delete(s.nodes, name)

	// 将该节点上的任务重置为 pending
	for _, task := range s.tasks {
		if task.Status.NodeName == node.Spec.Name {
			task.Status.NodeName = ""
			task.Status.Phase = "pending"
			task.Status.Message = "node removed, rescheduling"
		}
	}

	// 尝试重新调度
	s.trySchedulePending()
}

// DeleteTask 删除一个任务并释放资源
func (s *Scheduler) DeleteTask(name string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	task, ok := s.tasks[name]
	if !ok {
		return
	}

	delete(s.tasks, name)
	if task.Status.NodeName != "" {
		s.releaseResources(task.Status.NodeName, task.Spec.CPURequest, task.Spec.MemRequest)
	}

	s.trySchedulePending()
}

// GetTask 查询任务状态
func (s *Scheduler) GetTask(name string) (*Task, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	task, ok := s.tasks[name]
	if !ok {
		return nil, fmt.Errorf("task %s not found", name)
	}
	return task, nil
}

// ListTasks 列出所有任务
func (s *Scheduler) ListTasks() []*Task {
	s.mu.Lock()
	defer s.mu.Unlock()
	result := make([]*Task, 0, len(s.tasks))
	for _, t := range s.tasks {
		result = append(result, t)
	}
	return result
}

// ListNodes 列出所有节点
func (s *Scheduler) ListNodes() []*Node {
	s.mu.Lock()
	defer s.mu.Unlock()
	result := make([]*Node, 0, len(s.nodes))
	for _, n := range s.nodes {
		result = append(result, n)
	}
	return result
}

// === 调度核心 ===

// scheduleTask 对单个任务执行 Filter → Score → Bind
func (s *Scheduler) scheduleTask(task *Task) {
	// 阶段 1: Filter — 过滤不满足硬约束的节点
	candidates := s.filter(task)
	if len(candidates) == 0 {
		task.Status.Phase = "failed"
		task.Status.Message = "no suitable node found"
		return
	}

	// 阶段 2: Score — 按策略排序候选节点
	s.score(task, candidates)

	// 阶段 3: Bind — 选择最优节点并绑定
	chosen := candidates[0]
	task.Status.NodeName = chosen.Spec.Name
	task.Status.Phase = "scheduled"
	task.Status.Message = ""

	chosen.Status.CPUUsed += task.Spec.CPURequest
	chosen.Status.MemUsed += task.Spec.MemRequest
	chosen.Status.TaskCount++
}

// filter 过滤不满足硬约束的节点
func (s *Scheduler) filter(task *Task) []*Node {
	var candidates []*Node

	for _, node := range s.nodes {
		remainingCPU := node.Spec.CPUCap - node.Status.CPUUsed
		remainingMem := node.Spec.MemCap - node.Status.MemUsed
		if remainingCPU < task.Spec.CPURequest || remainingMem < task.Spec.MemRequest {
			continue
		}

		if !matchSelector(node.Spec.Labels, task.Spec.NodeSelector) {
			continue
		}

		candidates = append(candidates, node)
	}

	return candidates
}

// score 根据调度策略对候选节点排序
func (s *Scheduler) score(task *Task, candidates []*Node) {
	policy := task.Spec.Policy
	if policy == "" {
		policy = PolicySpread
	}

	switch policy {
	case PolicyBinPack:
		sort.Slice(candidates, func(i, j int) bool {
			return usageRatio(candidates[i]) > usageRatio(candidates[j])
		})
	case PolicySpread:
		sort.Slice(candidates, func(i, j int) bool {
			return usageRatio(candidates[i]) < usageRatio(candidates[j])
		})
	}
}

func usageRatio(node *Node) float64 {
	cpuRatio := float64(node.Status.CPUUsed) / float64(node.Spec.CPUCap)
	memRatio := float64(node.Status.MemUsed) / float64(node.Spec.MemCap)
	return (cpuRatio + memRatio) / 2
}

func matchSelector(nodeLabels, selector map[string]string) bool {
	for k, v := range selector {
		if nodeLabels[k] != v {
			return false
		}
	}
	return true
}

func (s *Scheduler) releaseResources(nodeName string, cpu int, mem int64) {
	node, ok := s.nodes[nodeName]
	if !ok {
		return
	}
	node.Status.CPUUsed -= cpu
	if node.Status.CPUUsed < 0 {
		node.Status.CPUUsed = 0
	}
	node.Status.MemUsed -= mem
	if node.Status.MemUsed < 0 {
		node.Status.MemUsed = 0
	}
	node.Status.TaskCount--
	if node.Status.TaskCount < 0 {
		node.Status.TaskCount = 0
	}
}

func (s *Scheduler) trySchedulePending() {
	for _, task := range s.tasks {
		if task.Status.Phase == "pending" || task.Status.Phase == "failed" {
			s.scheduleTask(task)
		}
	}
}
