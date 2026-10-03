package opcua

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"time"
)

// Config is the JSON device table consumed by the gateway:
//
//	{
//	  "devices": [
//	    {
//	      "deviceKey": "cnc-line1",
//	      "endpoint": "opc.tcp://10.0.0.5:4840",
//	      "username": "svc", "password": "pw",   // optional UA user auth
//	      "pollEvery": "5s",
//	      "nodes": [
//	        {"identifier":"temp","nodeID":"ns=2;s=Temp","dataType":"double","scale":1},
//	        {"identifier":"running","nodeID":"ns=2;s=Machine.Running","dataType":"bool"}
//	      ]
//	    }
//	  ]
//	}
//
// Values are reported as thing-model property uplinks; identifiers must exist
// in the product thing model.
type Config struct {
	Devices []DeviceCfg `json:"devices"`
}

type DeviceCfg struct {
	DeviceKey string `json:"deviceKey"`
	// Secret is the platform device secret (required when set, mirrors Modbus).
	Secret    string    `json:"secret,omitempty"`
	Endpoint  string    `json:"endpoint"` // opc.tcp://host:port
	Username  string    `json:"username,omitempty"`
	Password  string    `json:"password,omitempty"`
	PollEvery string    `json:"pollEvery"`
	Nodes     []NodeCfg `json:"nodes"`
}

type NodeCfg struct {
	Identifier string  `json:"identifier"`
	NodeID     string  `json:"nodeID"`   // UA node id, e.g. ns=2;s=Temp
	DataType   string  `json:"dataType"` // double|int|bool|string (default double)
	Scale      float64 `json:"scale"`
}

func (n NodeCfg) scale() float64 {
	if n.Scale == 0 {
		return 1
	}
	return n.Scale
}

// LoadConfig reads and validates the device table.
func LoadConfig(path string) (*Config, error) {
	if path == "" {
		return nil, errors.New("opcua config file not set (OPCUA_CONFIG_FILE)")
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read opcua config: %w", err)
	}
	var cfg Config
	if err := json.Unmarshal(raw, &cfg); err != nil {
		return nil, fmt.Errorf("parse opcua config: %w", err)
	}
	if len(cfg.Devices) == 0 {
		return nil, errors.New("opcua config has no devices")
	}
	for i := range cfg.Devices {
		d := &cfg.Devices[i]
		if d.DeviceKey == "" || d.Endpoint == "" {
			return nil, fmt.Errorf("device %d: deviceKey/endpoint required", i)
		}
		if d.PollEvery == "" {
			d.PollEvery = "5s"
		}
		if _, err := time.ParseDuration(d.PollEvery); err != nil {
			return nil, fmt.Errorf("device %s: pollEvery: %w", d.DeviceKey, err)
		}
		if len(d.Nodes) == 0 {
			return nil, fmt.Errorf("device %s: no nodes", d.DeviceKey)
		}
		for j := range d.Nodes {
			n := &d.Nodes[j]
			if n.Identifier == "" || n.NodeID == "" {
				return nil, fmt.Errorf("device %s: node %d needs identifier + nodeID", d.DeviceKey, j)
			}
			switch n.DataType {
			case "", "double", "int", "bool", "string":
			default:
				return nil, fmt.Errorf("device %s: node %s: unsupported dataType %q", d.DeviceKey, n.Identifier, n.DataType)
			}
		}
	}
	return &cfg, nil
}
