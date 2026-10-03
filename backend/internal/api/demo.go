package api

import (
	"context"
	"encoding/json"
	"math/rand"
	"net/http"
	"sync"
	"time"

	"github.com/aiiot/server/internal/access"
	"github.com/aiiot/server/internal/middleware"
	"github.com/aiiot/server/internal/models"
	"github.com/gin-gonic/gin"
)

// DemoKey is the project key reserved for the one-click demo.
const DemoKey = "quickstart"

// demoSimulators keeps per-project simulator goroutines alive for the life of
// the process (idempotent across repeated setup calls).
var (
	demoOnce   sync.Once
	demoSims   = map[uint]context.CancelFunc{}
	demoSimsMu sync.Mutex
)

// SetupDemo creates a self-contained demo project: product + thing model,
// three simulated devices reporting temperature/humidity every few seconds, a
// threshold rule and a dashboard. Repeated calls return the existing project.
func (h *Handlers) SetupDemo(c *gin.Context) {
	uid := middleware.UserID(c)
	var existing models.Project
	if err := h.DB.Where("key = ?", DemoKey).First(&existing).Error; err == nil {
		ok(c, gin.H{"projectId": existing.ID, "name": existing.Name, "already": true})
		return
	}

	project := models.Project{
		Key:         DemoKey,
		Name:        "一键演示 · Quickstart",
		Description: "自动生成的演示项目：3 台模拟设备持续上报温度/湿度，超温触发告警，看板实时展示。",
		OwnerID:     uid,
	}
	if err := h.DB.Create(&project).Error; err != nil {
		fail(c, http.StatusInternalServerError, err.Error())
		return
	}
	member := models.ProjectMember{ProjectID: project.ID, UserID: uid, Role: models.ProjectRoleOwner}
	_ = h.DB.Create(&member)

	ws := models.Workspace{ProjectID: project.ID, Key: "room1", Name: "车间 1"}
	if err := h.DB.Create(&ws).Error; err != nil {
		fail(c, http.StatusInternalServerError, err.Error())
		return
	}

	product := models.Product{
		Key: "demo-sensor", Name: "演示温湿度传感器", Protocol: "mqtt",
		DataFormat: "json", Status: models.ProductStatusDraft,
		CreatedBy: uid,
	}
	if err := h.DB.Create(&product).Error; err != nil {
		fail(c, http.StatusInternalServerError, err.Error())
		return
	}

	// thing model + publish directly
	tm := models.ThingModel{ProductID: product.ID, Version: "1.0.0", Status: models.ThingModelPublished}
	if err := h.DB.Create(&tm).Error; err != nil {
		fail(c, http.StatusInternalServerError, err.Error())
		return
	}
	elements := []models.ThingModelElement{
		{ThingModelID: tm.ID, Type: models.ElementProperty, Identifier: "temperature", Name: "温度",
			DataType: "double", AccessMode: "rw", Unit: "°C"},
		{ThingModelID: tm.ID, Type: models.ElementProperty, Identifier: "humidity", Name: "湿度",
			DataType: "double", AccessMode: "ro", Unit: "%RH"},
	}
	for i := range elements {
		if err := h.DB.Create(&elements[i]).Error; err != nil {
			fail(c, http.StatusInternalServerError, err.Error())
			return
		}
	}

	// three simulated devices with dynamic coordinates
	coords := [][2]float64{{31.2304, 121.4737}, {31.2404, 121.4637}, {31.2204, 121.4837}}
	var devs []dev
	for i := 1; i <= 3; i++ {
		dv := models.Device{
			ProjectID: project.ID, WorkspaceID: ws.ID, ProductID: product.ID,
			Key: "sim-" + itoa(i), Name: "模拟温湿度 " + itoa(i),
			Status: models.DeviceStatusEnabled, Secret: newSecret(),
			Tags: models.JSONMap{"lat": coords[i-1][0], "lng": coords[i-1][1]},
		}
		if err := h.DB.Create(&dv).Error; err != nil {
			fail(c, http.StatusInternalServerError, err.Error())
			return
		}
		devs = append(devs, dev{key: dv.Key, name: dv.Name, secret: dv.Secret, id: dv.ID, tags: dv.Tags})
	}

	// threshold rule: temperature > 30 -> warning alert
	if err := h.DB.Create(&models.Rule{
		ProjectID: project.ID, Name: "超温告警（演示）", Enabled: true,
		TriggerType: models.TriggerTelemetry, TriggerSource: "temperature",
		Condition: "value > 30", Priority: 1,
	}).Error; err != nil {
		fail(c, http.StatusInternalServerError, err.Error())
		return
	}
	if h.Rules != nil {
		_ = h.Rules.Reload(c)
	}
	// notification webhook not wired (no external URL) — alerts are stored and
	// visible in the Alerts tab; rules demo the lifecycle.

	// dashboard: device count + one trend panel
	panels := []map[string]any{
		{"type": "devices", "config": map[string]any{}},
		{"type": "trend", "title": "温度趋势",
			"config": map[string]any{"deviceId": devs[0].id, "deviceIds": []uint{devs[0].id, devs[1].id, devs[2].id},
				"identifiers": []string{"temperature", "humidity"}, "range": "1h"}},
	}
	if err := h.DB.Create(&models.DashboardBoard{ProjectID: project.ID, Panels: panels}).Error; err != nil {
		fail(c, http.StatusInternalServerError, err.Error())
		return
	}

	// start the simulator once per project
	h.startDemoSimulator(project.ID, devs)

	out := gin.H{
		"projectId": project.ID, "productId": product.ID, "already": false,
		"devices": func() []gin.H {
			var list []gin.H
			for _, d := range devs {
				list = append(list, gin.H{"deviceId": d.id, "key": d.key, "name": d.name, "secret": d.secret})
			}
			return list
		}(),
	}
	ok(c, out)
}

