package mqtt

import (
	"fmt"
	"strings"
)

// Topic scheme (minimal; no namespace prefix):
//
//	{productKey}/{deviceKey}/{suffix}
//
// The workspace/tenant is resolved at connection time from the access
// subdomain (TLS SNI / WebSocket Host), NOT from the topic. Cross-device
// traffic is mediated by the core, which delivers peer messages into the
// target device's own subtree.
//
// Uplink suffixes:
//
//	properties/post                       batch property report
//	properties/{identifier}               single property report
//	events/{identifier}/post               event report
//	services/{identifier}/reply            service reply
//	peer/{targetDeviceKey}                 directed peer message
//	peer/broadcast                         workspace broadcast
//	lifecycle                              online/offline transition
//
// Downlink suffixes:
//
//	properties/set                         set one or more properties
//	services/{identifier}/call             invoke a service
//	peer/{sourceDeviceKey}                 forwarded peer message
//	ota/upgrade                            firmware upgrade command
type topicInfo struct {
	ProductKey string
	DeviceKey  string
	Suffix     []string
	Wildcard   bool
}

func parseTopic(topic string) (*topicInfo, error) {
	// Reserved ($SYS/...) and empty topics are not device topics.
	if topic == "" || strings.HasPrefix(topic, "$") {
		return nil, fmt.Errorf("invalid topic")
	}
	parts := strings.Split(topic, "/")
	if len(parts) < 2 {
		return nil, fmt.Errorf("topic too short")
	}
	info := &topicInfo{}
	for _, p := range parts {
		if p == "+" || p == "#" {
			info.Wildcard = true
		}
	}
	info.ProductKey = parts[0]
	info.DeviceKey = parts[1]
	info.Suffix = parts[2:]
	return info, nil
}

func joinTopic(device DeviceTopic, suffix string) string {
	base := fmt.Sprintf("%s/%s", device.ProductKey, device.DeviceKey)
	if suffix == "" {
		return base
	}
	return base + "/" + suffix
}

// DeviceTopic is the addressing portion of a device topic.
type DeviceTopic struct {
	ProductKey string
	DeviceKey  string
}
