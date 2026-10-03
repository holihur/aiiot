package coap

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"

	piondtls "github.com/pion/dtls/v3"
	dtls "github.com/plgd-dev/go-coap/v3/dtls"
	coapNet "github.com/plgd-dev/go-coap/v3/net"
	"github.com/plgd-dev/go-coap/v3/options"
)

// serveDTLS runs the DTLS-PSK listener. Devices present their deviceKey as the
// PSK identity and their secret as the pre-shared key; the handshake proves
// possession of the secret, while application-layer authentication (the
// ?secret= query) still runs for every uplink, so the channel is encrypted and
// identity is verified end to end.
func (a *Adapter) serveDTLS() error {
	cfg := &piondtls.Config{
		PSK:          a.pskFor,
		CipherSuites: []piondtls.CipherSuiteID{piondtls.TLS_PSK_WITH_AES_128_CCM_8},
	}
	l, err := coapNet.NewDTLSListener("udp", a.opts.DTLSPSKAddr, cfg)
	if err != nil {
		return fmt.Errorf("dtls listener: %w", err)
	}
	srv := dtls.NewServer(options.WithMux(a.router()))
	defer srv.Stop()
	return srv.Serve(l)
}

// pskFor answers the DTLS handshake: identity (deviceKey) -> PSK (secret
// bytes). The secret is fetched from the core via the gateway HTTP token.
func (a *Adapter) pskFor(identity []byte) ([]byte, error) {
	key := string(identity)
	secret, err := a.deviceSecret(key)
	if err != nil {
		return nil, err
	}
	if raw, err := hex.DecodeString(secret); err == nil {
		return raw, nil
	}
	return []byte(secret), nil
}

// pskCache caches deviceKey -> secret with a TTL.
var pskCache sync.Map // deviceKey -> cachedSecret
type cachedSecret struct {
	secret    string
	expiresAt time.Time
}

func (a *Adapter) deviceSecret(deviceKey string) (string, error) {
	if v, ok := pskCache.Load(deviceKey); ok {
		c := v.(cachedSecret)
		if time.Now().Before(c.expiresAt) {
			return c.secret, nil
		}
		pskCache.Delete(deviceKey)
	}
	secret, err := a.fetchPSK(deviceKey)
	if err != nil {
		return "", err
	}
	pskCache.Store(deviceKey, cachedSecret{secret: secret, expiresAt: time.Now().Add(5 * time.Minute)})
	return secret, nil
}

func (a *Adapter) fetchPSK(deviceKey string) (string, error) {
	url := a.opts.CoreURL + "/internal/gateway/psk?deviceKey=" + deviceKey
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("X-Gateway-Token", a.opts.GatewayToken)
	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("fetch psk: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		raw, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return "", fmt.Errorf("fetch psk status %d: %s", resp.StatusCode, string(raw))
	}
	var out struct {
		PSK string `json:"psk"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return "", err
	}
	return out.PSK, nil
}
