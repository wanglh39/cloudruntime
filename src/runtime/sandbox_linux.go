//go:build linux
// +build linux

package runtime

import (
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"syscall"
)

const (
	cgroupRoot = "/sys/fs/cgroup/cloudruntime"

	defaultCloneFlags = syscall.CLONE_NEWPID |
		syscall.CLONE_NEWNET |
		syscall.CLONE_NEWNS |
		syscall.CLONE_NEWUTS |
		syscall.CLONE_NEWIPC |
		syscall.CLONE_NEWUSER
)

func createSandboxProcess(spec SandboxSpec) (*exec.Cmd, error) {
	cmd := exec.Command(spec.Command[0], spec.Command[1:]...)
	cmd.Env = append(os.Environ(), spec.Env...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	cmd.SysProcAttr = &syscall.SysProcAttr{
		Cloneflags: defaultCloneFlags,
		UidMappings: []syscall.SysProcIDMap{
			{ContainerID: 0, HostID: os.Getuid(), Size: 1},
		},
		GidMappings: []syscall.SysProcIDMap{
			{ContainerID: 0, HostID: os.Getgid(), Size: 1},
		},
	}

	if err := cmd.Start(); err != nil {
		return nil, err
	}
	return cmd, nil
}

func setupCgroup(spec SandboxSpec) error {
	cgroupPath := filepath.Join(cgroupRoot, spec.Name)
	if err := os.MkdirAll(cgroupPath, 0755); err != nil {
		return err
	}

	if spec.CPULimit > 0 {
		quota := spec.CPULimit * 100
		cpuMax := strconv.Itoa(quota) + " 100000"
		if err := os.WriteFile(filepath.Join(cgroupPath, "cpu.max"), []byte(cpuMax), 0644); err != nil {
			return err
		}
	}

	if spec.MemLimit > 0 {
		memMax := strconv.FormatInt(spec.MemLimit, 10)
		if err := os.WriteFile(filepath.Join(cgroupPath, "memory.max"), []byte(memMax), 0644); err != nil {
			return err
		}
	}

	if err := os.WriteFile(filepath.Join(cgroupPath, "pids.max"), []byte("100"), 0644); err != nil {
		return err
	}

	return nil
}

func addToCgroup(name string, pid int) error {
	cgroupPath := filepath.Join(cgroupRoot, name, "cgroup.procs")
	return os.WriteFile(cgroupPath, []byte(strconv.Itoa(pid)), 0644)
}

func cleanupCgroup(name string) error {
	cgroupPath := filepath.Join(cgroupRoot, name)
	return os.RemoveAll(cgroupPath)
}

func killProcess(pid int, sig syscall.Signal) error {
	return syscall.Kill(pid, sig)
}

func checkCgroupV2() bool {
	data, err := os.ReadFile("/sys/fs/cgroup/cgroup.controllers")
	if err != nil {
		return false
	}
	return len(data) > 0
}
