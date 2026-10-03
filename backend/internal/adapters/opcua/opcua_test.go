package opcua

import (
	"math"
	"os"
	"path/filepath"
	"testing"
)

func TestLoadConfigValidAndErrors(t *testing.T) {
	dir := t.TempDir()
	good := `{"devices":[{"deviceKey":"a","endpoint":"opc.tcp://h:4840","pollEvery":"5s","nodes":[{"identifier":"t","nodeID":"ns=2;s=T"}]}]}`
	p := filepath.Join(dir, "good.json")
	if err := os.WriteFile(p, []byte(good), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, err := LoadConfig(p)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if len(cfg.Devices) != 1 || cfg.Devices[0].Nodes[0].Identifier != "t" {
		t.Fatal("bad parse")
	}
	// missing endpoint
	bad := `{"devices":[{"deviceKey":"a","nodes":[]}]}`
	pb := filepath.Join(dir, "bad.json")
	_ = os.WriteFile(pb, []byte(bad), 0o600)
	if _, err := LoadConfig(pb); err == nil {
		t.Fatal("missing endpoint must fail")
	}
	// no devices
	if _, err := LoadConfig(filepath.Join(dir, "empty.json")); err == nil {
		t.Fatal("empty config must fail")
	}
}

func TestCoerce(t *testing.T) {
	cases := []struct {
		node NodeCfg
		val  any
		want float64
		ok   bool
	}{
		{NodeCfg{DataType: "double"}, float64(42.5), 42.5, true},
		{NodeCfg{DataType: "double"}, int64(7), 7, true},
		{NodeCfg{DataType: "int"}, int32(-3), -3, true},
		{NodeCfg{DataType: "bool"}, true, 1, true},
		{NodeCfg{DataType: "bool"}, false, 0, true},
		{NodeCfg{DataType: "string"}, "12.5", 12.5, true},
		{NodeCfg{DataType: "string"}, "abc", 0, false},
		{NodeCfg{DataType: "double", Scale: 0.1}, float64(100), 10, true},
	}
	for _, c := range cases {
		got, ok := coerce(c.node, c.val)
		if ok != c.ok || (ok && math.Abs(got-c.want) > 1e-9) {
			t.Errorf("coerce(%v, %v) = %v,%v want %v,%v", c.node.DataType, c.val, got, ok, c.want, c.ok)
		}
	}
}
