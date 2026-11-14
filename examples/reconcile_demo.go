package main

// 模块 0 入门示例：Reconcile Loop
// 这个示例展示了 Cloud Runtime 最核心的设计哲学——
// 声明式 API + 控制器循环 + 最终一致性
//
// 运行方式: go run examples/reconcile_demo.go

import (
	"fmt"
	"time"
)

// DesiredState 是用户声明的期望状态
type DesiredState struct {
	Replicas int
}

// ActualState 是系统观察到的实际状态
type ActualState struct {
	RunningReplicas int
}

// Reconciler 是控制器循环的核心
// 它持续比较期望状态和实际状态，驱动收敛
type Reconciler struct {
	desired DesiredState
	actual  ActualState
}

func NewReconciler(desired DesiredState) *Reconciler {
	return &Reconciler{
		desired: desired,
		actual:  ActualState{RunningReplicas: 0},
	}
}

// Reconcile 执行一次收敛
// 返回是否已经达到期望状态
func (r *Reconciler) Reconcile() bool {
	fmt.Printf("  期望: %d 实例, 实际: %d 实例", r.desired.Replicas, r.actual.RunningReplicas)

	if r.actual.RunningReplicas < r.desired.Replicas {
		// 需要扩容：启动新实例
		r.actual.RunningReplicas++
		fmt.Printf(" → 启动实例 #%d\n", r.actual.RunningReplicas)
		return false
	} else if r.actual.RunningReplicas > r.desired.Replicas {
		// 需要缩容：停止多余实例
		fmt.Printf(" → 停止实例 #%d\n", r.actual.RunningReplicas)
		r.actual.RunningReplicas--
		return false
	}

	fmt.Printf(" → 已收敛 ✓\n")
	return true
}

func main() {
	fmt.Println("=== Reconcile Loop 演示 ===")
	fmt.Println("核心思想: 声明期望状态 → 观察实际状态 → 驱动收敛")
	fmt.Println()

	// 场景 1：从 0 扩容到 3
	fmt.Println("场景 1: 用户声明需要 3 个实例")
	reconciler := NewReconciler(DesiredState{Replicas: 3})
	for i := 0; i < 10; i++ {
		fmt.Printf("第 %d 轮: ", i+1)
		if reconciler.Reconcile() {
			break
		}
		time.Sleep(500 * time.Millisecond)
	}
	fmt.Println()

	// 场景 2：从 3 缩容到 1
	fmt.Println("场景 2: 用户修改声明，只需要 1 个实例")
	reconciler.desired.Replicas = 1
	for i := 0; i < 10; i++ {
		fmt.Printf("第 %d 轮: ", i+1)
		if reconciler.Reconcile() {
			break
		}
		time.Sleep(500 * time.Millisecond)
	}
	fmt.Println()

	// 场景 3：模拟故障 — 一个实例意外消失
	fmt.Println("场景 3: 一个实例意外消失（模拟故障）")
	reconciler.actual.RunningReplicas = 0
	fmt.Printf("  故障发生: 实际实例数变为 %d\n", reconciler.actual.RunningReplicas)
	for i := 0; i < 10; i++ {
		fmt.Printf("第 %d 轮: ", i+1)
		if reconciler.Reconcile() {
			break
		}
		time.Sleep(500 * time.Millisecond)
	}
	fmt.Println()

	fmt.Println("=== 关键洞察 ===")
	fmt.Println("1. 用户只声明'要什么'(3个实例)，不关心'怎么做'")
	fmt.Println("2. 系统自动从任何偏差状态收敛到期望状态")
	fmt.Println("3. 故障自愈是 reconcile loop 的自然产物，不是额外功能")
	fmt.Println("4. 这就是 Kubernetes 的核心设计哲学")
}
