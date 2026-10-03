package api

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/aiiot/server/internal/access"
	"github.com/aiiot/server/internal/models"
	"github.com/aiiot/server/internal/service"
	"github.com/gin-gonic/gin"
)

func newSecret() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return hex.EncodeToString([]byte(time.Now().String()))[:32]
	}
	return hex.EncodeToString(b)
}

type deviceRequest struct {
	Key         string         `json:"key"`
	Name        string         `json:"name" binding:"required,max=160"`
	WorkspaceID uint           `json:"workspaceId" binding:"required"`
	Secret      string         `json:"secret"`
	Tags        models.JSONMap `json:"tags"`
}

// updateDeviceRequest is the loose-form update payload: only the non-zero
// fields are applied, so callers can update tags without resending the name.
type updateDeviceRequest struct {
	Name        string         `json:"name" binding:"omitempty,max=160"`
	WorkspaceID uint           `json:"workspaceId"`
	Status      string         `json:"status"`
	Tags        models.JSONMap `json:"tags"`
}

// ListProjectDevices lists devices across a project with optional filters.
func (h *Handlers) ListProjectDevices(c *gin.Context) {
	projectID, valid := parseID(c, "id")
	if !valid {
		return
	}
	if !h.requireProject(c, projectID, models.ProjectRoleViewer) {
		return
	}
	q := h.DB.WithContext(c).Where("project_id = ?", projectID)
	if pid := queryUint(c, "productId"); pid != 0 {
		q = q.Where("product_id = ?", pid)
	}
	if wid := queryUint(c, "workspaceId"); wid != 0 {
		q = q.Where("workspace_id = ?", wid)
	}
	if online := c.Query("online"); online != "" {
		q = q.Where("online = ?", online == "true")
	}
	if kw := strings.TrimSpace(c.Query("keyword")); kw != "" {
		q = q.Where("name ILIKE ? OR key ILIKE ?", "%"+kw+"%", "%"+kw+"%")
	}

	var total int64
	q.Model(&models.Device{}).Count(&total)
	c.Header("X-Total-Count", strconv.FormatInt(total, 10))

	// Sorting (whitelisted to prevent injection).
	sortCol := "id"
	switch c.Query("sort") {
	case "name", "key", "online", "last_seen_at", "created_at", "id":
		sortCol = c.Query("sort")
	}
	dir := "DESC"
	if strings.EqualFold(c.Query("order"), "asc") {
		dir = "ASC"
	}
	q = q.Order(sortCol + " " + dir)

	// Pagination: when page is provided, page through; otherwise keep the
	// legacy large limit for callers that do not paginate.
	if page := queryUint(c, "page"); page > 0 {
		pageSize := int(queryUint(c, "pageSize"))
		if pageSize <= 0 || pageSize > 200 {
			pageSize = 50
		}
		q = q.Offset(int(page-1) * pageSize).Limit(pageSize)
	} else {
		q = q.Limit(500)
	}

	var devices []models.Device
	if err := q.Preload("Product").Preload("Workspace").Find(&devices).Error; err != nil {
		fail(c, http.StatusInternalServerError, err.Error())
		return
	}
	ok(c, devices)
}

func (h *Handlers) ListWorkspaceDevices(c *gin.Context) {
	workspaceID, valid := parseID(c, "id")
	if !valid {
		return
	}
	ws, allowed := h.loadWorkspace(c, workspaceID, models.ProjectRoleViewer)
	if !allowed {
		return
	}
	var devices []models.Device
	limit := parseLimit(c.Query("limit"), 100, 1000)
	if err := h.DB.WithContext(c).Where("workspace_id = ?", ws.ID).
		Preload("Product").Order("id DESC").Limit(limit).Find(&devices).Error; err != nil {
		fail(c, http.StatusInternalServerError, err.Error())
		return
	}
	ok(c, devices)
}