// dev is one demo device descriptor.
type dev struct {
	key, name, secret string
	id                uint
	tags              models.JSONMap
}

// RestoreDemo resumes the demo simulator after a core restart: it locates the
// quickstart project and its sim-* devices and (re)starts reporting.
func (h *Handlers) RestoreDemo(ctx context.Context) {
	var project models.Project
	if err := h.DB.WithContext(ctx).Where("key = ?", DemoKey).First(&project).Error; err != nil {
		return
	}
	var devs []dev
	var rows []models.Device
	if err := h.DB.WithContext(ctx).Where("key LIKE 'sim-%' AND project_id = ?", project.ID).Find(&rows).Error; err != nil {
		return
	}
	for _, r := range rows {
		devs = append(devs, dev{key: r.Key, name: r.Name, secret: r.Secret, id: r.ID, tags: r.Tags})
	}
	if len(devs) == 0 {
		return
	}
	h.startDemoSimulator(project.ID, devs)
	h.Log.Info("demo simulator restored", "project", project.ID, "devices", len(devs))
}

// startDemoSimulator reports temperature/humidity for the demo devices every
// 5 seconds through the real ingest chain (dedup, rules, shadow, SSE).
func (h *Handlers) startDemoSimulator(projectID uint, devs []dev) {
	demoSimsMu.Lock()
	defer demoSimsMu.Unlock()
	if _, running := demoSims[projectID]; running {
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	demoSims[projectID] = cancel

	ingest := h.Ingest
	go func() {
		defer cancel()
		for i := 0; ; i++ {
			select {
			case <-ctx.Done():
				return
			case <-time.After(5 * time.Second):
			}
			now := time.Now().UTC()
			for devIdx := range devs {
				d := devs[devIdx]
				// temperature drifts 25..38, occasionally above the 30°C rule
				temp := 25 + rand.Float64()*13
				hum := 40 + rand.Float64()*30
				payload, _ := json.Marshal(map[string]float64{"temperature": temp, "humidity": hum})
				msg := &access.UplinkMessage{
					Protocol:   access.ProtocolMQTTName,
					Device:     access.DeviceRef{ProductKey: "demo-sensor", DeviceKey: d.key},
					Kind:       access.KindProperty,
					Identifier: "temperature",
					Payload:    payload,
					Timestamp:  now,
					Metadata:   map[string]string{},
				}
				if err := ingest.Handle(context.Background(), msg); err != nil {
					h.Log.Warn("demo simulator ingest failed", "device", d.key, "error", err)
				}
			}
		}
	}()
}

func itoa(n int) string {
	switch n {
	case 1:
		return "1"
	case 2:
		return "2"
	case 3:
		return "3"
	}
	return "0"
}
