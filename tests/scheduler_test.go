package runtime_test

import (
	"testing"

	sch "github.com/cloudruntime/mini/src/scheduler"
)

func makeNode(name string, cpu int, mem int64, labels map[string]string) *sch.Node {
	return &sch.Node{
		Spec: sch.NodeSpec{
			Name:   name,
			CPUCap: cpu,
			MemCap: mem,
			Labels: labels,
		},
		Status: sch.NodeStatus{Name: name},
	}
}

func makeTask(name string, cpu int, mem int64, policy sch.SchedulingPolicy) *sch.Task {
	return &sch.Task{
		Spec: sch.TaskSpec{
			Name:       name,
			CPURequest: cpu,
			MemRequest: mem,
			Policy:     policy,
		},
		Status: sch.TaskStatus{Name: name},
	}
}

func TestScheduler_Spread(t *testing.T) {
	s := sch.NewScheduler()

	s.AddNode(makeNode("node-a", 4000, 8589934592, nil))
	s.AddNode(makeNode("node-b", 4000, 8589934592, nil))
	s.AddNode(makeNode("node-c", 4000, 8589934592, nil))

	s.SubmitTask(makeTask("task-1", 500, 1073741824, sch.PolicySpread))
	s.SubmitTask(makeTask("task-2", 500, 1073741824, sch.PolicySpread))
	s.SubmitTask(makeTask("task-3", 500, 1073741824, sch.PolicySpread))

	nodes := make(map[string]int)
	for _, task := range s.ListTasks() {
		if task.Status.Phase != "scheduled" {
			t.Fatalf("task %s not scheduled, phase: %s", task.Spec.Name, task.Status.Phase)
		}
		nodes[task.Status.NodeName]++
	}

	if len(nodes) != 3 {
		t.Fatalf("spread: expected 3 distinct nodes, got %d: %v", len(nodes), nodes)
	}
}

func TestScheduler_BinPack(t *testing.T) {
	s := sch.NewScheduler()

	s.AddNode(makeNode("node-a", 4000, 8589934592, nil))
	s.AddNode(makeNode("node-b", 4000, 8589934592, nil))
	s.AddNode(makeNode("node-c", 4000, 8589934592, nil))

	s.SubmitTask(makeTask("task-1", 500, 1073741824, sch.PolicyBinPack))
	s.SubmitTask(makeTask("task-2", 500, 1073741824, sch.PolicyBinPack))
	s.SubmitTask(makeTask("task-3", 500, 1073741824, sch.PolicyBinPack))

	nodes := make(map[string]int)
	for _, task := range s.ListTasks() {
		if task.Status.Phase != "scheduled" {
			t.Fatalf("task %s not scheduled, phase: %s", task.Spec.Name, task.Status.Phase)
		}
		nodes[task.Status.NodeName]++
	}

	if len(nodes) > 2 {
		t.Fatalf("binpack: expected tasks concentrated on <=2 nodes, got %d: %v", len(nodes), nodes)
	}
}

func TestScheduler_NoResources(t *testing.T) {
	s := sch.NewScheduler()

	s.AddNode(makeNode("node-a", 100, 1073741824, nil))

	s.SubmitTask(makeTask("task-big", 500, 1073741824, sch.PolicySpread))

	task, err := s.GetTask("task-big")
	if err != nil {
		t.Fatal(err)
	}
	if task.Status.Phase != "failed" {
		t.Fatalf("expected failed, got %s", task.Status.Phase)
	}
}

func TestScheduler_NodeSelector(t *testing.T) {
	s := sch.NewScheduler()

	s.AddNode(makeNode("node-a", 4000, 8589934592, map[string]string{"zone": "east"}))
	s.AddNode(makeNode("node-b", 4000, 8589934592, map[string]string{"zone": "west"}))

	task := &sch.Task{
		Spec: sch.TaskSpec{
			Name:         "task-1",
			CPURequest:   500,
			MemRequest:   1073741824,
			Policy:       sch.PolicySpread,
			NodeSelector: map[string]string{"zone": "east"},
		},
		Status: sch.TaskStatus{Name: "task-1"},
	}
	s.SubmitTask(task)

	result, _ := s.GetTask("task-1")
	if result.Status.NodeName != "node-a" {
		t.Fatalf("expected node-a (zone=east), got %s", result.Status.NodeName)
	}
}

func TestScheduler_NodeRemoved_Reschedule(t *testing.T) {
	s := sch.NewScheduler()

	s.AddNode(makeNode("node-a", 4000, 8589934592, nil))
	s.AddNode(makeNode("node-b", 4000, 8589934592, nil))

	s.SubmitTask(makeTask("task-1", 500, 1073741824, sch.PolicySpread))

	task1, _ := s.GetTask("task-1")
	originalNode := task1.Status.NodeName
	s.RemoveNode(originalNode)

	task1, _ = s.GetTask("task-1")
	if task1.Status.Phase != "scheduled" {
		t.Fatalf("expected scheduled after reschedule, got %s (msg: %s)", task1.Status.Phase, task1.Status.Message)
	}
	if task1.Status.NodeName == originalNode {
		t.Fatal("expected different node after reschedule")
	}
}

func TestScheduler_DeleteTask_ReleaseResources(t *testing.T) {
	s := sch.NewScheduler()

	s.AddNode(makeNode("node-a", 1000, 2147483648, nil))

	s.SubmitTask(makeTask("task-1", 500, 1073741824, sch.PolicySpread))
	s.SubmitTask(makeTask("task-2", 500, 1073741824, sch.PolicySpread))

	s.DeleteTask("task-1")

	s.SubmitTask(makeTask("task-3", 500, 1073741824, sch.PolicySpread))

	task3, _ := s.GetTask("task-3")
	if task3.Status.Phase != "scheduled" {
		t.Fatalf("expected task-3 scheduled after resource release, got %s (msg: %s)", task3.Status.Phase, task3.Status.Message)
	}
}

func TestScheduler_NewNode_TriggersPendingSchedule(t *testing.T) {
	s := sch.NewScheduler()

	s.SubmitTask(makeTask("task-1", 500, 1073741824, sch.PolicySpread))

	task1, _ := s.GetTask("task-1")
	if task1.Status.Phase == "scheduled" {
		t.Fatal("expected task to not be scheduled without nodes")
	}

	s.AddNode(makeNode("node-a", 4000, 8589934592, nil))

	task1, _ = s.GetTask("task-1")
	if task1.Status.Phase != "scheduled" {
		t.Fatalf("expected scheduled after node added, got %s (msg: %s)", task1.Status.Phase, task1.Status.Message)
	}
}
