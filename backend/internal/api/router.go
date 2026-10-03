package api

import (
	"net/http"
	"time"

	"github.com/aiiot/server/internal/metrics"
	"github.com/aiiot/server/internal/middleware"
	"github.com/gin-gonic/gin"
)

// NewRouter wires all HTTP routes.
func NewRouter(h *Handlers) *gin.Engine {
	loginRL := middleware.NewRateLimiter(time.Minute, 30)
	registerRL := middleware.NewRateLimiter(time.Minute, 10)
	r := gin.New()
	r.Use(gin.Recovery(), middleware.CORS(h.CORSAllowed, h.AppEnv), metrics.HTTP())
	r.MaxMultipartMemory = 8 << 20

	r.GET("/healthz", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})
	r.GET("/metrics", gin.WrapH(metrics.Handler()))

	// Firmware download for devices (must live at the root path referenced by
	// OTA_PUBLIC_BASE_URL, not under /api/v1, and before the SPA fallback).
	r.GET("/ota/firmware/:id", h.DownloadFirmware)

	api := r.Group("/api/v1")
	api.GET("/openapi.yaml", h.OpenAPISpec)
	api.POST("/auth/register", middleware.RateLimit(registerRL, middleware.ClientKey), h.Register)
	api.POST("/auth/login", middleware.RateLimit(loginRL, middleware.ClientKey), h.Login)
	api.POST("/ingest/:productKey/:deviceKey", h.HTTPIngest)

	auth := api.Group("", middleware.Auth(h.Tokens))
	auth.Use(middleware.Audit(h.DB))
	{
		auth.GET("/auth/me", h.Me)
		auth.POST("/auth/password", h.ChangePassword)

		auth.GET("/projects", h.ListProjects)
		auth.POST("/projects", h.CreateProject)
		auth.GET("/projects/:id", h.GetProject)
		auth.PUT("/projects/:id", h.UpdateProject)
		auth.DELETE("/projects/:id", h.DeleteProject)

		auth.GET("/projects/:id/members", h.ListMembers)
		auth.POST("/projects/:id/members", h.AddMember)
		auth.PUT("/projects/:id/members/:memberId", h.UpdateMember)
		auth.DELETE("/projects/:id/members/:memberId", h.RemoveMember)

		auth.GET("/projects/:id/workspaces", h.ListWorkspaces)
		auth.POST("/projects/:id/workspaces", h.CreateWorkspace)
		auth.PUT("/workspaces/:id", h.UpdateWorkspace)
		auth.DELETE("/workspaces/:id", h.DeleteWorkspace)
		auth.GET("/workspaces/:id/devices", h.ListWorkspaceDevices)

		auth.GET("/products", h.ListProducts)
		auth.POST("/products", h.CreateProduct)
		auth.GET("/products/:id", h.GetProduct)
		auth.PUT("/products/:id", h.UpdateProduct)
		auth.DELETE("/products/:id", h.DeleteProduct)

		auth.GET("/products/:id/thing-model", h.GetThingModel)
		auth.PUT("/products/:id/thing-model", h.PutThingModel)
		auth.POST("/products/:id/thing-model/publish", h.PublishThingModel)
		auth.GET("/products/:id/thing-model/versions", h.ListThingModelVersions)
		auth.POST("/products/:id/thing-model/versions/:versionId/rollback", h.RollbackThingModel)
		auth.POST("/products/:id/thing-model/elements", h.CreateElement)
		auth.PUT("/products/:id/thing-model/elements/:elementId", h.UpdateElement)
		auth.DELETE("/products/:id/thing-model/elements/:elementId", h.DeleteElement)

		auth.GET("/projects/:id/devices", h.ListProjectDevices)
		auth.POST("/projects/:id/devices/batch", h.BatchDeviceCommand)
		auth.GET("/projects/:id/device-groups", h.ListDeviceGroups)
		auth.POST("/projects/:id/device-groups", h.CreateDeviceGroup)
		auth.GET("/projects/:id/alerts", h.ListAlerts)
		auth.GET("/projects/:id/dashboard", h.GetDashboard)
		auth.PUT("/projects/:id/dashboard", h.PutDashboard)
		auth.POST("/alerts/:id/ack", h.AckAlert)
		auth.POST("/alerts/:id/resolve", h.ResolveAlert)
		auth.PUT("/device-groups/:id", h.UpdateDeviceGroup)
		auth.DELETE("/device-groups/:id", h.DeleteDeviceGroup)
		auth.GET("/device-groups/:id/devices", h.ListGroupDevices)
		auth.POST("/device-groups/:id/devices", h.AddGroupDevices)
		auth.DELETE("/device-groups/:id/devices/:deviceId", h.RemoveGroupDevice)
		auth.GET("/events/stream", h.EventStream)
		auth.POST("/products/:id/devices", h.CreateDevice)
		auth.POST("/products/:id/firmwares", h.UploadFirmware)
		auth.GET("/products/:id/firmwares", h.ListFirmwares)
		auth.DELETE("/firmwares/:id", h.DeleteFirmware)
		auth.POST("/projects/:id/ota-tasks", h.CreateOTATask)
		auth.GET("/projects/:id/ota-tasks", h.ListOTATasks)
		auth.GET("/ota-tasks/:id", h.GetOTATask)
		auth.POST("/ota-tasks/:id/cancel", h.CancelOTATask)
		auth.POST("/ota-tasks/:id/rollback", h.RollbackOTATask)
		auth.GET("/projects/:id/channels", h.ListChannels)
		auth.POST("/projects/:id/channels", h.CreateChannel)
		auth.PUT("/channels/:id", h.UpdateChannel)
		auth.DELETE("/channels/:id", h.DeleteChannel)
		auth.POST("/channels/:id/test", h.TestChannel)
		auth.GET("/channels/:id/logs", h.ChannelLogs)
		auth.GET("/devices/:id", h.GetDevice)
		auth.PUT("/devices/:id", h.UpdateDevice)
		auth.DELETE("/devices/:id", h.DeleteDevice)
		auth.GET("/devices/:id/certificate", h.DeviceCertificateStatus)
		auth.POST("/devices/:id/certificate", h.DeviceCertificate)
		auth.DELETE("/devices/:id/certificate", h.RevokeDeviceCertificate)
		auth.GET("/products/:id/devices/export", h.ExportDevices)
		auth.POST("/products/:id/devices/import", h.ImportDevices)
		auth.POST("/telemetry/compare", h.CompareTelemetry)
		auth.GET("/devices/:id/telemetry/export", h.ExportTelemetry)
		auth.PUT("/devices/:id/status", h.SetDeviceStatus)
		auth.POST("/devices/:id/secret", h.RotateDeviceSecret)
		auth.GET("/devices/:id/latest", h.DeviceLatest)
		auth.GET("/devices/:id/telemetry", h.DeviceTelemetry)
		auth.GET("/devices/:id/events", h.DeviceEvents)
		auth.GET("/devices/:id/downlinks", h.DeviceDownlinks)
		auth.GET("/devices/:id/timeline", h.DeviceTimeline)
		auth.POST("/devices/:id/command", h.DeviceCommand)
		auth.POST("/devices/:id/peer", h.DevicePeer)
		auth.GET("/devices/:id/shadow", h.GetDeviceShadow)
		auth.GET("/devices/:id/connection", h.DeviceConnection)
		auth.PATCH("/devices/:id/shadow/desired", h.PatchDeviceShadowDesired)
		auth.DELETE("/devices/:id/shadow/desired", h.ClearDeviceShadowDesired)
		auth.GET("/devices/:id/shadow/history", h.GetDeviceShadowHistory)

		auth.GET("/projects/:id/rules", h.ListRules)
		auth.POST("/projects/:id/rules", h.CreateRule)
		auth.POST("/rules/validate", h.ValidateRuleCondition)
		auth.GET("/rules/reference", h.RuleReference)
		auth.GET("/rules/:id", h.GetRule)
		auth.PUT("/rules/:id", h.UpdateRule)
		auth.DELETE("/rules/:id", h.DeleteRule)
		auth.POST("/rules/:id/toggle", h.ToggleRule)
		auth.GET("/rules/:id/logs", h.RuleLogs)

		// Administration endpoints live under /admin (see below).
	}

	// Administration area: isolated audience (admin token only).
	admin := api.Group("/admin")
	admin.POST("/auth/login", h.AdminLogin)
	admin.GET("/auth/totp/status", h.AdminTOTPStatus)
	admin.Use(middleware.AdminAuth(h.Tokens))
	admin.POST("/auth/totp/setup", h.AdminTOTPSetup)
	admin.POST("/auth/totp/enable", h.AdminTOTPEnable)
	admin.POST("/auth/totp/disable", h.AdminTOTPDisable)
	admin.Use(middleware.Audit(h.DB))
	admin.GET("/auth/me", h.AdminMe)
	admin.POST("/auth/password", h.AdminChangePassword)
	admin.GET("/gateways", h.ListGateways)
	admin.DELETE("/gateways/:instanceId", h.DeregisterGateway)
	admin.GET("/storage", h.GetStorage)
	admin.PUT("/storage/retention", h.UpdateRetention)
	admin.POST("/storage/rollup/refresh", h.RefreshRollup)
	admin.DELETE("/storage/partitions/:name", h.DropPartition)
	admin.GET("/nats-stats", h.AdminNATSStats)
	admin.GET("/audit-logs", h.ListAuditLogs)

	internal := r.Group("/internal/gateway", h.gatewayAuth())
	{
		internal.POST("/register", h.GatewayRegister)
		internal.POST("/heartbeat", h.GatewayHeartbeat)
		internal.POST("/auth", h.GatewayAuthenticate)
		internal.POST("/uplink", h.GatewayUplink)
		internal.GET("/psk", h.GatewayPSK)
	}

	return r
}
