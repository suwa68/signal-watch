package main

import (
	"encoding/json"
	"strings"
	"testing"
)

func configFixture() map[string]any {
	env := map[string]string{"HTTP_PROXY": "", "HTTPS_PROXY": "", "ALL_PROXY": "", "http_proxy": "", "https_proxy": "", "all_proxy": "", "NO_PROXY": "*", "no_proxy": "*", "SIGNALWATCH_TELEGRAM_BOT_TOKEN": token, "SIGNALWATCH_TELEGRAM_CHAT_ID": chat, "SSL_CERT_FILE": "/acceptance-ca/ca.pem"}
	return map[string]any{
		"networks": map[string]any{"default": map[string]any{"internal": true}},
		"services": map[string]any{
			"monitor": map[string]any{"networks": map[string]any{"default": nil}, "environment": env, "read_only": true, "init": true, "user": "10001:10001", "restart": "no", "stop_grace_period": "15s", "volumes": []any{map[string]any{"type": "volume", "source": "monitor-state", "target": "/state"}, map[string]any{"type": "bind", "target": "/sources", "read_only": true}, map[string]any{"type": "volume", "target": "/acceptance-ca", "read_only": true}}},
			"fixture": map[string]any{"networks": map[string]any{"default": map[string]any{"aliases": []string{"api.telegram.org"}}}, "environment": env, "volumes": []any{map[string]any{"target": "/ca"}}},
			"inspect": map[string]any{"network_mode": "none", "volumes": []any{map[string]any{"source": "monitor-state", "target": "/state", "read_only": true}}},
		},
	}
}
func TestConfigurationRejectsEscapeAndMissingTLSAlias(t *testing.T) {
	cases := []struct {
		name   string
		change func(map[string]any)
	}{
		{"external network", func(m map[string]any) { m["networks"].(map[string]any)["default"] = map[string]any{"internal": false} }},
		{"missing TLS alias", func(m map[string]any) {
			m["services"].(map[string]any)["fixture"].(map[string]any)["networks"] = map[string]any{"default": nil}
		}},
		{"production entrypoint override", func(m map[string]any) {
			m["services"].(map[string]any)["monitor"].(map[string]any)["entrypoint"] = []string{"/fake"}
		}},
		{"proxy injection", func(m map[string]any) {
			m["services"].(map[string]any)["monitor"].(map[string]any)["environment"].(map[string]string)["HTTPS_PROXY"] = "http://proxy.invalid"
		}},
	}
	base, _ := json.Marshal(configFixture())
	if err := validateConfig(strings.NewReader(string(base))); err != nil {
		t.Fatal(err)
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			m := configFixture()
			tc.change(m)
			b, _ := json.Marshal(m)
			if validateConfig(strings.NewReader(string(b))) == nil {
				t.Fatal("unsafe resolved configuration accepted")
			}
		})
	}
}
