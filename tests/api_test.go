package runtime_test

import (
	"testing"
	"time"

	apipkg "github.com/cloudruntime/mini/src/api"
)

// MockReconciler 用于测试控制器循环
type MockReconciler struct {
	reconcileCount  int
	reconciledNames []string
}

func (m *MockReconciler) Reconcile(name string) error {
	m.reconcileCount++
	m.reconciledNames = append(m.reconciledNames, name)
	return nil
}

func TestResourceStore_Create(t *testing.T) {
	store := apipkg.NewResourceStore()

	obj, err := store.Create("sandboxes", "web", map[string]int{"cpu": 500})
	if err != nil {
		t.Fatal(err)
	}
	if obj.Meta.Name != "web" {
		t.Fatalf("expected name web, got %s", obj.Meta.Name)
	}
	if obj.Meta.ResourceVersion != 1 {
		t.Fatalf("expected version 1, got %d", obj.Meta.ResourceVersion)
	}
}

func TestResourceStore_Create_Duplicate(t *testing.T) {
	store := apipkg.NewResourceStore()

	store.Create("sandboxes", "web", nil)
	_, err := store.Create("sandboxes", "web", nil)
	if err == nil {
		t.Fatal("expected error for duplicate")
	}
}

func TestResourceStore_Get(t *testing.T) {
	store := apipkg.NewResourceStore()

	store.Create("sandboxes", "web", map[string]int{"cpu": 500})
	obj, err := store.Get("sandboxes", "web")
	if err != nil {
		t.Fatal(err)
	}
	if obj.Meta.Name != "web" {
		t.Fatalf("expected web, got %s", obj.Meta.Name)
	}
}

func TestResourceStore_Get_NotFound(t *testing.T) {
	store := apipkg.NewResourceStore()

	_, err := store.Get("sandboxes", "nonexistent")
	if err == nil {
		t.Fatal("expected error for not found")
	}
}

func TestResourceStore_List(t *testing.T) {
	store := apipkg.NewResourceStore()

	store.Create("sandboxes", "web", nil)
	store.Create("sandboxes", "db", nil)

	list := store.List("sandboxes")
	if len(list) != 2 {
		t.Fatalf("expected 2, got %d", len(list))
	}
}

func TestResourceStore_List_Empty(t *testing.T) {
	store := apipkg.NewResourceStore()

	list := store.List("sandboxes")
	if len(list) != 0 {
		t.Fatalf("expected 0, got %d", len(list))
	}
}

func TestResourceStore_Update(t *testing.T) {
	store := apipkg.NewResourceStore()

	store.Create("sandboxes", "web", map[string]int{"cpu": 500})
	obj, err := store.Update("sandboxes", "web", map[string]int{"cpu": 1000})
	if err != nil {
		t.Fatal(err)
	}

	if obj.Meta.ResourceVersion != 2 {
		t.Fatalf("expected version 2, got %d", obj.Meta.ResourceVersion)
	}
}

func TestResourceStore_UpdateStatus(t *testing.T) {
	store := apipkg.NewResourceStore()

	store.Create("sandboxes", "web", nil)
	err := store.UpdateStatus("sandboxes", "web", map[string]string{"phase": "running"})
	if err != nil {
		t.Fatal(err)
	}

	obj, _ := store.Get("sandboxes", "web")
	status, ok := obj.Status.(map[string]string)
	if !ok {
		t.Fatal("expected status to be map[string]string")
	}
	if status["phase"] != "running" {
		t.Fatalf("expected running, got %s", status["phase"])
	}
}

func TestResourceStore_Delete(t *testing.T) {
	store := apipkg.NewResourceStore()

	store.Create("sandboxes", "web", nil)
	err := store.Delete("sandboxes", "web")
	if err != nil {
		t.Fatal(err)
	}

	_, err = store.Get("sandboxes", "web")
	if err == nil {
		t.Fatal("expected error after delete")
	}
}

func TestResourceStore_Watch(t *testing.T) {
	store := apipkg.NewResourceStore()

	ch := store.Watch("sandboxes")

	// 创建资源，应该收到 ADDED 事件
	store.Create("sandboxes", "web", nil)

	select {
	case event := <-ch:
		if event.Type != apipkg.EventAdded {
			t.Fatalf("expected ADDED, got %s", event.Type)
		}
	case <-time.After(100 * time.Millisecond):
		t.Fatal("timeout waiting for ADDED event")
	}

	// 更新资源，应该收到 MODIFIED 事件
	store.Update("sandboxes", "web", map[string]int{"cpu": 1000})

	select {
	case event := <-ch:
		if event.Type != apipkg.EventModified {
			t.Fatalf("expected MODIFIED, got %s", event.Type)
		}
	case <-time.After(100 * time.Millisecond):
		t.Fatal("timeout waiting for MODIFIED event")
	}

	// 删除资源，应该收到 DELETED 事件
	store.Delete("sandboxes", "web")

	select {
	case event := <-ch:
		if event.Type != apipkg.EventDeleted {
			t.Fatalf("expected DELETED, got %s", event.Type)
		}
	case <-time.After(100 * time.Millisecond):
		t.Fatal("timeout waiting for DELETED event")
	}
}

func TestResourceStore_StopWatch(t *testing.T) {
	store := apipkg.NewResourceStore()

	ch := store.Watch("sandboxes")
	store.StopWatch("sandboxes", ch)

	// channel 应该被关闭
	_, ok := <-ch
	if ok {
		t.Fatal("expected channel to be closed")
	}
}

func TestControllerManager_Register(t *testing.T) {
	store := apipkg.NewResourceStore()
	cm := apipkg.NewControllerManager(store)

	reconciler := &MockReconciler{}
	cm.Register("sandboxes", reconciler)
}

func TestControllerManager_Reconcile(t *testing.T) {
	store := apipkg.NewResourceStore()
	cm := apipkg.NewControllerManager(store)

	reconciler := &MockReconciler{}
	cm.Register("sandboxes", reconciler)

	cm.Run()
	defer cm.Stop()

	// 创建资源，控制器应该收到事件并执行 Reconcile
	store.Create("sandboxes", "web", nil)

	time.Sleep(100 * time.Millisecond)

	if reconciler.reconcileCount == 0 {
		t.Fatal("expected reconcile to be called")
	}
}

func TestResourceStore_VersionIncrement(t *testing.T) {
	store := apipkg.NewResourceStore()

	obj1, _ := store.Create("sandboxes", "a", nil)
	obj2, _ := store.Create("sandboxes", "b", nil)

	if obj2.Meta.ResourceVersion <= obj1.Meta.ResourceVersion {
		t.Fatal("expected version to increment")
	}
}

func TestResourceStore_SpecStatusSeparation(t *testing.T) {
	store := apipkg.NewResourceStore()

	store.Create("sandboxes", "web", map[string]int{"cpu": 500})
	store.UpdateStatus("sandboxes", "web", map[string]string{"phase": "running"})

	obj, _ := store.Get("sandboxes", "web")

	// Spec 应该是用户设置的
	spec, ok := obj.Spec.(map[string]int)
	if !ok || spec["cpu"] != 500 {
		t.Fatal("spec should be user-set value")
	}

	// Status 应该是控制器设置的
	status, ok := obj.Status.(map[string]string)
	if !ok || status["phase"] != "running" {
		t.Fatal("status should be controller-set value")
	}
}
