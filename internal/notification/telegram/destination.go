// Package telegram renders and sends outbound Telegram notifications.
package telegram

// Destination identifies a Telegram chat or channel. ChatID is either a numeric
// identifier represented as a string or a supported @username.
type Destination struct {
	ID     string
	ChatID string
}
