package api

import (
	"net/http"

	"github.com/aiiot/server/internal/access"
	"github.com/aiiot/server/internal/models"
	"github.com/gin-gonic/gin"
)

type ruleRequest struct {
	Name          string           `json:"name" binding:"required,max=160"`
	Description   string           `json:"description"`
	ProductID     *uint            `json:"productId"`
	WorkspaceID   *uint            `json:"workspaceId"`
	Enabled       *bool            `json:"enabled"`
	TriggerType   string           `json:"triggerType" binding:"required"`
	TriggerSource string           `json:"triggerSource"`
	Condition     string           `json:"condition" binding:"required"`
	Actions       models.JSONList  `json:"actions"`
	Priority      int              `json:"priority"`
}

func (h *Handlers) ListRules(c *gin.Context) {
	projectID, valid := parseID(c, "id")
	if !valid {
		return
	}
	if !h.requireProject(c, projectID, models.ProjectRoleViewer) {
		return
	}
	var rules []models.Rule
	if err := h.DB.WithContext(c).Where("project_id = ?", projectID).Order("priority DESC, id").Find(&rules).Error; err != nil {
		fail(c, http.StatusInternalServerError, err.Error())
		return
	}
	ok(c, rules)
}

func (h *Handlers) CreateRule(c *gin.Context) {
	projectID, valid := parseID(c, "id")
	if !valid {
		return
	}
	if !h.requireProject(c, projectID, models.ProjectRoleMember) {
		return
	}
	var req ruleRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		fail(c, http.StatusBadRequest, err.Error())
		return
	}
	if err := h.Rules.Validate(req.Condition); err != nil {
		fail(c, http.StatusBadRequest, "invalid CEL condition: "+err.Error())
		return
	}
	rule := models.Rule{
		ProjectID:     projectID,
		ProductID:     req.ProductID,
		WorkspaceID:   req.WorkspaceID,
		Name:          req.Name,
		Description:   req.Description,
		Enabled:       true,
		TriggerType:   req.TriggerType,
		TriggerSource: req.TriggerSource,
		Condition:     req.Condition,
		Actions:       req.Actions,
		Priority:      req.Priority,
	}
	if rule.TriggerSource == "" {
		rule.TriggerSource = "*"
	}
	if req.Enabled != nil {
		rule.Enabled = *req.Enabled
	}
	if err := h.DB.Create(&rule).Error; err != nil {
		fail(c, http.StatusInternalServerError, err.Error())
		return
	}
	_ = h.Rules.Reload(c)
	created(c, rule)
}

func (h *Handlers) GetRule(c *gin.Context) {
	id, valid := parseID(c, "id")
	if !valid {
		return
	}
	rule, allowed := h.loadRule(c, id, models.ProjectRoleViewer)
	if !allowed {
		return
	}
	ok(c, rule)
}

func (h *Handlers) UpdateRule(c *gin.Context) {
	id, valid := parseID(c, "id")
	if !valid {
		return
	}
	rule, allowed := h.loadRule(c, id, models.ProjectRoleMember)
	if !allowed {
		return
	}
	var req ruleRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		fail(c, http.StatusBadRequest, err.Error())
		return
	}
	if err := h.Rules.Validate(req.Condition); err != nil {
		fail(c, http.StatusBadRequest, "invalid CEL condition: "+err.Error())
		return
	}
	rule.Name = req.Name
	rule.Description = req.Description
	rule.ProductID = req.ProductID
	rule.WorkspaceID = req.WorkspaceID
	rule.TriggerType = req.TriggerType
	rule.TriggerSource = req.TriggerSource
	if rule.TriggerSource == "" {
		rule.TriggerSource = "*"
	}
	rule.Condition = req.Condition
	rule.Actions = req.Actions
	rule.Priority = req.Priority
	if req.Enabled != nil {
		rule.Enabled = *req.Enabled
	}
	if err := h.DB.Save(rule).Error; err != nil {
		fail(c, http.StatusInternalServerError, err.Error())
		return
	}
	_ = h.Rules.Reload(c)
	ok(c, rule)
}

