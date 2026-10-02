package middleware

import (
	"strconv"
	"strings"

	"github.com/aiiot/server/internal/models"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// Audit records mutating API requests into audit_logs after the handler runs.
// Device ingest and auth endpoints are excluded to avoid noise.
func Audit(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		method := c.Request.Method
		path := c.Request.URL.Path
		if method == "GET" || method == "HEAD" || method == "OPTIONS" ||
			strings.HasPrefix(path, "/api/v1/ingest") ||
			strings.HasPrefix(path, "/api/v1/auth/") ||
			strings.HasPrefix(path, "/internal/") {
			c.Next()
			return
		}
		c.Next()

		row := &models.AuditLog{
			UserID:    UserID(c),
			Username:  Username(c),
			ProjectID: projectIDFromPath(path),
			Method:    method,
			Path:      path,
			Status:    c.Writer.Status(),
			IP:        c.ClientIP(),
		}
		if err := db.Create(row).Error; err != nil {
			_ = err // auditing must never break the request
		}
	}
}

// projectIDFromPath extracts the id from paths like /api/v1/projects/12/...
func projectIDFromPath(path string) uint {
	parts := strings.Split(strings.Trim(path, "/"), "/")
	for i, p := range parts {
		if p == "projects" && i+1 < len(parts) {
			if n, err := strconv.ParseUint(parts[i+1], 10, 64); err == nil {
				return uint(n)
			}
		}
	}
	return 0
}
