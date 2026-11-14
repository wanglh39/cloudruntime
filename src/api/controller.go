package api

import (
	"sync"
	"time"
)

// ControllerManager 管理所有控制器
// 每个控制器通过 Watch 监听资源变化，执行 Reconcile 驱动数据面
type ControllerManager struct {
	mu          sync.Mutex
	controllers map[string]Reconciler // kind -> reconciler
	store       *ResourceStore
	stopCh      chan struct{}
}

func NewControllerManager(store *ResourceStore) *ControllerManager {
	return &ControllerManager{
		controllers: make(map[string]Reconciler),
		store:       store,
		stopCh:      make(chan struct{}),
	}
}

// Register 注册一个控制器
func (cm *ControllerManager) Register(kind string, reconciler Reconciler) {
	cm.mu.Lock()
	defer cm.mu.Unlock()
	cm.controllers[kind] = reconciler
}

// Run 启动控制器循环
// 每个控制器 Watch 自己负责的资源类型，收到事件后执行 Reconcile
func (cm *ControllerManager) Run() {
	cm.mu.Lock()
	kinds := make([]string, 0, len(cm.controllers))
	for kind := range cm.controllers {
		kinds = append(kinds, kind)
	}
	cm.mu.Unlock()

	for _, kind := range kinds {
		go cm.runController(kind)
	}
}

// runController 运行单个控制器的 Watch + Reconcile 循环
func (cm *ControllerManager) runController(kind string) {
	ch := cm.store.Watch(kind)
	defer cm.store.StopWatch(kind, ch)

	// 初始 Reconcile：处理 Watch 注册前已存在的资源
	cm.mu.Lock()
	reconciler, ok := cm.controllers[kind]
	cm.mu.Unlock()

	if ok {
		for _, obj := range cm.store.List(kind) {
			_ = reconciler.Reconcile(obj.Meta.Name)
		}
	}

	for {
		select {
		case event := <-ch:
			cm.mu.Lock()
			reconciler, ok := cm.controllers[kind]
			cm.mu.Unlock()

			if !ok {
				continue
			}

			obj, ok := event.Object.(*ResourceObject)
			if !ok {
				continue
			}

			_ = reconciler.Reconcile(obj.Meta.Name)

		case <-cm.stopCh:
			return
		}
	}
}

// Stop 停止所有控制器
func (cm *ControllerManager) Stop() {
	close(cm.stopCh)
}

// RunPeriodicReconcile 定期执行全量 Reconcile
// 这是一个兜底机制：即使 Watch 漏掉了某些事件，定期 Reconcile 也能发现并修正偏差
func (cm *ControllerManager) RunPeriodicReconcile(interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			cm.mu.Lock()
			controllers := make(map[string]Reconciler, len(cm.controllers))
			for k, v := range cm.controllers {
				controllers[k] = v
			}
			cm.mu.Unlock()

			for kind, reconciler := range controllers {
				resources := cm.store.List(kind)
				for _, obj := range resources {
					_ = reconciler.Reconcile(obj.Meta.Name)
				}
			}

		case <-cm.stopCh:
			return
		}
	}
}
