package faas

import (
	"fmt"
	"sync"
	"time"
)

// FaaSEngine 函数运行时引擎
type FaaSEngine struct {
	mu        sync.Mutex
	functions map[string]*Function
	instances map[string]*Instance       // instance ID -> Instance
	funcInsts map[string][]*Instance     // function name -> instances
	warmPool  map[string]*WarmPoolConfig // function name -> warm pool config
	eventCh   chan *Event
	stopCh    chan struct{}
	idCounter int
}

func NewFaaSEngine() *FaaSEngine {
	return &FaaSEngine{
		functions: make(map[string]*Function),
		instances: make(map[string]*Instance),
		funcInsts: make(map[string][]*Instance),
		warmPool:  make(map[string]*WarmPoolConfig),
		eventCh:   make(chan *Event, 100),
		stopCh:    make(chan struct{}),
	}
}

// === 函数注册 ===

// Register 注册一个函数
func (e *FaaSEngine) Register(fn *Function) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	if _, exists := e.functions[fn.Name]; exists {
		return fmt.Errorf("function %s already registered", fn.Name)
	}

	e.functions[fn.Name] = fn
	e.funcInsts[fn.Name] = make([]*Instance, 0)

	// 设置默认预热池配置
	if fn.MinInstances > 0 {
		e.warmPool[fn.Name] = &WarmPoolConfig{
			MinReady:       fn.MinInstances,
			MaxIdle:        5 * time.Minute,
			ReplenishDelay: 100 * time.Millisecond,
		}
		// 预热：创建初始实例
		for i := 0; i < fn.MinInstances; i++ {
			e.createInstance(fn.Name)
		}
	}

	return nil
}

// Unregister 注销一个函数
func (e *FaaSEngine) Unregister(name string) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	if _, exists := e.functions[name]; !exists {
		return fmt.Errorf("function %s not found", name)
	}

	// 停止所有实例
	for _, inst := range e.funcInsts[name] {
		inst.Status = InstanceStopped
		delete(e.instances, inst.ID)
	}

	delete(e.functions, name)
	delete(e.funcInsts, name)
	delete(e.warmPool, name)
	return nil
}

// GetFunction 查询函数定义
func (e *FaaSEngine) GetFunction(name string) (*Function, error) {
	e.mu.Lock()
	defer e.mu.Unlock()

	fn, ok := e.functions[name]
	if !ok {
		return nil, fmt.Errorf("function %s not found", name)
	}
	return fn, nil
}

// ListFunctions 列出所有函数
func (e *FaaSEngine) ListFunctions() []*Function {
	e.mu.Lock()
	defer e.mu.Unlock()

	result := make([]*Function, 0, len(e.functions))
	for _, fn := range e.functions {
		result = append(result, fn)
	}
	return result
}

// === 实例管理 ===

// createInstance 创建一个函数实例（模拟冷启动）
func (e *FaaSEngine) createInstance(functionName string) *Instance {
	e.idCounter++
	id := fmt.Sprintf("inst-%d", e.idCounter)

	inst := &Instance{
		ID:           id,
		FunctionName: functionName,
		Status:       InstanceCreating,
		CreatedAt:    time.Now(),
		LastUsedAt:   time.Now(),
	}

	e.instances[id] = inst
	e.funcInsts[functionName] = append(e.funcInsts[functionName], inst)

	// 模拟冷启动延迟：creating → ready
	// 教学版中我们直接标记为 ready，但在 Invoke 中记录冷启动
	inst.Status = InstanceReady

	return inst
}

// getReadyInstance 获取一个就绪的实例
// 如果没有就绪实例，创建新实例（冷启动）
func (e *FaaSEngine) getReadyInstance(functionName string) (*Instance, bool) {
	insts := e.funcInsts[functionName]

	// 优先找 ready 状态的实例
	for _, inst := range insts {
		if inst.Status == InstanceReady {
			return inst, false // 热启动
		}
	}

	// 没有 ready 实例，创建新实例（冷启动）
	inst := e.createInstance(functionName)
	return inst, true // 冷启动
}

// releaseInstance 释放实例（执行完毕后回到 ready）
func (e *FaaSEngine) releaseInstance(inst *Instance) {
	inst.Status = InstanceReady
	inst.LastUsedAt = time.Now()
	inst.RequestCount++
}

// ListInstances 列出所有实例
func (e *FaaSEngine) ListInstances() []*Instance {
	e.mu.Lock()
	defer e.mu.Unlock()

	result := make([]*Instance, 0, len(e.instances))
	for _, inst := range e.instances {
		result = append(result, inst)
	}
	return result
}

// ListInstancesByFunction 列出某函数的所有实例
func (e *FaaSEngine) ListInstancesByFunction(functionName string) []*Instance {
	e.mu.Lock()
	defer e.mu.Unlock()

	return e.funcInsts[functionName]
}

// === 函数调用 ===

