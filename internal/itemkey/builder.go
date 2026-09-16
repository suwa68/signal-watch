// Package itemkey builds deterministic identities for normalized source items.
package itemkey

import (
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"fmt"
	"hash"
	"strings"
	"time"

	"github.com/suwa68/signal-watch/internal/source"
)

var ErrNoDeterministicIdentity = errors.New("no deterministic item identity")

// Build returns the highest-priority deterministic key available for item.
// Source scoping is intentionally handled by the state store rather than being
// encoded into the returned key.
func Build(item source.MonitorItem) (string, error) {
	if usable(item.ExternalID) {
		return "external:" + item.ExternalID, nil
	}
	if usable(item.URL) {
		return "url:" + item.URL, nil
	}
	if item.PublishedAt != nil && !item.PublishedAt.IsZero() && usable(item.Title) {
		return fmt.Sprintf(
			"published:%s:title:%s",
			item.PublishedAt.UTC().Format(time.RFC3339Nano),
			item.Title,
		), nil
	}
	if !usable(item.Title) && !usable(item.Content) {
		return "", ErrNoDeterministicIdentity
	}

	digest := sha256.New()
	writeHashComponent(digest, item.Title)
	writeHashComponent(digest, item.Content)
	return fmt.Sprintf("content:%x", digest.Sum(nil)), nil
}

func usable(value string) bool {
	return strings.TrimSpace(value) != ""
}

func writeHashComponent(digest hash.Hash, value string) {
	var length [8]byte
	binary.BigEndian.PutUint64(length[:], uint64(len(value)))
	_, _ = digest.Write(length[:])
	_, _ = digest.Write([]byte(value))
}
