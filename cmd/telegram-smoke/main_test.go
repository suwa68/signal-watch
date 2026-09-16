package main

import (
	"strings"
	"testing"
)

func TestSmokeRequiresBothEnvironmentVariables(t *testing.T) {
	for _, tt := range []struct {
		name   string
		token  string
		chatID string
	}{
		{"neither set", "", ""},
		{"token only", "test-token", ""},
		{"chat only", "", "@test_channel"},
		{"blank token", " \t", "@test_channel"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("SIGNALWATCH_TELEGRAM_BOT_TOKEN", tt.token)
			t.Setenv("SIGNALWATCH_TELEGRAM_CHAT_ID", tt.chatID)
			err := run()
			if err == nil || !strings.Contains(err.Error(), "SIGNALWATCH_TELEGRAM_BOT_TOKEN") || !strings.Contains(err.Error(), "SIGNALWATCH_TELEGRAM_CHAT_ID") {
				t.Fatalf("run() error = %v, want required environment variables", err)
			}
		})
	}
}
