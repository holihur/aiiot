package certs

import (
	"testing"
)

func TestLoadOrCreateAndReuse(t *testing.T) {
	dir := t.TempDir() + "/ca"
	m1, err := LoadOrCreate(dir)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	m2, err := LoadOrCreate(dir)
	if err != nil {
		t.Fatalf("reuse: %v", err)
	}
	if string(m1.CAPEM()) != string(m2.CAPEM()) {
		t.Fatal("CA must be stable across reloads")
	}
}

func TestSignAndVerify(t *testing.T) {
	m, err := LoadOrCreate(t.TempDir() + "/ca")
	if err != nil {
		t.Fatal(err)
	}
	certPEM, keyPEM, serial, err := m.SignDevice(7)
	if err != nil {
		t.Fatalf("sign: %v", err)
	}
	if len(certPEM) == 0 || len(keyPEM) == 0 || serial == "" {
		t.Fatal("empty material")
	}
	leaf, err := m.VerifyPeer(certPEM)
	if err != nil {
		t.Fatalf("verify: %v", err)
	}
	if leaf.Subject.CommonName != "dev-7" {
		t.Fatalf("cn=%q", leaf.Subject.CommonName)
	}
	id, ok := DeviceIDFromCN(leaf.Subject.CommonName)
	if !ok || id != 7 {
		t.Fatalf("cn parse = %d,%v", id, ok)
	}
}

func TestVerifyRejectsForeignCertificate(t *testing.T) {
	m, _ := LoadOrCreate(t.TempDir() + "/ca")
	other, err := LoadOrCreate(t.TempDir() + "/other")
	if err != nil {
		t.Fatal(err)
	}
	foreign, _, _, err := other.SignDevice(1)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := m.VerifyPeer(foreign); err == nil {
		t.Fatal("foreign CA certificate must fail verification")
	}
}

func TestDeviceIDFromCN(t *testing.T) {
	if _, ok := DeviceIDFromCN("dev-0"); ok {
		t.Fatal("dev-0 invalid")
	}
	if _, ok := DeviceIDFromCN("sensor-a"); ok {
		t.Fatal("non-dev CN invalid")
	}
}