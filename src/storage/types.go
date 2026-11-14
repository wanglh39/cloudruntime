package storage

// AccessMode 定义 Volume 的访问模式
type AccessMode string

const (
	ReadWriteOnce AccessMode = "RWO" // 单沙箱读写
	ReadOnlyMany  AccessMode = "ROX" // 多沙箱只读
	ReadWriteMany AccessMode = "RWX" // 多沙箱读写
)

// VolumeStatus 定义 Volume 的状态
type VolumeStatus string

const (
	VolumeAvailable VolumeStatus = "available" // 已创建，未绑定
	VolumeBound     VolumeStatus = "bound"     // 已绑定到沙箱
	VolumeReleased  VolumeStatus = "released"  // 沙箱已释放，Volume 仍存在
)

// StorageClass 定义存储类型
type StorageClass struct {
	Name        string `json:"name"`
	Backend     string `json:"backend"` // local, nfs, ceph
	Provisioner string `json:"provisioner"`
}

// Volume 持久化卷
type Volume struct {
	Name         string       `json:"name"`
	SizeBytes    int64        `json:"sizeBytes"`
	AccessMode   AccessMode   `json:"accessMode"`
	StorageClass string       `json:"storageClass"`
	Path         string       `json:"path"` // 宿主机上的路径
	Status       VolumeStatus `json:"status"`
	BoundTo      string       `json:"boundTo"` // 绑定的沙箱名
}

// VolumeClaim 用户对存储的声明（动态供应）
type VolumeClaim struct {
	Name         string     `json:"name"`
	SizeBytes    int64      `json:"sizeBytes"`
	AccessMode   AccessMode `json:"accessMode"`
	StorageClass string     `json:"storageClass"`
}

// Layer 文件系统层
type Layer struct {
	Name     string `json:"name"`
	Path     string `json:"path"`   // 层数据的路径
	Parent   string `json:"parent"` // 父层名称
	ReadOnly bool   `json:"readOnly"`
}

// Filesystem 沙箱的分层文件系统
type Filesystem struct {
	SandboxName string   `json:"sandboxName"`
	LowerDirs   []string `json:"lowerDirs"` // 只读层路径列表
	UpperDir    string   `json:"upperDir"`  // 读写层路径
	WorkDir     string   `json:"workDir"`   // overlayfs 工作目录
	MergedDir   string   `json:"mergedDir"` // 合并视图路径
}

// Snapshot Volume 快照
type Snapshot struct {
	Name       string `json:"name"`
	VolumeName string `json:"volumeName"`
	Path       string `json:"path"`
	SizeBytes  int64  `json:"sizeBytes"`
}

// DefaultStorageClasses 默认存储类
func DefaultStorageClasses() []StorageClass {
	return []StorageClass{
		{Name: "local-fast", Backend: "local", Provisioner: "local"},
		{Name: "local-bulk", Backend: "local", Provisioner: "local"},
	}
}
