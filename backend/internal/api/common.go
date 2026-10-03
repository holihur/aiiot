package api

import (
	"log/slog"
	"net/http"
	"strconv"

	"github.com/aiiot/server/internal/auth"
	"github.com/aiiot/server/internal/bus"
	"github.com/aiiot/server/internal/certs"
	"github.com/aiiot/server/internal/gateway"
	"github.com/aiiot/server/internal/middleware"
	"github.com/aiiot/server/internal/models"
	"github.com/aiiot/server/internal/service"
	"github.com/aiiot/server/internal/totp"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// Handlers holds all dependencies used by the HTTP layer.
type Handlers struct {
	DB           *gorm.DB
	Tokens       *auth.TokenService
	Resolver     *service.Resolver
	Telemetry    *service.TelemetryService
	Ingest       *service.IngestService
	Rules        *service.RuleEngine
	Shadow       *service.ShadowService
	OTA          *service.OTAService
	Notifier     *service.Notifier
	Settings     *service.Settings
	Hub          *service.Hub
	Downlink     *service.DownlinkService
	Registry     *gateway.Registry
	GatewayToken string
	Certs        *certs.Manager // device X.509; nil disables cert APIs
	TOTP         *totp.Cipher   // admin 2FA; nil disables the endpoints
	Geofence     *service.GeofenceService
	// PublicHost is advertised to devices in connection instructions.
	PublicHost string
	// RetentionDefault is the configured telemetry retention when no runtime
	// override is stored in system_settings.
	RetentionDefault int
	Log              *slog.Logger
	// Bus is the NATS uplink subscriber; used by the admin NATS ops page.
	Bus *bus.Subscriber
	// NATSSubject is the uplink subject, used to break out bus counters.
	NATSSubject string
	// CORSAllowed is the explicit cross-origin allow-list (empty = dev policy).
	CORSAllowed []string
	// AppEnv selects dev/production behaviour (CORS, secrets).
	AppEnv string
}

// dbTx is a short alias used inside transaction closures.
type dbTx = *gorm.DB

var roleRank = map[string]int{
	models.ProjectRoleViewer: 1,
	models.ProjectRoleMember: 2,
	models.ProjectRoleAdmin:  3,
	models.ProjectRoleOwner:  4,
}

func ok(c *gin.Context, data any) {
	c.JSON(http.StatusOK, data)
}

func created(c *gin.Context, data any) {
	c.JSON(http.StatusCreated, data)
}

func fail(c *gin.Context, status int, msg string) {
	c.AbortWithStatusJSON(status, gin.H{"error": msg})
}

func parseID(c *gin.Context, name string) (uint, bool) {
	raw := c.Param(name)
	n, err := strconv.ParseUint(raw, 10, 64)
	if err != nil || n == 0 {
		fail(c, http.StatusBadRequest, "invalid id "+name)
		return 0, false
	}
	return uint(n), true
}

func queryUint(c *gin.Context, name string) uint {
	n, err := strconv.ParseUint(c.Query(name), 10, 64)
	if err != nil {
		return 0
	}
	return uint(n)
}

// projectRole resolves the caller's effective role in a project.
func (h *Handlers) projectRole(c *gin.Context, projectID uint) (string, bool) {
	uid := middleware.UserID(c)
	var project models.Project
	if err := h.DB.WithContext(c).First(&project, projectID).Error; err != nil {
		return "", false
	}
	if middleware.SystemRole(c) == models.SystemRoleAdmin || project.OwnerID == uid {
		return models.ProjectRoleOwner, true
	}
	var m models.ProjectMember
	if err := h.DB.WithContext(c).Where("project_id = ? AND user_id = ?", projectID, uid).First(&m).Error; err != nil {
		return "", false
	}
	return m.Role, true
}

func (h *Handlers) requireProject(c *gin.Context, projectID uint, min string) bool {
	role, allow := h.projectRole(c, projectID)
	if !allow || roleRank[role] < roleRank[min] {
		fail(c, http.StatusForbidden, "insufficient project permissions")
		return false
	}
	return true
}

// requireSystemAdmin gates platform-wide operations (e.g. dropping storage
// partitions) to system administrators.
func (h *Handlers) requireSystemAdmin(c *gin.Context) bool {
	if middleware.SystemRole(c) != models.SystemRoleAdmin {
		fail(c, http.StatusForbidden, "system administrator required")
		return false
	}
	return true
}

// loadProduct fetches a global product. Products are not scoped to projects;
// authorization for project-owned resources is enforced separately.
func (h *Handlers) loadProduct(c *gin.Context, id uint) (*models.Product, bool) {
	var p models.Product
	if err := h.DB.WithContext(c).First(&p, id).Error; err != nil {
		fail(c, http.StatusNotFound, "product not found")
		return nil, false
	}
	return &p, true
}

// loadProductManage loads a product and enforces management permission: the
// product creator or a system admin may edit/delete it; everyone else (any
// logged-in user may view) gets 403.
func (h *Handlers) loadProductManage(c *gin.Context, id uint) (*models.Product, bool) {
	p, allowed := h.loadProduct(c, id)
	if !allowed {
		return nil, false
	}
	if middleware.SystemRole(c) == models.SystemRoleAdmin || p.CreatedBy == middleware.UserID(c) {
		return p, true
	}
	fail(c, http.StatusForbidden, "no permission to manage this product")
	return nil, false
}

func (h *Handlers) loadDevice(c *gin.Context, id uint, min string) (*models.Device, bool) {
	var d models.Device
	if err := h.DB.WithContext(c).First(&d, id).Error; err != nil {
		fail(c, http.StatusNotFound, "device not found")
		return nil, false
	}
	if !h.requireProject(c, d.ProjectID, min) {
		return nil, false
	}
	return &d, true
}

func (h *Handlers) loadWorkspace(c *gin.Context, id uint, min string) (*models.Workspace, bool) {
	var w models.Workspace
	if err := h.DB.WithContext(c).First(&w, id).Error; err != nil {
		fail(c, http.StatusNotFound, "workspace not found")
		return nil, false
	}
	if !h.requireProject(c, w.ProjectID, min) {
		return nil, false
	}
	return &w, true
}

func (h *Handlers) loadRule(c *gin.Context, id uint, min string) (*models.Rule, bool) {
	var r models.Rule
	if err := h.DB.WithContext(c).First(&r, id).Error; err != nil {
		fail(c, http.StatusNotFound, "rule not found")
		return nil, false
	}
	if !h.requireProject(c, r.ProjectID, min) {
		return nil, false
	}
	return &r, true
}