// CreateDevice creates a device under a product, returning its secret once.
func (h *Handlers) CreateDevice(c *gin.Context) {
	productID, valid := parseID(c, "id")
	if !valid {
		return
	}
	product, allowed := h.loadProduct(c, productID)
	if !allowed {
		return
	}
	var req deviceRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		fail(c, http.StatusBadRequest, err.Error())
		return
	}
	var ws models.Workspace
	if err := h.DB.WithContext(c).First(&ws, req.WorkspaceID).Error; err != nil {
		fail(c, http.StatusBadRequest, "workspace not found")
		return
	}
	if !h.requireProject(c, ws.ProjectID, models.ProjectRoleMember) {
		return
	}
	key := strings.TrimSpace(req.Key)
	if key == "" {
		key = generateKey("dev", req.Name)
	}
	var existing int64
	h.DB.Model(&models.Device{}).Where("key = ?", key).Count(&existing)
	if existing > 0 {
		fail(c, http.StatusConflict, "device key already exists")
		return
	}
	secret := req.Secret
	if secret == "" {
		secret = newSecret()
	}
	device := models.Device{
		ProjectID:   ws.ProjectID,
		WorkspaceID: ws.ID,
		ProductID:   product.ID,
		Key:         key,
		Secret:      secret,
		Name:        req.Name,
		Status:      models.DeviceStatusEnabled,
		Tags:        req.Tags,
	}
	if err := h.DB.Create(&device).Error; err != nil {
		fail(c, http.StatusInternalServerError, err.Error())
		return
	}
	created(c, gin.H{"device": device, "secret": secret})
}

func (h *Handlers) GetDevice(c *gin.Context) {
	id, valid := parseID(c, "id")
	if !valid {
		return
	}
	device, allowed := h.loadDevice(c, id, models.ProjectRoleViewer)
	if !allowed {
		return
	}
	h.DB.WithContext(c).Preload("Product").Preload("Workspace").First(device, device.ID)
	ok(c, device)
}

func (h *Handlers) UpdateDevice(c *gin.Context) {
	id, valid := parseID(c, "id")
	if !valid {
		return
	}
	device, allowed := h.loadDevice(c, id, models.ProjectRoleMember)
	if !allowed {
		return
	}
	var req updateDeviceRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		fail(c, http.StatusBadRequest, err.Error())
		return
	}
	if req.Name != "" {
		device.Name = req.Name
	}
	if req.Status != "" {
		device.Status = req.Status
	}
	if req.WorkspaceID != 0 && req.WorkspaceID != device.WorkspaceID {
		var ws models.Workspace
		if err := h.DB.WithContext(c).Where("id = ? AND project_id = ?", req.WorkspaceID, device.ProjectID).First(&ws).Error; err != nil {
			fail(c, http.StatusBadRequest, "workspace not found in this project")
			return
		}
		device.WorkspaceID = ws.ID
	}
	if req.Tags != nil {
		device.Tags = req.Tags
	}
	if err := h.DB.Save(device).Error; err != nil {
		fail(c, http.StatusInternalServerError, err.Error())
		return
	}
	ok(c, device)
}

func (h *Handlers) DeleteDevice(c *gin.Context) {
	id, valid := parseID(c, "id")
	if !valid {
		return
	}
	device, allowed := h.loadDevice(c, id, models.ProjectRoleAdmin)
	if !allowed {
		return
	}
	if err := h.DB.Delete(&models.Device{}, device.ID).Error; err != nil {
		fail(c, http.StatusInternalServerError, err.Error())
		return
	}
	ok(c, gin.H{"ok": true})
}

