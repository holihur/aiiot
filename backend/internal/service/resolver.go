package service

import (
	"context"
	"crypto/subtle"
	"fmt"
	"sync"
	"time"

	"github.com/aiiot/server/internal/access"
	"github.com/aiiot/server/internal/models"
	"gorm.io/gorm"
)

// DeviceContext bundles everything needed to route messages for one device.
type DeviceContext struct {
	Device       *models.Device
	Product      *models.Product
	Workspace    *models.Workspace
	Project      *models.Project
	WorkspaceKey string
}

// Ref builds the wire-level device reference used by the access layer.
func (d *DeviceContext) Ref() access.DeviceRef {
	return access.DeviceRef{
		WorkspaceKey: d.WorkspaceKey,
		ProductKey:   d.Product.Key,
		DeviceKey:    d.Device.Key,
	}
}

// Resolver resolves devices by key or ID and caches immutable key lookups.
type Resolver struct {
	db *gorm.DB

	mu     sync.RWMutex
	spaces map[uint]string // workspaceID -> key
}

func NewResolver(db *gorm.DB) *Resolver {
	return &Resolver{db: db, spaces: map[uint]string{}}
}

func (r *Resolver) WorkspaceKey(ctx context.Context, id uint) (string, error) {
	r.mu.RLock()
	if k, ok := r.spaces[id]; ok {
		r.mu.RUnlock()
		return k, nil
	}
	r.mu.RUnlock()

	var w models.Workspace
	if err := r.db.WithContext(ctx).Select("id", "key").First(&w, id).Error; err != nil {
		return "", err
	}
	r.mu.Lock()
	r.spaces[id] = w.Key
	r.mu.Unlock()
	return w.Key, nil
}

// Resolve looks up a device from its product/device keys. Device keys are
// globally unique. Products are global, so the product key is only verified
// when supplied (it comes from the connection username).
func (r *Resolver) Resolve(ctx context.Context, productKey, deviceKey string) (*DeviceContext, error) {
	var device models.Device
	if err := r.db.WithContext(ctx).Where("key = ?", deviceKey).First(&device).Error; err != nil {
		return nil, fmt.Errorf("device %q: %w", deviceKey, err)
	}
	var product models.Product
	if err := r.db.WithContext(ctx).First(&product, device.ProductID).Error; err != nil {
		return nil, fmt.Errorf("product for device %q: %w", deviceKey, err)
	}
	if productKey != "" && product.Key != productKey {
		return nil, fmt.Errorf("device %q does not belong to product %q", deviceKey, productKey)
	}
	var project models.Project
	if err := r.db.WithContext(ctx).First(&project, device.ProjectID).Error; err != nil {
		return nil, fmt.Errorf("project for device %q: %w", deviceKey, err)
	}
	return r.assemble(ctx, &device, &product, &project)
}

// ResolveByDeviceID looks up a device from its internal ID.
func (r *Resolver) ResolveByDeviceID(ctx context.Context, id uint) (*DeviceContext, error) {
	var device models.Device
	if err := r.db.WithContext(ctx).First(&device, id).Error; err != nil {
		return nil, err
	}
	var product models.Product
	if err := r.db.WithContext(ctx).First(&product, device.ProductID).Error; err != nil {
		return nil, err
	}
	var project models.Project
	if err := r.db.WithContext(ctx).First(&project, device.ProjectID).Error; err != nil {
		return nil, err
	}
	return r.assemble(ctx, &device, &product, &project)
}

func (r *Resolver) assemble(ctx context.Context, device *models.Device, product *models.Product, project *models.Project) (*DeviceContext, error) {
	var ws models.Workspace
	if err := r.db.WithContext(ctx).First(&ws, device.WorkspaceID).Error; err != nil {
		return nil, fmt.Errorf("workspace %d: %w", device.WorkspaceID, err)
	}
	r.mu.Lock()
	r.spaces[ws.ID] = ws.Key
	r.mu.Unlock()

	return &DeviceContext{
		Device:       device,
		Product:      product,
		Workspace:    &ws,
		Project:      project,
		WorkspaceKey: ws.Key,
	}, nil
}

// Authenticate validates a device credential presented by a gateway.
func (r *Resolver) Authenticate(ctx context.Context, req *access.AuthRequest) (*access.AuthResponse, error) {
	resp := &access.AuthResponse{Authorized: false}

	var device models.Device
	if err := r.db.WithContext(ctx).Where("key = ?", req.DeviceKey).First(&device).Error; err != nil {
		resp.Reason = "unknown device"
		return resp, nil
	}
	var product models.Product
	if err := r.db.WithContext(ctx).First(&product, device.ProductID).Error; err != nil {
		resp.Reason = "unknown product"
		return resp, nil
	}
	if req.ProductKey != "" && product.Key != req.ProductKey {
		resp.Reason = "product mismatch"
		return resp, nil
	}
	result, err := r.authorizeDevice(ctx, &product, &device, req, resp)
	if err != nil {
		return result, err
	}
	// Tenant binding: when the access subdomain supplied a workspace hint, the
	// device must belong to it.
	if result.Authorized && req.Workspace != "" && result.WorkspaceKey != req.Workspace {
		return &access.AuthResponse{Authorized: false, Reason: "device is not in workspace " + req.Workspace}, nil
	}
	return result, nil
}

func (r *Resolver) authorizeDevice(ctx context.Context, product *models.Product, device *models.Device, req *access.AuthRequest, resp *access.AuthResponse) (*access.AuthResponse, error) {
	if device.Status != models.DeviceStatusEnabled {
		resp.Reason = "device disabled"
		return resp, nil
	}
	if device.Secret != "" {
		got := req.Password
		if subtle.ConstantTimeCompare([]byte(device.Secret), []byte(got)) != 1 {
			resp.Reason = "invalid secret"
			return resp, nil
		}
	}
	workspaceKey, err := r.WorkspaceKey(ctx, device.WorkspaceID)
	if err != nil {
		return nil, err
	}
	*resp = access.AuthResponse{
		Authorized:   true,
		DeviceID:     device.ID,
		ProjectID:    device.ProjectID,
		WorkspaceID:  device.WorkspaceID,
		ProductID:    product.ID,
		WorkspaceKey: workspaceKey,
		ProductKey:   product.Key,
		DeviceKey:    device.Key,
		Protocol:     product.Protocol,
	}
	return resp, nil
}

// TouchSeen updates the device's last-seen timestamp and online flag.
func (r *Resolver) TouchSeen(ctx context.Context, deviceID uint) {
	now := time.Now().UTC()
	r.db.WithContext(ctx).Model(&models.Device{}).Where("id = ?", deviceID).
		Updates(map[string]any{"last_seen_at": now, "online": true})
}
