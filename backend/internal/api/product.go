package api

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/aiiot/server/internal/middleware"
	"github.com/aiiot/server/internal/models"
	"github.com/gin-gonic/gin"
)

type productRequest struct {
	// Key is optional: when empty a stable key is generated from the name.
	Key         string `json:"key" binding:"omitempty,min=2,max=64"`
	Name        string `json:"name" binding:"required,max=160"`
	Category    string `json:"category"`
	Protocol    string `json:"protocol" binding:"required,oneof=mqtt coap custom modbus"`
	DataFormat  string `json:"dataFormat"`
	Description string `json:"description"`
	Status      string `json:"status"`
}

func (h *Handlers) ListProducts(c *gin.Context) {
	q := h.DB.WithContext(c).Order("name")
	if kw := strings.TrimSpace(c.Query("keyword")); kw != "" {
		q = q.Where("name ILIKE ? OR key ILIKE ?", "%"+kw+"%", "%"+kw+"%")
	}
	if protocol := c.Query("protocol"); protocol != "" {
		q = q.Where("protocol = ?", protocol)
	}
	var products []models.Product
	if err := q.Find(&products).Error; err != nil {
		fail(c, http.StatusInternalServerError, err.Error())
		return
	}
	ok(c, products)
}

func (h *Handlers) CreateProduct(c *gin.Context) {
	var req productRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		fail(c, http.StatusBadRequest, err.Error())
		return
	}
	req.Key = strings.ToLower(strings.TrimSpace(req.Key))
	if req.DataFormat == "" {
		req.DataFormat = "json"
	}
	if req.Status == "" {
		req.Status = models.ProductStatusDraft
	}
	// Auto-generate the key when the user did not supply one.
	if req.Key == "" {
		for i := 0; i < 5; i++ {
			req.Key = generateKey("prod", req.Name)
			var n int64
			h.DB.Model(&models.Product{}).Where("key = ?", req.Key).Count(&n)
			if n == 0 {
				break
			}
		}
	} else {
		var existing int64
		h.DB.Model(&models.Product{}).Where("key = ?", req.Key).Count(&existing)
		if existing > 0 {
			fail(c, http.StatusConflict, "product key already exists")
			return
		}
	}
	product := models.Product{
		Key:         req.Key,
		Name:        req.Name,
		Category:    req.Category,
		Protocol:    req.Protocol,
		DataFormat:  req.DataFormat,
		Description: req.Description,
		Status:      req.Status,
		CreatedBy:   middleware.UserID(c),
	}
	err := h.DB.Transaction(func(tx dbTx) error {
		if err := tx.Create(&product).Error; err != nil {
			return err
		}
		return tx.Create(&models.ThingModel{ProductID: product.ID, Version: "1.0.0"}).Error
	})
	if err != nil {
		fail(c, http.StatusInternalServerError, err.Error())
		return
	}
	created(c, product)
}

func (h *Handlers) GetProduct(c *gin.Context) {
	id, valid := parseID(c, "id")
	if !valid {
		return
	}
	product, allowed := h.loadProduct(c, id)
	if !allowed {
		return
	}
	var tm models.ThingModel
	if err := h.DB.WithContext(c).Where("product_id = ?", product.ID).First(&tm).Error; err == nil {
		var elements []models.ThingModelElement
		h.DB.WithContext(c).Where("thing_model_id = ?", tm.ID).Order("type, identifier").Find(&elements)
		tm.Elements = elements
		product.ThingModel = &tm
	}
	ok(c, product)
}

func (h *Handlers) UpdateProduct(c *gin.Context) {
	id, valid := parseID(c, "id")
	if !valid {
		return
	}
	product, allowed := h.loadProductManage(c, id)
	if !allowed {
		return
	}
	var req productRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		fail(c, http.StatusBadRequest, err.Error())
		return
	}
	product.Name = req.Name
	product.Category = req.Category
	product.Description = req.Description
	if req.Protocol != "" {
		product.Protocol = req.Protocol
	}
	if req.DataFormat != "" {
		product.DataFormat = req.DataFormat
	}
	if req.Status != "" {
		product.Status = req.Status
	}
	if err := h.DB.Save(product).Error; err != nil {
		fail(c, http.StatusInternalServerError, err.Error())
		return
	}
	ok(c, product)
}

