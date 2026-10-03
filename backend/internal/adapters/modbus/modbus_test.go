package modbus

import (
	"math"
	"testing"
	"time"
)

func TestDecodeTypes(t *testing.T) {
	cases := []struct {
		name  string
		reg   RegCfg
		words []uint16
		want  float64
	}{
		{"uint16", RegCfg{Type: "uint16"}, []uint16{400}, 400},
		{"int16-negative", RegCfg{Type: "int16"}, []uint16{0xFFFB}, -5},
		{"bool-on", RegCfg{Type: "bool"}, []uint16{1}, 1},
		{"bool-off", RegCfg{Type: "bool"}, []uint16{0}, 0},
		{"uint32", RegCfg{Type: "uint32"}, []uint16{0x0012, 0x0034}, 0x00120034},
		{"uint32-high", RegCfg{Type: "uint32"}, []uint16{0x1234, 0x5678}, 0x12345678},
		{"int32", RegCfg{Type: "int32"}, []uint16{0xFFFF, 0xFFFB}, -5},
		{"float32 1.0", RegCfg{Type: "float32"}, []uint16{0x3F80, 0x0000}, 1},
		{"float32 -2.5", RegCfg{Type: "float32"}, []uint16{uint16(math.Float32bits(-2.5) >> 16), uint16(math.Float32bits(-2.5))}, -2.5},
		{"scale", RegCfg{Type: "int16", Scale: 0.1}, []uint16{255}, 25.5},
	}
	for _, c := range cases {
		got, err := c.reg.decode(c.words)
		if err != nil {
			t.Fatalf("%s: %v", c.name, err)
		}
		if math.Abs(got-c.want) > 1e-6 {
			t.Errorf("%s: got %v want %v", c.name, got, c.want)
		}
	}
}

func TestDecodeInsufficientWords(t *testing.T) {
	if _, err := (RegCfg{Type: "float32"}).decode([]uint16{1}); err == nil {
		t.Fatal("float32 with 1 word must fail")
	}
}

func TestCountPerType(t *testing.T) {
	if (RegCfg{Type: "int16"}).count() != 1 || (RegCfg{Type: "float32"}).count() != 2 {
		t.Fatal("count mismatch")
	}
}

// TestFrameLayout verifies wire-level MBAP framing through a fake server.
func TestClientTransaction(t *testing.T) {
	fake := newFakeModbusServer(t, func(req []byte) []byte {
		// req: txn(2) proto(2)=0 len(2) unit(1) fn(1) ...
		if len(req) < 7 {
			return nil
		}
		unit := req[6]
		fn := req[7]
		_ = unit
		if fn == fnReadHolding {
			// return two registers 0x0102 0x0304 (PDU: fn, count, data...)
			return []byte{fn, 4, 1, 2, 3, 4}
		}
		if fn == fnWriteSingle {
			return []byte{fn, req[8], req[9], req[10], req[11]}
		}
		return []byte{fn | 0x80, excIllegalFunction}
	}, 8)
	defer fake.Close()

	c, err := Dial(fake.host, fake.port, 2*time.Second)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer c.Close()

	regs, err := c.ReadRegisters(1, fnReadHolding, 0, 2)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if len(regs) != 2 || regs[0] != 0x0102 || regs[1] != 0x0304 {
		t.Fatalf("bad registers: %v", regs)
	}

	if err := c.WriteSingle(1, 10, 0xABCD); err != nil {
		t.Fatalf("write: %v", err)
	}

	_, err = c.ReadRegisters(1, fnReadInput, 0, 1)
	if err == nil {
		t.Fatal("expected exception error")
	}
}

func TestWriteMultiFrame(t *testing.T) {
	fake := newFakeModbusServer(t, func(req []byte) []byte {
		unit, fn := req[6], req[7]
		_ = unit
		if fn == fnWriteMulti {
			return []byte{fn, req[8], req[9], req[10], req[11]}
		}
		return nil
	}, 11)
	defer fake.Close()
	c, err := Dial(fake.host, fake.port, 2*time.Second)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer c.Close()
	if err := c.WriteMultiple(2, 20, []uint16{1, 2, 3}); err != nil {
		t.Fatalf("write multi: %v", err)
	}
}
