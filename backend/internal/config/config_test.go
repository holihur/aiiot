package config

import "testing"

func TestValidateProductionRejectsDefaultSecrets(t *testing.T) {
	cfg := &Config{AppEnv: "production", JWTSecret: DefaultJWTSecret, Gateway: GatewayServerConfig{Token: DefaultGatewayToken}}
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected error for default secrets in production")
	}
}

func TestValidateProductionRejectsShortJWT(t *testing.T) {
	cfg := &Config{AppEnv: "production", JWTSecret: "short", Gateway: GatewayServerConfig{Token: "a-long-enough-token-..."}}
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected error for short JWT secret")
	}
}

func TestValidateProductionAcceptsStrongSecrets(t *testing.T) {
	cfg := &Config{
		AppEnv:    "production",
		JWTSecret: "0123456789abcdef0123456789abcdef",             // 32 chars
		Gateway:   GatewayServerConfig{Token: "0123456789abcdef"}, // 16 chars
	}
	if err := cfg.Validate(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestValidateDevelopmentIsLenient(t *testing.T) {
	cfg := &Config{AppEnv: "development", JWTSecret: "", Gateway: GatewayServerConfig{}}
	if err := cfg.Validate(); err != nil {
		t.Fatalf("development must not be blocked by secret checks: %v", err)
	}
}

func TestSplitList(t *testing.T) {
	got := splitList(" https://a.com , https://b.com ,")
	if len(got) != 2 || got[0] != "https://a.com" || got[1] != "https://b.com" {
		t.Fatalf("unexpected split: %v", got)
	}
}