func (h *Handlers) DeleteProduct(c *gin.Context) {
	id, valid := parseID(c, "id")
	if !valid {
		return
	}
	product, allowed := h.loadProductManage(c, id)
	if !allowed {
		return
	}
	var deviceCount int64
	h.DB.Model(&models.Device{}).Where("product_id = ?", product.ID).Count(&deviceCount)
	if deviceCount > 0 {
		fail(c, http.StatusConflict, "product still has devices")
		return
	}
	// Delete the thing model (and its elements) first — products reference
	// thing_models via a foreign key.
	if err := h.DB.Transaction(func(tx dbTx) error {
		tx.Exec("DELETE FROM thing_model_elements WHERE thing_model_id IN (SELECT id FROM thing_models WHERE product_id = ?)", product.ID)
		if err := tx.Where("product_id = ?", product.ID).Delete(&models.ThingModel{}).Error; err != nil {
			return err
		}
		return tx.Delete(&models.Product{}, product.ID).Error
	}); err != nil {
		fail(c, http.StatusInternalServerError, err.Error())
		return
	}
	ok(c, gin.H{"ok": true})
}

// --- thing model ----------------------------------------------------------

type thingModelRequest struct {
	Version     string                     `json:"version"`
	Description string                     `json:"description"`
	Elements    []models.ThingModelElement `json:"elements"`
}

func (h *Handlers) GetThingModel(c *gin.Context) {
	id, valid := parseID(c, "id")
	if !valid {
		return
	}
	product, allowed := h.loadProduct(c, id)
	if !allowed {
		return
	}
	var tm models.ThingModel
	if err := h.DB.WithContext(c).Where("product_id = ?", product.ID).First(&tm).Error; err != nil {
		tm = models.ThingModel{ProductID: product.ID, Version: "1.0.0"}
		h.DB.Create(&tm)
	}
	var elements []models.ThingModelElement
	h.DB.WithContext(c).Where("thing_model_id = ?", tm.ID).Order("type, identifier").Find(&elements)
	tm.Elements = elements
	ok(c, tm)
}

// PutThingModel replaces the full element set (used by the visual editor).
func (h *Handlers) PutThingModel(c *gin.Context) {
	id, valid := parseID(c, "id")
	if !valid {
		return
	}
	product, allowed := h.loadProductManage(c, id)
	if !allowed {
		return
	}
	var req thingModelRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		fail(c, http.StatusBadRequest, err.Error())
		return
	}
	var tm models.ThingModel
	if err := h.DB.WithContext(c).Where("product_id = ?", product.ID).First(&tm).Error; err != nil {
		tm = models.ThingModel{ProductID: product.ID}
		h.DB.Create(&tm)
	}
	if req.Version != "" {
		tm.Version = req.Version
	}
	tm.Description = req.Description

	err := h.DB.Transaction(func(tx dbTx) error {
		if err := tx.Save(&tm).Error; err != nil {
			return err
		}
		if err := tx.Where("thing_model_id = ?", tm.ID).Delete(&models.ThingModelElement{}).Error; err != nil {
			return err
		}
		for i := range req.Elements {
			el := req.Elements[i]
			el.ID = 0
			el.ThingModelID = tm.ID
			if el.AccessMode == "" && el.Type == models.ElementProperty {
				el.AccessMode = models.AccessReadWrite
			}
			if err := tx.Create(&el).Error; err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		fail(c, http.StatusInternalServerError, err.Error())
		return
	}
	tm.Elements = req.Elements
	ok(c, tm)
}

type elementRequest struct {
	Type        string         `json:"type" binding:"required,oneof=property service event"`
	Identifier  string         `json:"identifier" binding:"required,min=1,max=128"`
	Name        string         `json:"name"`
	DataType    string         `json:"dataType" binding:"required"`
	EventType   string         `json:"eventType"` // info | alarm | fault (events)
	AccessMode  string         `json:"accessMode"`
	Unit        string         `json:"unit"`
	Min         *float64       `json:"min"`
	Max         *float64       `json:"max"`
	Step        *float64       `json:"step"`
	Specs       models.JSONMap `json:"specs"`
	Required    bool           `json:"required"`
	Description string         `json:"description"`
}

func (h *Handlers) CreateElement(c *gin.Context) {
	id, valid := parseID(c, "id")
	if !valid {
		return
	}
	product, allowed := h.loadProductManage(c, id)
	if !allowed {
		return
	}
	var req elementRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		fail(c, http.StatusBadRequest, err.Error())
		return
	}
	var tm models.ThingModel
	if err := h.DB.WithContext(c).Where("product_id = ?", product.ID).First(&tm).Error; err != nil {
		tm = models.ThingModel{ProductID: product.ID, Version: "1.0.0"}
		h.DB.Create(&tm)
	}
	el := models.ThingModelElement{
		ThingModelID: tm.ID,
		Type:         req.Type,
		Identifier:   strings.TrimSpace(req.Identifier),
		Name:         req.Name,
		DataType:     req.DataType,
		EventType:    req.EventType,
		AccessMode:   req.AccessMode,
		Unit:         req.Unit,
		Min:          req.Min,
		Max:          req.Max,
		Step:         req.Step,
		Specs:        req.Specs,
		Required:     req.Required,
		Description:  req.Description,
	}
	if el.AccessMode == "" && el.Type == models.ElementProperty {
		el.AccessMode = models.AccessReadWrite
	}
	if err := h.DB.Create(&el).Error; err != nil {
		fail(c, http.StatusInternalServerError, err.Error())
		return
	}
	created(c, el)
}

