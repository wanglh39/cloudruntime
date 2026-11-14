package storage

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
)

const (
	storageRoot = "/var/lib/cloudruntime"
	volumeRoot  = storageRoot + "/volumes"
	layerRoot   = storageRoot + "/layers"
	snapRoot    = storageRoot + "/snapshots"
)

// StorageManager 管理所有存储资源
type StorageManager struct {
	mu          sync.Mutex
	volumes     map[string]*Volume
	layers      map[string]*Layer
	filesystems map[string]*Filesystem
	snapshots   map[string]*Snapshot
	classes     map[string]*StorageClass
}

func NewStorageManager() *StorageManager {
	sm := &StorageManager{
		volumes:     make(map[string]*Volume),
		layers:      make(map[string]*Layer),
		filesystems: make(map[string]*Filesystem),
		snapshots:   make(map[string]*Snapshot),
		classes:     make(map[string]*StorageClass),
	}

	for _, sc := range DefaultStorageClasses() {
		sm.classes[sc.Name] = &sc
	}

	return sm
}

// === Volume 管理 ===

// CreateVolume 创建一个持久化卷（静态供应）
func (sm *StorageManager) CreateVolume(name string, sizeBytes int64, accessMode AccessMode, storageClass string) (*Volume, error) {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	if _, exists := sm.volumes[name]; exists {
		return nil, fmt.Errorf("volume %s already exists", name)
	}

	volPath := filepath.Join(volumeRoot, name)
	if err := os.MkdirAll(volPath, 0755); err != nil {
		return nil, fmt.Errorf("failed to create volume dir: %w", err)
	}

	vol := &Volume{
		Name:         name,
		SizeBytes:    sizeBytes,
		AccessMode:   accessMode,
		StorageClass: storageClass,
		Path:         volPath,
		Status:       VolumeAvailable,
	}

	sm.volumes[name] = vol
	return vol, nil
}

// CreateVolumeFromClaim 根据声明动态创建卷
func (sm *StorageManager) CreateVolumeFromClaim(claim VolumeClaim) (*Volume, error) {
	return sm.CreateVolume(claim.Name, claim.SizeBytes, claim.AccessMode, claim.StorageClass)
}

// DeleteVolume 删除一个卷
func (sm *StorageManager) DeleteVolume(name string) error {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	vol, ok := sm.volumes[name]
	if !ok {
		return fmt.Errorf("volume %s not found", name)
	}

	if vol.Status == VolumeBound {
		return fmt.Errorf("volume %s is bound to %s, cannot delete", name, vol.BoundTo)
	}

	_ = os.RemoveAll(vol.Path)
	delete(sm.volumes, name)
	return nil
}

// BindVolume 将卷绑定到沙箱
func (sm *StorageManager) BindVolume(volumeName, sandboxName string) error {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	vol, ok := sm.volumes[volumeName]
	if !ok {
		return fmt.Errorf("volume %s not found", volumeName)
	}

	if vol.Status == VolumeBound && vol.BoundTo != sandboxName {
		if vol.AccessMode == ReadWriteOnce {
			return fmt.Errorf("volume %s is RWO and already bound to %s", volumeName, vol.BoundTo)
		}
	}

	vol.Status = VolumeBound
	vol.BoundTo = sandboxName
	return nil
}

// UnbindVolume 解绑卷
func (sm *StorageManager) UnbindVolume(volumeName string) error {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	vol, ok := sm.volumes[volumeName]
	if !ok {
		return fmt.Errorf("volume %s not found", volumeName)
	}

	vol.Status = VolumeReleased
	vol.BoundTo = ""
	return nil
}

// GetVolume 查询卷信息
func (sm *StorageManager) GetVolume(name string) (*Volume, error) {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	vol, ok := sm.volumes[name]
	if !ok {
		return nil, fmt.Errorf("volume %s not found", name)
	}
	return vol, nil
}

// ListVolumes 列出所有卷
func (sm *StorageManager) ListVolumes() []*Volume {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	result := make([]*Volume, 0, len(sm.volumes))
	for _, v := range sm.volumes {
		result = append(result, v)
	}
	return result
}

// === 分层文件系统 ===