func (h *Handlers) SetDeviceStatus(c *gin.Context) {
	id, valid := parseID(c, "id")
	if !valid {
		return
	}
	device, allowed := h.loadDevice(c, id, models.ProjectRoleMember)
	if !allowed {
		return
	}
	var body struct {
		Status string `json:"status" binding:"required,oneof=enabled disabled"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		fail(c, http.StatusBadRequest, err.Error())
		return
	}
	device.Status = body.Status
	if err := h.DB.Save(device).Error; err != nil {
		fail(c, http.StatusInternalServerError, err.Error())
		return
	}
	ok(c, device)
}

func (h *Handlers) RotateDeviceSecret(c *gin.Context) {
	id, valid := parseID(c, "id")
	if !valid {
		return
	}
	device, allowed := h.loadDevice(c, id, models.ProjectRoleAdmin)
	if !allowed {
		return
	}
	secret := newSecret()
	if err := h.DB.Model(device).Update("secret", secret).Error; err != nil {
		fail(c, http.StatusInternalServerError, err.Error())
		return
	}
	ok(c, gin.H{"deviceId": device.ID, "secret": secret})
}

func (h *Handlers) DeviceLatest(c *gin.Context) {
	id, valid := parseID(c, "id")
	if !valid {
		return
	}
	device, allowed := h.loadDevice(c, id, models.ProjectRoleViewer)
	if !allowed {
		return
	}
	values, err := h.Telemetry.Latest(c, device.ID)
	if err != nil {
		fail(c, http.StatusInternalServerError, err.Error())
		return
	}
	ok(c, values)
}

func (h *Handlers) DeviceTelemetry(c *gin.Context) {
	id, valid := parseID(c, "id")
	if !valid {
		return
	}
	device, allowed := h.loadDevice(c, id, models.ProjectRoleViewer)
	if !allowed {
		return
	}
	from := parseTime(c.Query("from"), time.Now().Add(-24*time.Hour))
	to := parseTime(c.Query("to"), time.Now().Add(time.Minute))
	rawIdent := c.Query("identifier")
	var identifiers []string
	for _, part := range strings.Split(rawIdent, ",") {
		if part = strings.TrimSpace(part); part != "" {
			identifiers = append(identifiers, part)
		}
	}
	if len(identifiers) == 1 {
		rawIdent = identifiers[0]
	}
	interval := time.Duration(0)
	if raw := c.Query("interval"); raw != "" {
		if d, err := time.ParseDuration(raw); err == nil {
			interval = d
		}
	}
	if interval > 0 {
		var points []service.AggregatePoint
		var err error
		source := "raw"
		if interval >= time.Hour {
			points, err = h.Telemetry.QueryAggregateRollup(c, service.RangeQuery{
				DeviceID: device.ID, Identifier: rawIdent, Identifiers: identifiers, From: from, To: to,
			}, interval)
			source = "rollup"
		} else {
			points, err = h.Telemetry.QueryAggregate(c, service.RangeQuery{
				DeviceID: device.ID, Identifier: rawIdent, Identifiers: identifiers, From: from, To: to,
			}, interval)
		}
		if err != nil {
			fail(c, http.StatusInternalServerError, err.Error())
			return
		}
		ok(c, gin.H{"mode": "aggregate", "source": source, "points": points})
		return
	}
	rows, err := h.Telemetry.QueryRange(c, service.RangeQuery{
		DeviceID:    device.ID,
		Identifier:  rawIdent,
		Identifiers: identifiers,
		From:        from,
		To:          to,
		Limit:       int(queryUint(c, "limit")),
	})
	if err != nil {
		fail(c, http.StatusInternalServerError, err.Error())
		return
	}
	ok(c, gin.H{"mode": "raw", "points": rows})
}

// DeviceEvents lists the events reported by a device, with optional keyword
// search (identifier or payload text) and a since window.
func (h *Handlers) DeviceEvents(c *gin.Context) {
	id, valid := parseID(c, "id")
	if !valid {
		return
	}
	device, allowed := h.loadDevice(c, id, models.ProjectRoleViewer)
	if !allowed {
		return
	}
	limit := parseLimit(c.Query("limit"), 200, 1000)
	q := h.DB.WithContext(c).Where("device_id = ?", device.ID)
	if match := c.Query("q"); match != "" {
		pattern := "%" + match + "%"
		q = q.Where("identifier ILIKE ? OR type ILIKE ? OR payload::text ILIKE ?", pattern, pattern, pattern)
	}
	if since := c.Query("since"); since != "" {
		if t, err := time.Parse(time.RFC3339, since); err == nil {
			q = q.Where("occurred_at >= ?", t)
		}
	}
	var events []models.DeviceEvent
	if err := q.Order("occurred_at DESC").Limit(limit).Find(&events).Error; err != nil {
		fail(c, http.StatusInternalServerError, err.Error())
		return
	}
	ok(c, events)
}

// DeviceDownlinks returns the recent downlink journal for a device: every
// platform-originated message (API, batch, rule, shadow delta, OTA) with its
// result.
func (h *Handlers) DeviceDownlinks(c *gin.Context) {
	id, valid := parseID(c, "id")
	if !valid {
		return
	}
	device, allowed := h.loadDevice(c, id, models.ProjectRoleViewer)
	if !allowed {
		return
	}
	limit := parseLimit(c.Query("limit"), 100, 500)
	q := h.DB.WithContext(c).Where("device_id = ?", device.ID)
	if since := c.Query("since"); since != "" {
		if t, err := time.Parse(time.RFC3339, since); err == nil {
			q = q.Where("occurred_at >= ?", t)
		}
	}
	var logs []models.DeviceDownlinkLog
	if err := q.Order("occurred_at DESC").Limit(limit).Find(&logs).Error; err != nil {
		fail(c, http.StatusInternalServerError, err.Error())
		return
	}
	ok(c, logs)
}

// timelineItem is one entry in the unified device timeline.
type timelineItem struct {
	At         time.Time      `json:"at"`
	Type       string         `json:"type"` // device / lifecycle / event / shadow / downlink / ota
	Title      string         `json:"title"`
	Identifier string         `json:"identifier,omitempty"`
	Source     string         `json:"source,omitempty"`
	Status     string         `json:"status,omitempty"`
	Detail     map[string]any `json:"detail,omitempty"`
}

// DeviceTimeline merges device creation, lifecycle transitions, thing-model
// events, shadow changes, downlinks and OTA progress into one reverse
// chronological feed.
func (h *Handlers) DeviceTimeline(c *gin.Context) {
	id, valid := parseID(c, "id")
	if !valid {
		return
	}
	device, allowed := h.loadDevice(c, id, models.ProjectRoleViewer)
	if !allowed {
		return
	}
	limit := parseLimit(c.Query("limit"), 100, 500)

	items := []timelineItem{{
		At:     device.CreatedAt,
		Type:   "device",
		Title:  "device_created",
		Detail: map[string]any{"key": device.Key, "product": device.ProductID},
	}}

	// Thing-model + lifecycle events.
	var events []models.DeviceEvent
	h.DB.WithContext(c).Where("device_id = ?", device.ID).Order("occurred_at DESC").Limit(limit).Find(&events)
	for _, e := range events {
		items = append(items, timelineItem{
			At: e.OccurredAt, Type: "event", Title: e.Type,
			Identifier: e.Identifier, Detail: map[string]any{"payload": e.Payload},
		})
	}

	// Shadow changes.
	var shadows []models.DeviceShadowLog
	h.DB.WithContext(c).Where("device_id = ?", device.ID).Order("occurred_at DESC").Limit(limit).Find(&shadows)
	for _, s := range shadows {
		items = append(items, timelineItem{
			At: s.OccurredAt, Type: "shadow", Title: "shadow_changed", Source: s.Source,
			Detail: map[string]any{"desired": s.Desired, "reported": s.Reported, "delta": s.Delta},
		})
	}

	// Downlinks.
	var dl []models.DeviceDownlinkLog
	h.DB.WithContext(c).Where("device_id = ?", device.ID).Order("occurred_at DESC").Limit(limit).Find(&dl)
	for _, d := range dl {
		items = append(items, timelineItem{
			At: d.OccurredAt, Type: "downlink", Title: "downlink_" + d.Kind,
			Identifier: d.Identifier, Source: d.Source, Status: d.Status,
			Detail: map[string]any{
				"payload": d.Payload,
				"target":  d.Target,
				"error":   d.Error,
			},
		})
	}

	// OTA task membership (dispatched / progress / result).
	var ota []struct {
		models.OTATaskDevice
		TaskName string `gorm:"column:task_name"`
	}
	h.DB.WithContext(c).Table("ota_task_devices ot").
		Select("ot.*, t.name AS task_name").
		Joins("JOIN ota_tasks t ON t.id = ot.task_id").
		Where("ot.device_id = ?", device.ID).
		Order("ot.created_at DESC").Limit(limit).Find(&ota)
	for _, o := range ota {
		items = append(items, timelineItem{
			At: o.CreatedAt, Type: "ota", Title: "ota_" + o.OTATaskDevice.Status,
			Source: models.DownlinkSourceOTA, Status: o.OTATaskDevice.Status,
			Detail: map[string]any{
				"task": o.TaskName, "taskId": o.TaskID,
				"progress": o.Progress, "message": o.Message,
			},
		})
	}

	sort.Slice(items, func(i, j int) bool { return items[i].At.After(items[j].At) })
	if len(items) > limit {
		items = items[:limit]
	}
	ok(c, items)
}

// parseLimit parses a page-size query parameter with a default and a cap.
func parseLimit(raw string, def, cap int) int {
	n, err := strconv.Atoi(raw)
	if err != nil || n <= 0 {
		return def
	}
	if n > cap {
		return cap
	}
	return n
}

type commandRequest struct {
	Kind       string          `json:"kind"`
	Identifier string          `json:"identifier"`
	Payload    json.RawMessage `json:"payload"`
}

type batchDeviceRequest struct {
	DeviceIDs []uint `json:"deviceIds"`
	// GroupID targets all devices in a group (union with DeviceIDs).
	GroupID uint `json:"groupId"`
	// Action is "command" (default) or "set_desired".
	Action     string          `json:"action"`
	Kind       string          `json:"kind"`
	Identifier string          `json:"identifier"`
	Payload    json.RawMessage `json:"payload"`
	Desired    map[string]any  `json:"desired"`
}

// BatchDeviceCommand applies a downlink command or a shadow desired-state
// change to many devices at once, returning a per-device result.
func (h *Handlers) BatchDeviceCommand(c *gin.Context) {
	projectID, valid := parseID(c, "id")
	if !valid {
		return
	}
	if !h.requireProject(c, projectID, models.ProjectRoleMember) {
		return
	}
	var req batchDeviceRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		fail(c, http.StatusBadRequest, err.Error())
		return
	}
	if req.GroupID != 0 {
		var group models.DeviceGroup
		if err := h.DB.WithContext(c).Where("id = ? AND project_id = ?", req.GroupID, projectID).First(&group).Error; err != nil {
			fail(c, http.StatusBadRequest, "device group not found in this project")
			return
		}
		var members []models.DeviceGroupMember
		h.DB.WithContext(c).Where("group_id = ?", req.GroupID).Find(&members)
		for _, m := range members {
			req.DeviceIDs = append(req.DeviceIDs, m.DeviceID)
		}
	}
	if len(req.DeviceIDs) == 0 {
		fail(c, http.StatusBadRequest, "deviceIds is required")
		return
	}
	if req.Action == "" {
		req.Action = "command"
	}
	if req.Action == "set_desired" && len(req.Desired) == 0 {
		fail(c, http.StatusBadRequest, "desired is required for set_desired")
		return
	}

	kind := access.KindProperty
	switch req.Kind {
	case "", "property":
		kind = access.KindProperty
	case "service", "service_call":
		kind = access.KindServiceCall
	case "peer":
		kind = access.KindPeer
	default:
		kind = access.UplinkKind(req.Kind)
	}

	type result struct {
		DeviceID uint   `json:"deviceId"`
		OK       bool   `json:"ok"`
		Error    string `json:"error,omitempty"`
	}
	results := make([]result, 0, len(req.DeviceIDs))
	succeeded, failed := 0, 0
	for _, id := range req.DeviceIDs {
		var device models.Device
		if err := h.DB.WithContext(c).Where("id = ? AND project_id = ?", id, projectID).First(&device).Error; err != nil {
			results = append(results, result{DeviceID: id, OK: false, Error: "device not in project"})
			failed++
			continue
		}
		var err error
		if req.Action == "set_desired" {
			dc, rerr := h.Resolver.ResolveByDeviceID(c, id)
			if rerr != nil {
				results = append(results, result{DeviceID: id, OK: false, Error: rerr.Error()})
				failed++
				continue
			}
			_, err = h.Shadow.ApplyDesired(c, dc, req.Desired, models.ShadowSourceAPI, nil)
		} else {
			err = h.Downlink.Send(c, id, kind, req.Identifier, req.Payload, map[string]string{"source": "api-batch"})
		}
		if err != nil {
			results = append(results, result{DeviceID: id, OK: false, Error: err.Error()})
			failed++
			continue
		}
		results = append(results, result{DeviceID: id, OK: true})
		succeeded++
	}
	ok(c, gin.H{"total": len(req.DeviceIDs), "succeeded": succeeded, "failed": failed, "results": results})
}

// DeviceCommand sends a downlink command (property set or service call).
func (h *Handlers) DeviceCommand(c *gin.Context) {
	id, valid := parseID(c, "id")
	if !valid {
		return
	}
	device, allowed := h.loadDevice(c, id, models.ProjectRoleMember)
	if !allowed {
		return
	}
	var req commandRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		fail(c, http.StatusBadRequest, err.Error())
		return
	}
	kind := access.KindProperty
	switch req.Kind {
	case "", "property":
		kind = access.KindProperty
	case "service", "service_call":
		kind = access.KindServiceCall
	case "peer":
		kind = access.KindPeer
	default:
		kind = access.UplinkKind(req.Kind)
	}
	if err := h.Downlink.Send(c, device.ID, kind, req.Identifier, req.Payload, map[string]string{"source": "api"}); err != nil {
		fail(c, http.StatusBadGateway, err.Error())
		return
	}
	ok(c, gin.H{"ok": true})
}

type peerRequest struct {
	Target    string          `json:"target"`
	Broadcast bool            `json:"broadcast"`
	Payload   json.RawMessage `json:"payload"`
}

// DevicePeer lets an operator (or the UI) inject a workspace peer message.
func (h *Handlers) DevicePeer(c *gin.Context) {
	id, valid := parseID(c, "id")
	if !valid {
		return
	}
	device, allowed := h.loadDevice(c, id, models.ProjectRoleMember)
	if !allowed {
		return
	}
	var req peerRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		fail(c, http.StatusBadRequest, err.Error())
		return
	}
	meta := map[string]string{"from": device.Key, "source": "api"}
	if req.Broadcast {
		sent, err := h.Downlink.BroadcastWorkspace(c, device.WorkspaceID, device.ID, access.KindPeer, "", req.Payload, meta)
		if err != nil {
			fail(c, http.StatusBadGateway, err.Error())
			return
		}
		ok(c, gin.H{"sent": sent})
		return
	}
	target, err := h.Downlink.ResolveDeviceKey(c, device.WorkspaceID, req.Target)
	if err != nil {
		fail(c, http.StatusNotFound, err.Error())
		return
	}
	if err := h.Downlink.SendTo(c, target, access.KindPeer, "", req.Payload, meta); err != nil {
		fail(c, http.StatusBadGateway, err.Error())
		return
	}
	ok(c, gin.H{"sent": 1})
}

func parseTime(raw string, def time.Time) time.Time {
	if raw == "" {
		return def
	}
	if t, err := time.Parse(time.RFC3339, raw); err == nil {
		return t
	}
	return def
}

func generateKey(prefix, name string) string {
	base := strings.ToLower(strings.TrimSpace(name))
	base = strings.Map(func(r rune) rune {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			return r
		}
		if r == ' ' || r == '-' || r == '_' {
			return '-'
		}
		return -1
	}, base)
	base = strings.Trim(base, "-")
	// Non-ASCII names (e.g. Chinese) collapse to nothing; fall back to the
	// type prefix. Also avoid keys that start with a digit.
	if base == "" {
		base = prefix
	} else if base[0] >= '0' && base[0] <= '9' {
		base = prefix + "-" + base
	}
	if len(base) > 48 {
		base = base[:48]
	}
	b := make([]byte, 4)
	if _, err := rand.Read(b); err != nil {
		return base + "-" + hex.EncodeToString([]byte(time.Now().Format("150405.000000")))
	}
	return base + "-" + hex.EncodeToString(b)
}
