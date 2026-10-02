package models

import "time"

// Base holds common fields shared by most persisted entities.
type Base struct {
	ID        uint      `gorm:"primaryKey" json:"id"`
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}

const (
	SystemRoleAdmin = "admin"
	SystemRoleUser  = "user"
)

// User is a platform account. A user may belong to many projects through
// ProjectMember.
type User struct {
	Base
	Username     string `gorm:"uniqueIndex;size:64;not null" json:"username"`
	Email        string `gorm:"uniqueIndex;size:160;not null" json:"email"`
	DisplayName  string `gorm:"size:120" json:"displayName"`
	PasswordHash string `gorm:"size:255;not null" json:"-"`
	SystemRole   string `gorm:"size:32;not null;default:user" json:"systemRole"`
	Status       string `gorm:"size:32;not null;default:active" json:"status"`
}

func (User) TableName() string { return "users" }

const (
	ProjectRoleOwner  = "owner"
	ProjectRoleAdmin  = "admin"
	ProjectRoleMember = "member"
	ProjectRoleViewer = "viewer"
)

// Project is the top-level tenant boundary. Everything (workspaces, products,
// devices, rules) lives under a project.
type Project struct {
	Base
	Key         string `gorm:"uniqueIndex;size:64;not null" json:"key"`
	Name        string `gorm:"size:160;not null" json:"name"`
	Description string `gorm:"type:text" json:"description"`
	OwnerID     uint   `gorm:"index;not null" json:"ownerId"`

	Owner   *User           `gorm:"foreignKey:OwnerID" json:"owner,omitempty"`
	Members []ProjectMember `gorm:"foreignKey:ProjectID" json:"members,omitempty"`
}

func (Project) TableName() string { return "projects" }

// ProjectMember grants a user access to a project with a scoped role.
type ProjectMember struct {
	Base
	ProjectID uint   `gorm:"uniqueIndex:idx_project_user;not null" json:"projectId"`
	UserID    uint   `gorm:"uniqueIndex:idx_project_user;not null" json:"userId"`
	Role      string `gorm:"size:32;not null;default:member" json:"role"`

	User *User `gorm:"foreignKey:UserID" json:"user,omitempty"`
}

func (ProjectMember) TableName() string { return "project_members" }

// Workspace is a logical grouping of devices. Devices may only exchange
// messages with other devices that live in the same workspace. The key is
// globally unique because it forms the device topic namespace.
type Workspace struct {
	Base
	ProjectID   uint   `gorm:"uniqueIndex:idx_workspace_project;not null" json:"projectId"`
	Key         string `gorm:"uniqueIndex:idx_workspace_project;size:64;not null" json:"key"`
	Name        string `gorm:"size:160;not null" json:"name"`
	Description string `gorm:"type:text" json:"description"`
}

func (Workspace) TableName() string { return "workspaces" }

const (
	ProtocolMQTT   = "mqtt"
	ProtocolCoAP   = "coap"
	ProtocolCustom = "custom"

	ProductStatusDraft     = "draft"
	ProductStatusPublished = "published"
)

// Product is a global model/type of device, independent of any project. It
// owns a thing model and defines the transport protocol used by its devices.
type Product struct {
	Base
	Key         string `gorm:"uniqueIndex;size:64;not null" json:"key"`
	Name        string `gorm:"size:160;not null" json:"name"`
	Category    string `gorm:"size:64" json:"category"`
	Protocol    string `gorm:"size:32;not null;default:mqtt" json:"protocol"`
	DataFormat  string `gorm:"size:32;not null;default:json" json:"dataFormat"`
	Description string `gorm:"type:text" json:"description"`
	Status      string `gorm:"size:32;not null;default:draft" json:"status"`

	ThingModel *ThingModel `gorm:"foreignKey:ProductID" json:"thingModel,omitempty"`
}

func (Product) TableName() string { return "products" }

const (
	ElementProperty = "property"
	ElementService  = "service"
	ElementEvent    = "event"

	AccessRead      = "r"
	AccessWrite     = "w"
	AccessReadWrite = "rw"
)

// ThingModel is the versioned description of a product's capabilities.
type ThingModel struct {
	Base
	ProductID   uint   `gorm:"uniqueIndex;not null" json:"productId"`
	Version     string `gorm:"size:32;not null;default:1.0.0" json:"version"`
	Description string `gorm:"type:text" json:"description"`

	Elements []ThingModelElement `gorm:"foreignKey:ThingModelID" json:"elements,omitempty"`
}

func (ThingModel) TableName() string { return "thing_models" }

// ThingModelElement is a single property, service or event definition.
type ThingModelElement struct {
	Base
	ThingModelID uint   `gorm:"index;not null" json:"thingModelId"`
	Type         string `gorm:"size:32;not null" json:"type"`       // property|service|event
	Identifier   string `gorm:"size:128;not null" json:"identifier"` // e.g. temperature
	Name         string `gorm:"size:160" json:"name"`
	DataType     string `gorm:"size:32;not null;default:double" json:"dataType"`
	AccessMode   string `gorm:"size:8;default:rw" json:"accessMode"`
	Unit         string `gorm:"size:32" json:"unit"`
	Min          *float64 `json:"min"`
	Max          *float64 `json:"max"`
	Step         *float64 `json:"step"`
	UnitName     string `gorm:"size:32" json:"unitName"`
	Specs        JSONMap `gorm:"type:jsonb" json:"specs"` // enum, struct fields, call params, etc.
	Required     bool   `gorm:"default:false" json:"required"`
	Description  string `gorm:"type:text" json:"description"`
}

func (ThingModelElement) TableName() string { return "thing_model_elements" }

const (
	DeviceStatusEnabled  = "enabled"
	DeviceStatusDisabled = "disabled"
)

// Device is a concrete connected thing.
type Device struct {
	Base
	ProjectID   uint    `gorm:"index;not null" json:"projectId"`
	WorkspaceID uint    `gorm:"index;not null" json:"workspaceId"`
	ProductID   uint    `gorm:"index;not null" json:"productId"`
	Key         string  `gorm:"uniqueIndex:idx_device_key;size:128;not null" json:"key"`
	Secret      string  `gorm:"size:128" json:"-"`
	Name        string  `gorm:"size:160;not null" json:"name"`
	Status      string  `gorm:"size:32;not null;default:enabled" json:"status"`
	Online      bool    `gorm:"default:false" json:"online"`
	FirmwareVersion string `gorm:"size:64" json:"firmwareVersion"`
	LastOnlineAt *time.Time `json:"lastOnlineAt"`
	LastSeenAt  *time.Time  `json:"lastSeenAt"`
	Tags        JSONMap `gorm:"type:jsonb" json:"tags"`

	Product   *Product   `gorm:"foreignKey:ProductID" json:"product,omitempty"`
	Workspace *Workspace `gorm:"foreignKey:WorkspaceID" json:"workspace,omitempty"`
}

func (Device) TableName() string { return "devices" }