// Invoke 调用一个函数
// 这是 FaaS 引擎的核心方法：事件 → 路由 → 实例分配 → 执行 → 返回
func (e *FaaSEngine) Invoke(eventName string, payload []byte) (*InvokeResult, error) {
	e.mu.Lock()

	fn, ok := e.functions[eventName]
	if !ok {
		e.mu.Unlock()
		return nil, fmt.Errorf("function %s not found", eventName)
	}

	// 从预热池或创建新实例
	inst, coldStart := e.getReadyInstance(eventName)
	inst.Status = InstanceBusy
	e.mu.Unlock()

	// 模拟执行延迟
	startTime := time.Now()

	// 冷启动额外延迟（模拟沙箱创建 + 代码加载 + 运行时初始化）
	if coldStart {
		time.Sleep(150 * time.Millisecond) // 模拟冷启动
	}
	time.Sleep(10 * time.Millisecond) // 模拟函数执行

	duration := time.Since(startTime)

	e.mu.Lock()
	e.releaseInstance(inst)

	// 检查预热池是否需要补充
	e.replenishWarmPool(eventName)
	e.mu.Unlock()

	result := &InvokeResult{
		Output:    []byte(fmt.Sprintf("function %s executed successfully (handler: %s)", fn.Name, fn.Handler)),
		Duration:  duration,
		ColdStart: coldStart,
	}

	return result, nil
}

// replenishWarmPool 检查并补充预热池
func (e *FaaSEngine) replenishWarmPool(functionName string) {
	config, ok := e.warmPool[functionName]
	if !ok {
		return
	}

	insts := e.funcInsts[functionName]
	readyCount := 0
	for _, inst := range insts {
		if inst.Status == InstanceReady {
			readyCount++
		}
	}

	// 如果就绪实例数低于配置的最小值，创建新实例补充
	for i := readyCount; i < config.MinReady; i++ {
		e.createInstance(functionName)
	}
}

// === 事件路由 ===

// SubmitEvent 提交一个事件
func (e *FaaSEngine) SubmitEvent(event *Event) {
	e.eventCh <- event
}

// RunEventLoop 启动事件循环
func (e *FaaSEngine) RunEventLoop() {
	for {
		select {
		case event := <-e.eventCh:
			_, _ = e.Invoke(event.FunctionName, event.Payload)
		case <-e.stopCh:
			return
		}
	}
}

// Stop 停止事件循环
func (e *FaaSEngine) Stop() {
	close(e.stopCh)
}

// === 预热池管理 ===

// SetWarmPoolConfig 设置预热池配置
func (e *FaaSEngine) SetWarmPoolConfig(functionName string, config WarmPoolConfig) {
	e.mu.Lock()
	defer e.mu.Unlock()

	e.warmPool[functionName] = &config

	// 立即补充到 MinReady
	for i := 0; i < config.MinReady; i++ {
		insts := e.funcInsts[functionName]
		readyCount := 0
		for _, inst := range insts {
			if inst.Status == InstanceReady {
				readyCount++
			}
		}
		if readyCount < config.MinReady {
			e.createInstance(functionName)
		}
	}
}

// GetWarmPoolStatus 获取预热池状态
func (e *FaaSEngine) GetWarmPoolStatus(functionName string) (readyCount int, totalCount int) {
	e.mu.Lock()
	defer e.mu.Unlock()

	insts := e.funcInsts[functionName]
	for _, inst := range insts {
		totalCount++
		if inst.Status == InstanceReady {
			readyCount++
		}
	}
	return
}

// ReclaimIdle 回收空闲实例
// 超过 MaxIdle 时间未使用的实例被回收
func (e *FaaSEngine) ReclaimIdle() int {
	e.mu.Lock()
	defer e.mu.Unlock()

	reclaimed := 0
	now := time.Now()

	for funcName, insts := range e.funcInsts {
		config, hasConfig := e.warmPool[funcName]
		minKeep := 0
		if hasConfig {
			minKeep = config.MinReady
		}

		readyInsts := make([]*Instance, 0)
		for _, inst := range insts {
			if inst.Status == InstanceReady {
				readyInsts = append(readyInsts, inst)
			}
		}

		// 如果就绪实例数 > 最小保留数，回收多余的空闲实例
		if len(readyInsts) > minKeep {
			for _, inst := range readyInsts[minKeep:] {
				idleDuration := now.Sub(inst.LastUsedAt)
				maxIdle := 5 * time.Minute
				if hasConfig {
					maxIdle = config.MaxIdle
				}
				if idleDuration > maxIdle {
					inst.Status = InstanceStopped
					delete(e.instances, inst.ID)
					reclaimed++
				}
			}
		}

		// 重建实例列表（移除已停止的）
		newInsts := make([]*Instance, 0)
		for _, inst := range insts {
			if inst.Status != InstanceStopped {
				newInsts = append(newInsts, inst)
			}
		}
		e.funcInsts[funcName] = newInsts
	}

	return reclaimed
}
