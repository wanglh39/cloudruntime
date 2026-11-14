package runtime_test

import (
	"runtime"
	"testing"

	rt "github.com/cloudruntime/mini/src/runtime"
)

func TestSandboxManager_Create_NonLinux(t *testing.T) {
	if runtime.GOOS == "linux" {
		t.Skip("this test only runs on non-Linux")
	}

	mgr := rt.NewSandboxManager()
	_, err := mgr.Create(rt.SandboxSpec{
		Name:    "test-sandbox",
		Command: []string{"/bin/sh"},
	})

	if err == nil {
		t.Fatal("expected error on non-Linux, got nil")
	}
}

func TestSandboxManager_DuplicateName(t *testing.T) {
	mgr := rt.NewSandboxManager()
	mgr.Create(rt.SandboxSpec{
		Name:    "existing",
		Command: []string{"/bin/sh"},
	})

	_, err := mgr.Create(rt.SandboxSpec{
		Name:    "existing",
		Command: []string{"/bin/sh"},
	})

	if err == nil {
		t.Fatal("expected error for duplicate name, got nil")
	}
}

func TestSandboxManager_Stop_NotFound(t *testing.T) {
	mgr := rt.NewSandboxManager()
	err := mgr.Stop("nonexistent")
	if err == nil {
		t.Fatal("expected error for nonexistent sandbox, got nil")
	}
}

func TestSandboxManager_Delete_NotFound(t *testing.T) {
	mgr := rt.NewSandboxManager()
	err := mgr.Delete("nonexistent")
	if err == nil {
		t.Fatal("expected error for nonexistent sandbox, got nil")
	}
}

func TestSandboxManager_Get_NotFound(t *testing.T) {
	mgr := rt.NewSandboxManager()
	_, err := mgr.Get("nonexistent")
	if err == nil {
		t.Fatal("expected error for nonexistent sandbox, got nil")
	}
}

func TestSandboxManager_List_Empty(t *testing.T) {
	mgr := rt.NewSandboxManager()
	list := mgr.List()
	if len(list) != 0 {
		t.Fatalf("expected empty list, got %d items", len(list))
	}
}

func TestSandboxSpec_Fields(t *testing.T) {
	spec := rt.SandboxSpec{
		Name:     "test",
		Command:  []string{"/bin/echo", "hello"},
		CPULimit: 500,
		MemLimit: 536870912,
	}
	if spec.Name != "test" {
		t.Fatalf("expected name 'test', got '%s'", spec.Name)
	}
	if len(spec.Command) != 2 {
		t.Fatalf("expected 2 command args, got %d", len(spec.Command))
	}
	if spec.CPULimit != 500 {
		t.Fatalf("expected CPULimit 500, got %d", spec.CPULimit)
	}
}
