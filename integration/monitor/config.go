package main

import (
	"encoding/json"
	"fmt"
	"io"
)

// Validate the resolved model rather than grepping YAML. Inherited networks,
// mounts, or environment interpolation must fail before runtime starts.
func validateConfig(r io.Reader) error {
	type volume struct {
		Type, Source, Target string
		ReadOnly             bool `json:"read_only"`
	}
	type service struct {
		Entrypoint      []string
		Command         []string
		Environment     map[string]string
		Networks        map[string]any
		Ports           []any
		Volumes         []volume
		ReadOnly        bool `json:"read_only"`
		Init            bool
		Restart         string
		User            string
		NetworkMode     string `json:"network_mode"`
		StopGracePeriod string `json:"stop_grace_period"`
	}
	var model struct {
		Services map[string]service
		Networks map[string]struct {
			Internal bool
			External bool
		}
	}
	if err := json.NewDecoder(r).Decode(&model); err != nil {
		return err
	}
	net, ok := model.Networks["default"]
	if !ok || !net.Internal || net.External || len(model.Networks) != 1 {
		return fmt.Errorf("acceptance network must be exclusively internal")
	}
	for _, name := range []string{"monitor", "fixture"} {
		s, ok := model.Services[name]
		if !ok || len(s.Networks) != 1 || !hasDefault(s.Networks) || s.NetworkMode != "" || len(s.Ports) != 0 {
			return fmt.Errorf("%s network/ports invalid", name)
		}
		for _, key := range []string{"HTTP_PROXY", "HTTPS_PROXY", "ALL_PROXY", "http_proxy", "https_proxy", "all_proxy"} {
			v, ok := s.Environment[key]
			if !ok || v != "" {
				return fmt.Errorf("%s proxy %s not explicitly empty", name, key)
			}
		}
		for _, v := range s.Volumes {
			if v.Target != "/state" && v.Target != "/sources" && v.Target != "/acceptance-ca" && v.Target != "/ca" {
				return fmt.Errorf("unexpected mount target %s", v.Target)
			}
		}
	}
	network, ok := model.Services["fixture"].Networks["default"].(map[string]any)
	if !ok {
		return fmt.Errorf("missing Telegram TLS alias")
	}
	aliases, ok := network["aliases"].([]any)
	if !ok || len(aliases) != 1 || aliases[0] != "api.telegram.org" {
		return fmt.Errorf("invalid Telegram TLS alias")
	}
	m := model.Services["monitor"]
	if len(m.Entrypoint) != 0 || len(m.Command) != 0 {
		return fmt.Errorf("production entrypoint/command must be unchanged")
	}
	if !m.ReadOnly || !m.Init || m.User != "10001:10001" || m.Restart != "no" || m.StopGracePeriod != "15s" {
		return fmt.Errorf("monitor security/lifecycle contract invalid")
	}
	if m.Environment["SIGNALWATCH_TELEGRAM_BOT_TOKEN"] != token || m.Environment["SIGNALWATCH_TELEGRAM_CHAT_ID"] != chat || m.Environment["SSL_CERT_FILE"] != "/acceptance-ca/ca.pem" {
		return fmt.Errorf("monitor synthetic credentials/CA invalid")
	}
	targets := map[string]volume{}
	for _, v := range m.Volumes {
		targets[v.Target] = v
	}
	if v := targets["/state"]; v.Type != "volume" || v.ReadOnly {
		return fmt.Errorf("state must be writable named volume")
	}
	for _, target := range []string{"/sources", "/acceptance-ca"} {
		if !targets[target].ReadOnly {
			return fmt.Errorf("%s must be read-only", target)
		}
	}
	i := model.Services["inspect"]
	if i.NetworkMode != "none" || len(i.Volumes) != 1 || !i.Volumes[0].ReadOnly || i.Volumes[0].Target != "/state" || i.Volumes[0].Source != targets["/state"].Source {
		return fmt.Errorf("inspector must be offline and read-only")
	}
	fmt.Println("PASS resolved isolated Compose configuration")
	return nil
}

func hasDefault(networks map[string]any) bool { _, ok := networks["default"]; return ok }