func (h *Handlers) UpdateElement(c *gin.Context) {
	productID, valid := parseID(c, "id")
	if !valid {
		return
	}
	elementID, valid := parseID(c, "elementId")
	if !valid {
		return
	}
	product, allowed := h.loadProductManage(c, productID)
	if !allowed {
		return
	}
	var req elementRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		fail(c, http.StatusBadRequest, err.Error())
		return
	}
	var tm models.ThingModel
	if err := h.DB.WithContext(c).Where("product_id = ?", product.ID).First(&tm).Error; err != nil {
		fail(c, http.StatusNotFound, "thing model not found")
		return
	}
	var el models.ThingModelElement
	if err := h.DB.WithContext(c).Where("id = ? AND thing_model_id = ?", elementID, tm.ID).First(&el).Error; err != nil {
		fail(c, http.StatusNotFound, "element not found")
		return
	}
	el.Type = req.Type
	el.Identifier = strings.TrimSpace(req.Identifier)
	el.Name = req.Name
	el.DataType = req.DataType
	el.EventType = req.EventType
	el.AccessMode = req.AccessMode
	el.Unit = req.Unit
	el.Min = req.Min
	el.Max = req.Max
	el.Step = req.Step
	el.Specs = req.Specs
	el.Required = req.Required
	el.Description = req.Description
	if err := h.DB.Save(&el).Error; err != nil {
		fail(c, http.StatusInternalServerError, err.Error())
		return
	}
	ok(c, el)
}

func (h *Handlers) DeleteElement(c *gin.Context) {
	productID, valid := parseID(c, "id")
	if !valid {
		return
	}
	elementID, valid := parseID(c, "elementId")
	if !valid {
		return
	}
	product, allowed := h.loadProductManage(c, productID)
	if !allowed {
		return
	}
	var tm models.ThingModel
	if err := h.DB.WithContext(c).Where("product_id = ?", product.ID).First(&tm).Error; err != nil {
		fail(c, http.StatusNotFound, "thing model not found")
		return
	}
	if err := h.DB.Where("id = ? AND thing_model_id = ?", elementID, tm.ID).Delete(&models.ThingModelElement{}).Error; err != nil {
		fail(c, http.StatusInternalServerError, err.Error())
		return
	}
	ok(c, gin.H{"ok": true})
}

// --- thing-model lifecycle: publish / versions / rollback ------------------

