package runtime

import (
	"fmt"
	"sync"
	"time"
)

// SandboxManager 管理所有沙箱的生命周期
type SandboxManager struct {
	mu        sync.Mutex
	sandboxes map[string]*Sandbox
}

func NewSandboxManager() *SandboxManager {
	return &SandboxManager{
		sandboxes: make(map[string]*Sandbox),
	}
}

// Create 创建并启动一个新沙箱
func (m *SandboxManager) Create(spec SandboxSpec) (*Sandbox, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, exists := m.sandboxes[spec.Name]; exists {
		return nil, fmt.Errorf("sandbox %s already exists", spec.Name)
	}

	if err := setupCgroup(spec); err != nil {
		return nil, fmt.Errorf("cgroup setup failed: %w", err)
	}

	cmd, err := createSandboxProcess(spec)
	if err != nil {
		_ = cleanupCgroup(spec.Name)
		return nil, fmt.Errorf("failed to start sandbox: %w", err)
	}

	if err := addToCgroup(spec.Name, cmd.Process.Pid); err != nil {
		_ = cmd.Process.Kill()
		_ = cleanupCgroup(spec.Name)
		return nil, fmt.Errorf("failed to add to cgroup: %w", err)
	}

	sandbox := &Sandbox{
		Spec: spec,
		Status: SandboxStatus{
			Name:  spec.Name,
			State: "running",
			PID:   cmd.Process.Pid,
		},
	}
	m.sandboxes[spec.Name] = sandbox

	go func() {
		_ = cmd.Wait()
		m.mu.Lock()
		if sb, ok := m.sandboxes[spec.Name]; ok {
			sb.Status.State = "stopped"
		}
		m.mu.Unlock()
	}()

	return sandbox, nil
}

// Stop 停止一个沙箱
func (m *SandboxManager) Stop(name string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	sb, ok := m.sandboxes[name]
	if !ok {
		return fmt.Errorf("sandbox %s not found", name)
	}

	if sb.Status.State != "running" {
		return fmt.Errorf("sandbox %s is not running (state: %s)", name, sb.Status.State)
	}

	_ = killProcess(sb.Status.PID, 15) // SIGTERM

	done := make(chan struct{})
	go func() {
		for i := 0; i < 50; i++ {
			m.mu.Lock()
			state := sb.Status.State
			m.mu.Unlock()
			if state != "running" {
				close(done)
				return
			}
			time.Sleep(100 * time.Millisecond)
		}
		_ = killProcess(sb.Status.PID, 9) // SIGKILL
		close(done)
	}()
	<-done

	sb.Status.State = "stopped"
	return nil
}

// Delete 删除一个沙箱
func (m *SandboxManager) Delete(name string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	sb, ok := m.sandboxes[name]
	if !ok {
		return fmt.Errorf("sandbox %s not found", name)
	}

	if sb.Status.State == "running" {
		_ = killProcess(sb.Status.PID, 9) // SIGKILL
	}

	_ = cleanupCgroup(name)
	delete(m.sandboxes, name)
	return nil
}

// List 返回所有沙箱
func (m *SandboxManager) List() []*Sandbox {
	m.mu.Lock()
	defer m.mu.Unlock()

	result := make([]*Sandbox, 0, len(m.sandboxes))
	for _, sb := range m.sandboxes {
		result = append(result, sb)
	}
	return result
}

// Get 返回指定沙箱
func (m *SandboxManager) Get(name string) (*Sandbox, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	sb, ok := m.sandboxes[name]
	if !ok {
		return nil, fmt.Errorf("sandbox %s not found", name)
	}
	return sb, nil
}
