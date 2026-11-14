package runtime_test

import (
	"testing"
	"time"

	faaspkg "github.com/cloudruntime/mini/src/faas"
)

func makeFunction(name string, minInst int) *faaspkg.Function {
	return &faaspkg.Function{
		Name:         name,
		Runtime:      "python",
		Handler:      "handler",
		Code:         "/code/" + name,
		Timeout:      30,
		MemoryLimit:  536870912,
		MinInstances: minInst,
		MaxInstances: 10,
	}
}

func TestFaaSEngine_Register(t *testing.T) {
	engine := faaspkg.NewFaaSEngine()

	err := engine.Register(makeFunction("hello", 0))
	if err != nil {
		t.Fatal(err)
	}

	fn, err := engine.GetFunction("hello")
	if err != nil {
		t.Fatal(err)
	}
	if fn.Name != "hello" {
		t.Fatalf("expected hello, got %s", fn.Name)
	}
}

func TestFaaSEngine_Register_Duplicate(t *testing.T) {
	engine := faaspkg.NewFaaSEngine()

	engine.Register(makeFunction("hello", 0))
	err := engine.Register(makeFunction("hello", 0))
	if err == nil {
		t.Fatal("expected error for duplicate registration")
	}
}

func TestFaaSEngine_Invoke_ColdStart(t *testing.T) {
	engine := faaspkg.NewFaaSEngine()
	engine.Register(makeFunction("hello", 0))

	// 首次调用应该冷启动
	result, err := engine.Invoke("hello", []byte("test"))
	if err != nil {
		t.Fatal(err)
	}
	if !result.ColdStart {
		t.Fatal("expected cold start on first invoke")
	}
}

func TestFaaSEngine_Invoke_HotStart(t *testing.T) {
	engine := faaspkg.NewFaaSEngine()
	engine.Register(makeFunction("hello", 0))

	// 首次调用：冷启动
	engine.Invoke("hello", []byte("test1"))

	// 第二次调用：应该热启动（复用已有实例）
	result, err := engine.Invoke("hello", []byte("test2"))
	if err != nil {
		t.Fatal(err)
	}
	if result.ColdStart {
		t.Fatal("expected hot start on second invoke")
	}
}

func TestFaaSEngine_Invoke_NotFound(t *testing.T) {
	engine := faaspkg.NewFaaSEngine()

	_, err := engine.Invoke("nonexistent", nil)
	if err == nil {
		t.Fatal("expected error for nonexistent function")
	}
}

func TestFaaSEngine_WarmPool_Precreate(t *testing.T) {
	engine := faaspkg.NewFaaSEngine()

	// 注册函数时设置 MinInstances=3，应该预创建 3 个实例
	engine.Register(makeFunction("hello", 3))

	ready, total := engine.GetWarmPoolStatus("hello")
	if total < 3 {
		t.Fatalf("expected at least 3 instances, got %d", total)
	}
	if ready < 3 {
		t.Fatalf("expected at least 3 ready instances, got %d", ready)
	}
}

func TestFaaSEngine_WarmPool_FirstInvoke_HotStart(t *testing.T) {
	engine := faaspkg.NewFaaSEngine()

	// 预热池有 2 个就绪实例
	engine.Register(makeFunction("hello", 2))

	// 首次调用应该是热启动（因为预热池已有实例）
	result, err := engine.Invoke("hello", []byte("test"))
	if err != nil {
		t.Fatal(err)
	}
	if result.ColdStart {
		t.Fatal("expected hot start with warm pool")
	}
}

func TestFaaSEngine_WarmPool_Replenish(t *testing.T) {
	engine := faaspkg.NewFaaSEngine()
	engine.Register(makeFunction("hello", 2))

	// 初始有 2 个就绪实例
	ready, _ := engine.GetWarmPoolStatus("hello")
	if ready < 2 {
		t.Fatalf("expected 2 ready, got %d", ready)
	}

	// 调用一次，消耗一个实例，但调用结束后实例回到 ready
	// 预热池应该自动补充
	engine.Invoke("hello", []byte("test"))

	ready, _ = engine.GetWarmPoolStatus("hello")
	if ready < 2 {
		t.Fatalf("expected warm pool to replenish to 2, got %d", ready)
	}
}

func TestFaaSEngine_SetWarmPoolConfig(t *testing.T) {
	engine := faaspkg.NewFaaSEngine()
	engine.Register(makeFunction("hello", 0))

	// 初始没有预热池
	ready, _ := engine.GetWarmPoolStatus("hello")
	if ready != 0 {
		t.Fatalf("expected 0 ready, got %d", ready)
	}

	// 设置预热池配置
	engine.SetWarmPoolConfig("hello", faaspkg.WarmPoolConfig{
		MinReady:       3,
		MaxIdle:        5 * time.Minute,
		ReplenishDelay: 100 * time.Millisecond,
	})

	ready, _ = engine.GetWarmPoolStatus("hello")
	if ready < 3 {
		t.Fatalf("expected 3 ready after config, got %d", ready)
	}
}

func TestFaaSEngine_Unregister(t *testing.T) {
	engine := faaspkg.NewFaaSEngine()
	engine.Register(makeFunction("hello", 2))

	engine.Unregister("hello")

	_, err := engine.GetFunction("hello")
	if err == nil {
		t.Fatal("expected error after unregister")
	}
}

func TestFaaSEngine_ListFunctions(t *testing.T) {
	engine := faaspkg.NewFaaSEngine()
	engine.Register(makeFunction("hello", 0))
	engine.Register(makeFunction("world", 0))

	fns := engine.ListFunctions()
	if len(fns) != 2 {
		t.Fatalf("expected 2 functions, got %d", len(fns))
	}
}

func TestFaaSEngine_ListInstances(t *testing.T) {
	engine := faaspkg.NewFaaSEngine()
	engine.Register(makeFunction("hello", 3))

	insts := engine.ListInstances()
	if len(insts) < 3 {
		t.Fatalf("expected at least 3 instances, got %d", len(insts))
	}
}

func TestFaaSEngine_ReclaimIdle(t *testing.T) {
	engine := faaspkg.NewFaaSEngine()
	engine.Register(makeFunction("hello", 0))

	// 调用一次创建实例
	engine.Invoke("hello", []byte("test"))

	// 没有预热池配置，实例空闲后应该被回收
	// 但 MaxIdle 默认 5 分钟，所以立即回收不会有效果
	reclaimed := engine.ReclaimIdle()
	if reclaimed != 0 {
		t.Fatalf("expected 0 reclaimed (idle < MaxIdle), got %d", reclaimed)
	}
}

func TestFaaSEngine_InvokeResult(t *testing.T) {
	engine := faaspkg.NewFaaSEngine()
	engine.Register(makeFunction("hello", 0))

	result, err := engine.Invoke("hello", []byte("test"))
	if err != nil {
		t.Fatal(err)
	}

	if len(result.Output) == 0 {
		t.Fatal("expected non-empty output")
	}
	if result.Duration <= 0 {
		t.Fatal("expected positive duration")
	}
}