// PublishThingModel validates that the thing model has at least one property,
// one service and one event, snapshots it, and marks it published.
func (h *Handlers) PublishThingModel(c *gin.Context) {
	productID, valid := parseID(c, "id")
	if !valid {
		return
	}
	product, allowed := h.loadProductManage(c, productID)
	if !allowed {
		return
	}
	var tm models.ThingModel
	if err := h.DB.WithContext(c).Where("product_id = ?", product.ID).First(&tm).Error; err != nil {
		fail(c, http.StatusNotFound, "thing model not found")
		return
	}
	var els []models.ThingModelElement
	h.DB.WithContext(c).Where("thing_model_id = ?", tm.ID).Find(&els)
	if len(els) == 0 {
		fail(c, http.StatusBadRequest, "thing model unusable: add at least one element (property, service or event) before publishing")
		return
	}
	payload, err := json.Marshal(els)
	if err != nil {
		fail(c, http.StatusInternalServerError, err.Error())
		return
	}
	var lst models.JSONList
	if err := json.Unmarshal(payload, &lst); err != nil {
		fail(c, http.StatusInternalServerError, err.Error())
		return
	}
	now := time.Now().UTC()
	snapshot := models.ThingModelVersion{
		ThingModelID: tm.ID,
		Version:      tm.Version,
		Payload:      lst,
		CreatedBy:    middleware.UserID(c),
		PublishedAt:  now,
	}
	if err := h.DB.WithContext(c).Transaction(func(tx dbTx) error {
		if err := tx.Create(&snapshot).Error; err != nil {
			return err
		}
		return tx.Model(&tm).Update("status", models.ThingModelPublished).Error
	}); err != nil {
		fail(c, http.StatusInternalServerError, err.Error())
		return
	}
	h.Log.Info("thing model published", "product", product.Key, "version", tm.Version, "elements", len(els))
	ok(c, gin.H{"ok": true, "version": tm.Version, "status": models.ThingModelPublished, "snapshotId": snapshot.ID})
}

// ListThingModelVersions returns the published snapshots of a thing model.
func (h *Handlers) ListThingModelVersions(c *gin.Context) {
	productID, valid := parseID(c, "id")
	if !valid {
		return
	}
	if _, allowed := h.loadProduct(c, productID); !allowed {
		return
	}
	var tm models.ThingModel
	if err := h.DB.WithContext(c).Where("product_id = ?", productID).First(&tm).Error; err != nil {
		fail(c, http.StatusNotFound, "thing model not found")
		return
	}
	var vs []struct {
		ID          uint      `json:"id"`
		Version     string    `json:"version"`
		CreatedBy   uint      `json:"createdBy"`
		PublishedAt time.Time `json:"publishedAt"`
	}
	h.DB.WithContext(c).Model(&models.ThingModelVersion{}).
		Select("id, version, created_by, published_at").
		Where("thing_model_id = ?", tm.ID).
		Order("published_at DESC").Scan(&vs)
	ok(c, vs)
}

// RollbackThingModel restores a published snapshot into the current thing
// model and returns the model to draft (it must be published again).
func (h *Handlers) RollbackThingModel(c *gin.Context) {
	productID, valid := parseID(c, "id")
	if !valid {
		return
	}
	snapshotID, valid := parseID(c, "versionId")
	if !valid {
		return
	}
	product, allowed := h.loadProductManage(c, productID)
	if !allowed {
		return
	}
	var tm models.ThingModel
	if err := h.DB.WithContext(c).Where("product_id = ?", product.ID).First(&tm).Error; err != nil {
		fail(c, http.StatusNotFound, "thing model not found")
		return
	}
	var snap models.ThingModelVersion
	if err := h.DB.WithContext(c).Where("id = ? AND thing_model_id = ?", snapshotID, tm.ID).First(&snap).Error; err != nil {
		fail(c, http.StatusNotFound, "snapshot not found")
		return
	}
	els := make([]models.ThingModelElement, 0, len(snap.Payload))
	for _, m := range snap.Payload {
		var el models.ThingModelElement
		b, err := json.Marshal(m)
		if err != nil {
			fail(c, http.StatusInternalServerError, "corrupt snapshot: "+err.Error())
			return
		}
		if err := json.Unmarshal(b, &el); err != nil {
			fail(c, http.StatusInternalServerError, "corrupt snapshot: "+err.Error())
			return
		}
		els = append(els, el)
	}
	err := h.DB.WithContext(c).Transaction(func(tx dbTx) error {
		if err := tx.Where("thing_model_id = ?", tm.ID).Delete(&models.ThingModelElement{}).Error; err != nil {
			return err
		}
		for i := range els {
			els[i].ID = 0
			els[i].ThingModelID = tm.ID
			if err := tx.Create(&els[i]).Error; err != nil {
				return err
			}
		}
		return tx.Model(&tm).Updates(map[string]any{
			"status":  models.ThingModelDraft,
			"version": snap.Version,
		}).Error
	})
	if err != nil {
		fail(c, http.StatusInternalServerError, err.Error())
		return
	}
	h.Log.Info("thing model rolled back", "product", product.Key, "version", snap.Version, "elements", len(els))
	ok(c, gin.H{"ok": true, "elements": len(els)})
}
