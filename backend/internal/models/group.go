package models

// DeviceGroup is a named set of devices within a project, used for batch
// operations, OTA targeting and organising large fleets.
type DeviceGroup struct {
	Base
	ProjectID   uint   `gorm:"index;not null" json:"projectId"`
	Name        string `gorm:"size:160;not null" json:"name"`
	Description string `gorm:"type:text" json:"description"`
}

func (DeviceGroup) TableName() string { return "device_groups" }

// DeviceGroupMember links a device to a group.
type DeviceGroupMember struct {
	Base
	GroupID  uint `gorm:"uniqueIndex:idx_group_device;not null" json:"groupId"`
	DeviceID uint `gorm:"uniqueIndex:idx_group_device;not null" json:"deviceId"`
}

func (DeviceGroupMember) TableName() string { return "device_group_members" }
