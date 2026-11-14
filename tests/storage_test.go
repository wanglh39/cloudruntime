package runtime_test

import (
	"testing"

	st "github.com/cloudruntime/mini/src/storage"
)

func TestStorageManager_CreateVolume(t *testing.T) {
	sm := st.NewStorageManager()

	vol, err := sm.CreateVolume("vol-1", 1073741824, st.ReadWriteOnce, "local-fast")
	if err != nil {
		t.Fatal(err)
	}

	if vol.Name != "vol-1" {
		t.Fatalf("expected name vol-1, got %s", vol.Name)
	}
	if vol.Status != st.VolumeAvailable {
		t.Fatalf("expected available, got %s", vol.Status)
	}
	if vol.AccessMode != st.ReadWriteOnce {
		t.Fatalf("expected RWO, got %s", vol.AccessMode)
	}
}

func TestStorageManager_CreateVolume_Duplicate(t *testing.T) {
	sm := st.NewStorageManager()

	sm.CreateVolume("vol-1", 1073741824, st.ReadWriteOnce, "local-fast")
	_, err := sm.CreateVolume("vol-1", 1073741824, st.ReadWriteOnce, "local-fast")
	if err == nil {
		t.Fatal("expected error for duplicate volume")
	}
}

func TestStorageManager_BindVolume(t *testing.T) {
	sm := st.NewStorageManager()

	sm.CreateVolume("vol-1", 1073741824, st.ReadWriteOnce, "local-fast")

	if err := sm.BindVolume("vol-1", "sandbox-a"); err != nil {
		t.Fatal(err)
	}

	vol, _ := sm.GetVolume("vol-1")
	if vol.Status != st.VolumeBound {
		t.Fatalf("expected bound, got %s", vol.Status)
	}
	if vol.BoundTo != "sandbox-a" {
		t.Fatalf("expected bound to sandbox-a, got %s", vol.BoundTo)
	}
}

func TestStorageManager_BindVolume_RWO_AlreadyBound(t *testing.T) {
	sm := st.NewStorageManager()

	sm.CreateVolume("vol-1", 1073741824, st.ReadWriteOnce, "local-fast")
	sm.BindVolume("vol-1", "sandbox-a")

	// RWO 卷已绑定到 sandbox-a，不能再绑定到 sandbox-b
	err := sm.BindVolume("vol-1", "sandbox-b")
	if err == nil {
		t.Fatal("expected error for RWO double bind")
	}
}

func TestStorageManager_BindVolume_RWX_MultiBind(t *testing.T) {
	sm := st.NewStorageManager()

	sm.CreateVolume("vol-1", 1073741824, st.ReadWriteMany, "local-fast")
	sm.BindVolume("vol-1", "sandbox-a")

	// RWX 卷可以被多个沙箱绑定
	err := sm.BindVolume("vol-1", "sandbox-b")
	if err != nil {
		t.Fatalf("RWX should allow multi-bind: %v", err)
	}
}

func TestStorageManager_UnbindVolume(t *testing.T) {
	sm := st.NewStorageManager()

	sm.CreateVolume("vol-1", 1073741824, st.ReadWriteOnce, "local-fast")
	sm.BindVolume("vol-1", "sandbox-a")
	sm.UnbindVolume("vol-1")

	vol, _ := sm.GetVolume("vol-1")
	if vol.Status != st.VolumeReleased {
		t.Fatalf("expected released, got %s", vol.Status)
	}
	if vol.BoundTo != "" {
		t.Fatalf("expected empty boundTo, got %s", vol.BoundTo)
	}
}

func TestStorageManager_DeleteVolume_Bound(t *testing.T) {
	sm := st.NewStorageManager()

	sm.CreateVolume("vol-1", 1073741824, st.ReadWriteOnce, "local-fast")
	sm.BindVolume("vol-1", "sandbox-a")

	// 绑定中的卷不能删除
	err := sm.DeleteVolume("vol-1")
	if err == nil {
		t.Fatal("expected error for deleting bound volume")
	}
}

