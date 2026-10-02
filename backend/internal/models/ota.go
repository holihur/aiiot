package models

import "time"

const (
	FirmwareStatusDraft     = "draft"
	FirmwareStatusPublished = "published"
)

// Firmware is an uploaded binary for a product, addressed by semantic version.
type Firmware struct {
	Base
	ProductID   uint   `gorm:"index;not null" json:"productId"`
	Version     string `gorm:"size:64;not null" json:"version"`
	Name        string `gorm:"size:160" json:"name"`
	Description string `gorm:"type:text" json:"description"`
	FileName    string `gorm:"size:255" json:"fileName"`
	StoredPath  string `gorm:"size:512" json:"-"`
	FileSize    int64  `json:"fileSize"`
	Checksum    string `gorm:"size:128" json:"checksum"` // sha256 hex
	Status      string `gorm:"size:32;not null;default:published" json:"status"`
	UploadedBy  uint   `json:"uploadedBy"`
}

func (Firmware) TableName() string { return "firmwares" }

const (
	OTATaskPending   = "pending"
	OTATaskRunning   = "running"
	OTATaskSucceeded = "succeeded"
	OTATaskFailed    = "failed"
	OTATaskCanceled  = "canceled"
)

// OTATask is a firmware rollout to a set of devices.
type OTATask struct {
	Base
	ProjectID  uint   `gorm:"index;not null" json:"projectId"`
	ProductID  uint   `gorm:"index;not null" json:"productId"`
	FirmwareID uint   `gorm:"index;not null" json:"firmwareId"`
	Name       string `gorm:"size:160;not null" json:"name"`
	Status     string `gorm:"size:32;not null;default:pending" json:"status"`

	Total      int `json:"total"`
	Succeeded  int `json:"succeeded"`
	Failed     int `json:"failed"`
	InProgress int `json:"inProgress"`

	CreatedBy  uint       `json:"createdBy"`
	StartedAt  *time.Time `json:"startedAt"`
	FinishedAt *time.Time `json:"finishedAt"`

	// BatchSize splits the rollout into waves of this many devices (0 = one
	// wave). CurrentWave is the wave currently being dispatched.
	BatchSize     int  `gorm:"default:0" json:"batchSize"`
	CurrentWave   int  `gorm:"default:0" json:"currentWave"`
	HaltOnFailure bool `gorm:"default:true" json:"haltOnFailure"`

	Firmware *Firmware       `gorm:"foreignKey:FirmwareID" json:"firmware,omitempty"`
	Devices  []OTATaskDevice `gorm:"foreignKey:TaskID" json:"devices,omitempty"`
}

func (OTATask) TableName() string { return "ota_tasks" }

const (
	OTADevicePending     = "pending"
	OTADeviceDispatched  = "dispatched"
	OTADeviceDownloading = "downloading"
	OTADeviceUpgrading   = "upgrading"
	OTADeviceSucceeded   = "succeeded"
	OTADeviceFailed      = "failed"
)

// OTATaskDevice tracks per-device rollout progress.
type OTATaskDevice struct {
	Base
	TaskID   uint   `gorm:"uniqueIndex:idx_ota_task_device;not null" json:"taskId"`
	DeviceID uint   `gorm:"uniqueIndex:idx_ota_task_device;not null" json:"deviceId"`
	Status   string `gorm:"size:32;not null;default:pending" json:"status"`
	Progress int    `json:"progress"`
	Message  string `gorm:"type:text" json:"message"`
	Wave     int    `gorm:"default:0" json:"wave"`

	Device *Device `gorm:"foreignKey:DeviceID" json:"device,omitempty"`
}

func (OTATaskDevice) TableName() string { return "ota_task_devices" }
