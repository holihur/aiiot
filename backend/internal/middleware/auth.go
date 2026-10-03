package middleware

import (
	"net/http"
	"strings"

	"github.com/aiiot/server/internal/auth"
	"github.com/aiiot/server/internal/models"
	"github.com/gin-gonic/gin"
)

const (
	CtxUserID     = "ctx_user_id"
	CtxUsername   = "ctx_username"
	CtxSystemRole = "ctx_system_role"
)

// Auth validates the bearer token and stores user info in the context.
func Auth(tokens *auth.TokenService, versions *TokenVersionCache) gin.HandlerFunc {
	return func(c *gin.Context) {
		header := c.GetHeader("Authorization")
		token := ""
		if strings.HasPrefix(header, "Bearer ") {
			token = strings.TrimPrefix(header, "Bearer ")
		} else if t := c.Query("token"); t != "" {
			token = t
		}
		if token == "" {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "missing token"})
			return
		}
		claims, err := tokens.Parse(token)
		if err != nil {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "invalid token"})
			return
		}
		// Admin tokens are a separate audience and cannot access business APIs.
		if claims.Kind == auth.TokenKindAdmin {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "admin token not allowed here"})
			return
		}
		if !versions.OK(claims.Kind, claims.UserID, claims.TokenVersion) {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "token revoked"})
			return
		}
		c.Set(CtxUserID, claims.UserID)
		c.Set(CtxUsername, claims.Username)
		c.Set(CtxSystemRole, claims.SystemRole)
		c.Next()
	}
}

func UserID(c *gin.Context) uint {
	if v, ok := c.Get(CtxUserID); ok {
		if id, ok := v.(uint); ok {
			return id
		}
	}
	return 0
}

func SystemRole(c *gin.Context) string {
	if v, ok := c.Get(CtxSystemRole); ok {
		if r, ok := v.(string); ok {
			return r
		}
	}
	return ""
}

func Username(c *gin.Context) string {
	if v, ok := c.Get(CtxUsername); ok {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return ""
}

// AdminAuth authenticates the administration audience only. Business user
// tokens are rejected here just as admin tokens are rejected by Auth.
func AdminAuth(tokens *auth.TokenService, versions *TokenVersionCache) gin.HandlerFunc {
	return func(c *gin.Context) {
		header := c.GetHeader("Authorization")
		token := ""
		if strings.HasPrefix(header, "Bearer ") {
			token = strings.TrimPrefix(header, "Bearer ")
		} else if t := c.Query("token"); t != "" {
			token = t
		}
		if token == "" {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "missing admin token"})
			return
		}
		claims, err := tokens.Parse(token)
		if err != nil || claims.Kind != auth.TokenKindAdmin {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "invalid admin token"})
			return
		}
		if !versions.OK(claims.Kind, claims.UserID, claims.TokenVersion) {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "token revoked"})
			return
		}
		c.Set(CtxUserID, claims.UserID)
		c.Set(CtxUsername, claims.Username)
		c.Set(CtxSystemRole, models.SystemRoleAdmin)
		c.Next()
	}
}

// CORS adopts a permissive development policy when allowed is empty and env
// is not production (reflects any origin with credentials). In production an
// explicit allow-list is required for credentialed cross-origin requests;
// without one, CORS headers are omitted so the SPA is expected to be served
// from the same origin.
func CORS(allowed []string, appEnv string) gin.HandlerFunc {
	prod := appEnv == "production"
	return func(c *gin.Context) {
		origin := c.GetHeader("Origin")
		if origin == "" {
			c.Next()
			return
		}
		allow := false
		if len(allowed) > 0 {
			for _, a := range allowed {
				if a == origin {
					allow = true
					break
				}
			}
		} else {
			allow = !prod
		}
		if !allow {
			c.Next()
			return
		}
		c.Header("Access-Control-Allow-Origin", origin)
		c.Header("Access-Control-Allow-Credentials", "true")
		c.Header("Access-Control-Allow-Headers", "Authorization,Content-Type,X-Requested-With,X-Ingest-Key")
		c.Header("Access-Control-Allow-Methods", "GET,POST,PUT,PATCH,DELETE,OPTIONS")
		if c.Request.Method == http.MethodOptions {
			c.AbortWithStatus(http.StatusNoContent)
			return
		}
		c.Next()
	}
}