func (h *Handlers) ToggleRule(c *gin.Context) {
	id, valid := parseID(c, "id")
	if !valid {
		return
	}
	rule, allowed := h.loadRule(c, id, models.ProjectRoleMember)
	if !allowed {
		return
	}
	rule.Enabled = !rule.Enabled
	if err := h.DB.Save(rule).Error; err != nil {
		fail(c, http.StatusInternalServerError, err.Error())
		return
	}
	_ = h.Rules.Reload(c)
	ok(c, rule)
}

func (h *Handlers) DeleteRule(c *gin.Context) {
	id, valid := parseID(c, "id")
	if !valid {
		return
	}
	rule, allowed := h.loadRule(c, id, models.ProjectRoleAdmin)
	if !allowed {
		return
	}
	if err := h.DB.Delete(&models.Rule{}, rule.ID).Error; err != nil {
		fail(c, http.StatusInternalServerError, err.Error())
		return
	}
	_ = h.Rules.Reload(c)
	ok(c, gin.H{"ok": true})
}

func (h *Handlers) RuleLogs(c *gin.Context) {
	id, valid := parseID(c, "id")
	if !valid {
		return
	}
	rule, allowed := h.loadRule(c, id, models.ProjectRoleViewer)
	if !allowed {
		return
	}
	var logs []models.RuleExecutionLog
	if err := h.DB.WithContext(c).Where("rule_id = ?", rule.ID).
		Order("occurred_at DESC").Limit(200).Find(&logs).Error; err != nil {
		fail(c, http.StatusInternalServerError, err.Error())
		return
	}
	ok(c, logs)
}

func (h *Handlers) ValidateRuleCondition(c *gin.Context) {
	var body struct {
		Condition string `json:"condition" binding:"required"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		fail(c, http.StatusBadRequest, err.Error())
		return
	}
	if err := h.Rules.Validate(body.Condition); err != nil {
		ok(c, gin.H{"valid": false, "error": err.Error()})
		return
	}
	ok(c, gin.H{"valid": true})
}

// RuleReference lists the CEL variables available to rules, for the UI.
func (h *Handlers) RuleReference(c *gin.Context) {
	ok(c, gin.H{
		"variables": []gin.H{
			{"name": "project_id", "type": "int", "description": "project id"},
			{"name": "workspace_id", "type": "int", "description": "workspace id"},
			{"name": "product_id", "type": "int", "description": "product id"},
			{"name": "device_id", "type": "int", "description": "device id"},
			{"name": "device_key", "type": "string", "description": "device key"},
			{"name": "identifier", "type": "string", "description": "property/event identifier"},
			{"name": "kind", "type": "string", "description": "property|event|service_reply|peer|lifecycle"},
			{"name": "state", "type": "string", "description": "online|offline for lifecycle events"},
			{"name": "value", "type": "dyn", "description": "reported value"},
			{"name": "params", "type": "map<string,dyn>", "description": "full params map"},
			{"name": "payload", "type": "dyn", "description": "raw decoded payload"},
			{"name": "now", "type": "timestamp", "description": "event time"},
		},
		"examples": []gin.H{
			{"name": "High temperature", "condition": `identifier == "temperature" && double(value) > 40.0`},
			{"name": "Device offline", "condition": `kind == "lifecycle" && state == "offline"`},
			{"name": "Low battery", "condition": `identifier == "battery" && double(value) < 15.0`},
		},
		"triggers": []string{
			models.TriggerTelemetry, models.TriggerEvent, models.TriggerDeviceOnline,
			models.TriggerDeviceOffline, models.TriggerPeerMessage,
		},
		"actions": []string{
			models.ActionWebhook, models.ActionMQTTPublish, models.ActionDeviceCommand, models.ActionLog,
			models.ActionSetDesired, "downlink",
		},
		"kinds": []string{string(access.KindProperty), string(access.KindServiceCall), string(access.KindPeer)},
	})
}
