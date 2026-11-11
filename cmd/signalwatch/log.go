package main

import (
	"encoding/json"
	"io"
	"os"
	"regexp"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"golang.org/x/sys/unix"
)

var requestURL = regexp.MustCompile(`(?i)https?://[^\s"'<>]+`)

type operationalLog struct {
	mu             sync.Mutex
	stdout, stderr io.Writer
	token          string
}

func newOperationalLog(stdout, stderr io.Writer, token string) *operationalLog {
	return &operationalLog{stdout: stdout, stderr: stderr, token: token}
}
func (l *operationalLog) safe(text string, limit int) string {
	if l.token != "" {
		text = strings.ReplaceAll(text, l.token, "[REDACTED]")
	}
	text = requestURL.ReplaceAllStringFunc(text, func(value string) string {
		lower := strings.ToLower(value)
		if strings.Contains(lower, "api.telegram.org") || strings.Contains(lower, "/bot") {
			return "[REDACTED TELEGRAM URL]"
		}
		return value
	})
	text = strings.ToValidUTF8(text, "?")
	if len(text) > limit {
		text = text[:limit]
		for !utf8.ValidString(text) {
			text = text[:len(text)-1]
		}
		text += "…"
	}
	return text
}
func (l *operationalLog) event(event, sourceID string, err error) {
	record := map[string]string{"event": event, "source_id": l.safe(sourceID, 128), "status": "success"}
	if err != nil {
		record["status"] = "error"
		record["error"] = l.safe(err.Error(), 512)
	}
	l.write(l.stdout, record)
}
func (l *operationalLog) write(out io.Writer, record map[string]string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	_ = json.NewEncoder(out).Encode(record)
}

func (l *operationalLog) finalFailure(message string) {
	l.finalWrite(func() {
		l.writeFinalRecord(l.stderr, map[string]string{"event": "fatal", "error": l.safe(message, 512)})
	})
}
func (l *operationalLog) finalShutdown() {
	// Global lifecycle record: no variable/secret fields to redact or truncate.
	l.finalWrite(func() {
		l.writeFinalRecord(l.stdout, map[string]string{"event": "shutdown", "status": "success"})
	})
}

func (l *operationalLog) writeFinalRecord(out io.Writer, record map[string]string) {
	if file, ok := out.(*os.File); ok {
		raw, err := file.SyscallConn()
		if err != nil {
			return
		}
		duplicate := -1
		controlErr := raw.Control(func(fd uintptr) {
			// By default Go terminates on EPIPE from File.Write on fd 1 or 2.
			// A duplicate above fd 2 reports EPIPE as an ordinary write error.
			if fd == 1 || fd == 2 {
				duplicate, err = unix.FcntlInt(fd, unix.F_DUPFD_CLOEXEC, 3)
			}
		})
		if duplicate >= 3 {
			duplicateFile := os.NewFile(uintptr(duplicate), file.Name())
			defer duplicateFile.Close()
			out = duplicateFile
		}
		if controlErr != nil || err != nil {
			return
		}
	}
	l.write(out, record)
}

// The final lifecycle record gets a short best-effort flush budget, including
// any wait on another write. main then exits the process.
func (l *operationalLog) finalWrite(write func()) {
	done := make(chan struct{})
	go func() { write(); close(done) }()
	timer := time.NewTimer(25 * time.Millisecond)
	defer timer.Stop()
	select {
	case <-done:
	case <-timer.C:
	}
}