func TestStorageManager_DeleteVolume_Released(t *testing.T) {
	sm := st.NewStorageManager()

	sm.CreateVolume("vol-1", 1073741824, st.ReadWriteOnce, "local-fast")
	sm.BindVolume("vol-1", "sandbox-a")
	sm.UnbindVolume("vol-1")

	// 释放后的卷可以删除
	err := sm.DeleteVolume("vol-1")
	if err != nil {
		t.Fatalf("expected success, got %v", err)
	}
}

func TestStorageManager_CreateVolumeFromClaim(t *testing.T) {
	sm := st.NewStorageManager()

	claim := st.VolumeClaim{
		Name:         "pvc-1",
		SizeBytes:    2147483648,
		AccessMode:   st.ReadWriteOnce,
		StorageClass: "local-fast",
	}

	vol, err := sm.CreateVolumeFromClaim(claim)
	if err != nil {
		t.Fatal(err)
	}

	if vol.Name != "pvc-1" {
		t.Fatalf("expected name pvc-1, got %s", vol.Name)
	}
	if vol.SizeBytes != 2147483648 {
		t.Fatalf("expected 2GB, got %d", vol.SizeBytes)
	}
}

func TestStorageManager_CreateLayer(t *testing.T) {
	sm := st.NewStorageManager()

	layer, err := sm.CreateLayer("base-layer", "")
	if err != nil {
		t.Fatal(err)
	}

	if layer.Name != "base-layer" {
		t.Fatalf("expected base-layer, got %s", layer.Name)
	}
	if !layer.ReadOnly {
		t.Fatal("expected read-only layer")
	}
}

func TestStorageManager_CreateLayer_Chain(t *testing.T) {
	sm := st.NewStorageManager()

	sm.CreateLayer("base", "")
	layer2, err := sm.CreateLayer("python", "base")
	if err != nil {
		t.Fatal(err)
	}

	if layer2.Parent != "base" {
		t.Fatalf("expected parent base, got %s", layer2.Parent)
	}
}

func TestStorageManager_CreateFilesystem(t *testing.T) {
	sm := st.NewStorageManager()

	sm.CreateLayer("base", "")
	sm.CreateLayer("python", "base")

	fs, err := sm.CreateFilesystem("sandbox-1", []string{"base", "python"})
	if err != nil {
		t.Fatal(err)
	}

	if fs.SandboxName != "sandbox-1" {
		t.Fatalf("expected sandbox-1, got %s", fs.SandboxName)
	}
	if len(fs.LowerDirs) != 2 {
		t.Fatalf("expected 2 lower dirs, got %d", len(fs.LowerDirs))
	}
}

func TestStorageManager_CreateFilesystem_MissingLayer(t *testing.T) {
	sm := st.NewStorageManager()

	_, err := sm.CreateFilesystem("sandbox-1", []string{"nonexistent"})
	if err == nil {
		t.Fatal("expected error for missing layer")
	}
}

func TestStorageManager_Snapshot(t *testing.T) {
	sm := st.NewStorageManager()

	sm.CreateVolume("vol-1", 1073741824, st.ReadWriteOnce, "local-fast")

	snap, err := sm.CreateSnapshot("snap-1", "vol-1")
	if err != nil {
		t.Fatal(err)
	}

	if snap.VolumeName != "vol-1" {
		t.Fatalf("expected vol-1, got %s", snap.VolumeName)
	}

	// 列出快照
	snaps := sm.ListSnapshots()
	if len(snaps) != 1 {
		t.Fatalf("expected 1 snapshot, got %d", len(snaps))
	}

	// 删除快照
	sm.DeleteSnapshot("snap-1")
	snaps = sm.ListSnapshots()
	if len(snaps) != 0 {
		t.Fatalf("expected 0 snapshots after delete, got %d", len(snaps))
	}
}

func TestStorageManager_ListStorageClasses(t *testing.T) {
	sm := st.NewStorageManager()

	classes := sm.ListStorageClasses()
	if len(classes) < 2 {
		t.Fatalf("expected at least 2 storage classes, got %d", len(classes))
	}
}

func TestStorageManager_ListVolumes_Empty(t *testing.T) {
	sm := st.NewStorageManager()

	vols := sm.ListVolumes()
	if len(vols) != 0 {
		t.Fatalf("expected 0 volumes, got %d", len(vols))
	}
}