// CreateLayer 创建一个文件系统层
func (sm *StorageManager) CreateLayer(name, parent string) (*Layer, error) {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	if _, exists := sm.layers[name]; exists {
		return nil, fmt.Errorf("layer %s already exists", name)
	}

	layerPath := filepath.Join(layerRoot, name)
	if err := os.MkdirAll(layerPath, 0755); err != nil {
		return nil, fmt.Errorf("failed to create layer dir: %w", err)
	}

	layer := &Layer{
		Name:     name,
		Path:     layerPath,
		Parent:   parent,
		ReadOnly: true,
	}

	sm.layers[name] = layer
	return layer, nil
}

// CreateFilesystem 为沙箱创建分层文件系统
// 基于 overlayfs：lower=父层链, upper=读写层, work=工作目录, merged=合并视图
func (sm *StorageManager) CreateFilesystem(sandboxName string, layers []string) (*Filesystem, error) {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	if _, exists := sm.filesystems[sandboxName]; exists {
		return nil, fmt.Errorf("filesystem for %s already exists", sandboxName)
	}

	fsRoot := filepath.Join(storageRoot, "fs", sandboxName)
	upperDir := filepath.Join(fsRoot, "upper")
	workDir := filepath.Join(fsRoot, "work")
	mergedDir := filepath.Join(fsRoot, "merged")

	for _, dir := range []string{upperDir, workDir, mergedDir} {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return nil, fmt.Errorf("failed to create fs dir %s: %w", dir, err)
		}
	}

	var lowerDirs []string
	for _, layerName := range layers {
		layer, ok := sm.layers[layerName]
		if !ok {
			return nil, fmt.Errorf("layer %s not found", layerName)
		}
		lowerDirs = append(lowerDirs, layer.Path)
	}

	fs := &Filesystem{
		SandboxName: sandboxName,
		LowerDirs:   lowerDirs,
		UpperDir:    upperDir,
		WorkDir:     workDir,
		MergedDir:   mergedDir,
	}

	sm.filesystems[sandboxName] = fs
	return fs, nil
}

// DeleteFilesystem 删除沙箱的分层文件系统
func (sm *StorageManager) DeleteFilesystem(sandboxName string) error {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	fsRoot := filepath.Join(storageRoot, "fs", sandboxName)
	_ = os.RemoveAll(fsRoot)
	delete(sm.filesystems, sandboxName)
	return nil
}

// GetFilesystem 查询沙箱的文件系统
func (sm *StorageManager) GetFilesystem(sandboxName string) (*Filesystem, error) {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	fs, ok := sm.filesystems[sandboxName]
	if !ok {
		return nil, fmt.Errorf("filesystem for %s not found", sandboxName)
	}
	return fs, nil
}

// === 快照 ===

// CreateSnapshot 为卷创建快照
func (sm *StorageManager) CreateSnapshot(name, volumeName string) (*Snapshot, error) {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	vol, ok := sm.volumes[volumeName]
	if !ok {
		return nil, fmt.Errorf("volume %s not found", volumeName)
	}

	snapPath := filepath.Join(snapRoot, name)
	if err := os.MkdirAll(snapPath, 0755); err != nil {
		return nil, fmt.Errorf("failed to create snapshot dir: %w", err)
	}

	snap := &Snapshot{
		Name:       name,
		VolumeName: volumeName,
		Path:       snapPath,
		SizeBytes:  vol.SizeBytes,
	}

	sm.snapshots[name] = snap
	return snap, nil
}

// DeleteSnapshot 删除快照
func (sm *StorageManager) DeleteSnapshot(name string) error {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	snap, ok := sm.snapshots[name]
	if !ok {
		return fmt.Errorf("snapshot %s not found", name)
	}

	_ = os.RemoveAll(snap.Path)
	delete(sm.snapshots, name)
	return nil
}

// ListSnapshots 列出所有快照
func (sm *StorageManager) ListSnapshots() []*Snapshot {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	result := make([]*Snapshot, 0, len(sm.snapshots))
	for _, s := range sm.snapshots {
		result = append(result, s)
	}
	return result
}

// === 存储类 ===

// ListStorageClasses 列出所有存储类
func (sm *StorageManager) ListStorageClasses() []*StorageClass {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	result := make([]*StorageClass, 0, len(sm.classes))
	for _, sc := range sm.classes {
		result = append(result, sc)
	}
	return result
}
