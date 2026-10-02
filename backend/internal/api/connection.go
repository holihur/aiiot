package api

import (
	"fmt"
	"net"
	"net/http"

	"github.com/aiiot/server/internal/models"
	"github.com/gin-gonic/gin"
)

// DeviceConnection returns everything a user needs to connect a device: the
// credential and the protocol-specific topics/resources for its product
// protocol. Restricted to project members because it reveals the secret.
func (h *Handlers) DeviceConnection(c *gin.Context) {
	id, valid := parseID(c, "id")
	if !valid {
		return
	}
	device, allowed := h.loadDevice(c, id, models.ProjectRoleMember)
	if !allowed {
		return
	}
	dc, err := h.Resolver.ResolveByDeviceID(c, device.ID)
	if err != nil {
		fail(c, http.StatusInternalServerError, err.Error())
		return
	}

	host := h.PublicHost
	if host == "" {
		host = c.Request.Host
		if hh, _, err := net.SplitHostPort(host); err == nil {
			host = hh
		}
	}

	ws := dc.WorkspaceKey
	dk := device.Key
	base := fmt.Sprintf("%s/%s", dc.Product.Key, dk)
	username := fmt.Sprintf("%s/%s", dc.Product.Key, dk)

	resp := gin.H{
		"protocol":   dc.Product.Protocol,
		"host":       host,
		"username":   username,
		"password":   device.Secret,
		"productKey": dc.Product.Key,
		"deviceKey":  dk,
		"workspace":  ws,
	}
	switch dc.Product.Protocol {
	case models.ProtocolMQTT:
		resp["mqtt"] = gin.H{
			"tcpPort":       1883,
			"wsPort":        8083,
			"uplinkTopic":   base + "/properties/post",
			"downlinkTopic": base + "/properties/set",
			"eventTopic":    base + "/events/{identifier}/post",
			"peerTopic":     base + "/peer/{targetDeviceKey}",
			"samplePayload": `{"temperature":25.4,"humidity":60}`,
		}
	case models.ProtocolCoAP:
		resp["coap"] = gin.H{
			"udpPort":        5683,
			"propertyPath":   fmt.Sprintf("/%s/%s/properties", dc.Product.Key, dk),
			"eventPath":      fmt.Sprintf("/%s/%s/events/{identifier}", dc.Product.Key, dk),
			"samplePayload":  `{"temperature":25.4}`,
			"secretQueryKey": "secret",
		}
	default: // custom protocol / TCP
		resp["custom"] = gin.H{
			"tcpPort": 9000,
			"authFrame": gin.H{
				"type": "auth", "productKey": dc.Product.Key, "deviceKey": dk, "secret": device.Secret,
			},
			"propertyFrame": gin.H{"type": "property", "params": gin.H{"temperature": 25.4}},
		}
	}
	ok(c, resp)
}
