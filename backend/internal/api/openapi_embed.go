package api

import (
	_ "embed"
	"net/http"

	"github.com/gin-gonic/gin"
)

//go:embed openapi.yaml
var openapiYAML []byte

// OpenAPISpec serves the OpenAPI 3.0 contract (also the source for the SDKs).
func (h *Handlers) OpenAPISpec(c *gin.Context) {
	c.Data(http.StatusOK, "application/yaml; charset=utf-8", openapiYAML)
}
