//go:build !linux
// +build !linux

package runtime

import (
	"fmt"
	"os/exec"
	"runtime"
)

const (
	cgroupRoot = ""
)

func createSandboxProcess(spec SandboxSpec) (*exec.Cmd, error) {
	return nil, fmt.Errorf("sandbox requires Linux (namespace/cgroups), current OS: %s; use WSL2 or a Linux VM", runtime.GOOS)
}

func setupCgroup(spec SandboxSpec) error {
	return fmt.Errorf("cgroups not available on %s", runtime.GOOS)
}

func addToCgroup(name string, pid int) error {
	return fmt.Errorf("cgroups not available")
}

func cleanupCgroup(name string) error {
	return nil
}

func killProcess(pid int, sig int) error {
	return fmt.Errorf("kill not available on %s", runtime.GOOS)
}

func checkCgroupV2() bool {
	return false
}
