package main

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"runtime"
	"strconv"
	"strings"
	"time"
)

type runLog struct {
	Event  string `json:"event"`
	Source string `json:"source_id"`
	Status string `json:"status"`
	Error  string `json:"error"`
}

func completedRunLogs(r io.Reader, source string) ([]runLog, error) {
	scanner := bufio.NewScanner(r)
	var runs []runLog
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		var entry runLog
		if err := json.Unmarshal([]byte(line), &entry); err != nil {
			return nil, err
		}
		if entry.Event == "source_run_completed" && entry.Source == source {
			runs = append(runs, entry)
		}
	}
	return runs, scanner.Err()
}
func completedRuns(r io.Reader, source string) (int, string, error) {
	runs, err := completedRunLogs(r, source)
	if err != nil {
		return 0, "", err
	}
	status := ""
	if len(runs) > 0 {
		status = runs[len(runs)-1].Status
	}
	return len(runs), status, nil
}

func command(args []string) error {
	if len(args) == 0 {
		return errors.New("subcommand required")
	}
	switch args[0] {
	case "serve":
		return serve()
	case "ready":
		return checkReady()
	case "ctl":
		if len(args) != 2 {
			return errors.New("ctl JSON required")
		}
		c := &http.Client{Timeout: 12 * time.Second, Transport: &http.Transport{Proxy: nil}}
		r, err := c.Post("http://127.0.0.1:9090/", "application/json", strings.NewReader(args[1]))
		if err != nil {
			return err
		}
		defer r.Body.Close()
		b, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
		if err != nil {
			return err
		}
		if r.StatusCode != 200 {
			return fmt.Errorf("control: %s", b)
		}
		_, err = os.Stdout.Write(b)
		return err
	case "log-count", "check-run":
		if len(args) < 2 {
			return errors.New("source required")
		}
		n, status, err := completedRuns(os.Stdin, args[1])
		if err != nil {
			return err
		}
		if args[0] == "log-count" {
			fmt.Println(n)
			return nil
		}
		if len(args) != 4 {
			return errors.New("source cursor status required")
		}
		before, err := strconv.Atoi(args[2])
		if err != nil {
			return err
		}
		if n <= before || status != args[3] {
			return errors.New("completion not yet observed")
		}
		return nil
	case "assert-shutdown":
		if len(args) != 3 {
			return errors.New("source and pre-signal completion cursor required")
		}
		before, err := strconv.Atoi(args[2])
		if err != nil {
			return err
		}
		return shutdownRun(os.Stdin, args[1], before)
	case "assert-exit":
		if len(args) != 2 {
			return errors.New("exit code required")
		}
		code, err := strconv.Atoi(args[1])
		if err != nil {
			return err
		}
		var p struct {
			State    string
			ExitCode int
		}
		if err = json.NewDecoder(os.Stdin).Decode(&p); err != nil {
			return err
		}
		if p.State != "exited" || p.ExitCode != code {
			return fmt.Errorf("container state=%s exit=%d want exited/%d", p.State, p.ExitCode, code)
		}
		return nil
	case "assert-live-inspect":
		b, err := io.ReadAll(io.LimitReader(os.Stdin, 65536))
		if err != nil {
			return err
		}
		if !strings.Contains(string(b), "inspector requires stopped monitor") {
			return fmt.Errorf("missing live-owner inspection refusal: %s", b)
		}
		return nil
	case "production-policy":
		var m struct {
			Services map[string]struct{ Restart string }
		}
		if err := json.NewDecoder(os.Stdin).Decode(&m); err != nil {
			return err
		}
		if m.Services["monitor"].Restart != "unless-stopped" {
			return errors.New("production restart policy must be unless-stopped")
		}
		fmt.Println("PASS production restart policy")
		return nil
	case "assert-owner":
		b, err := io.ReadAll(io.LimitReader(os.Stdin, 65536))
		if err != nil {
			return err
		}
		if !strings.Contains(string(b), "acquire state directory ownership") {
			return fmt.Errorf("missing ownership failure: %s", b)
		}
		return nil
	case "inspect":
		if len(args) != 2 {
			return errors.New("expected state JSON required")
		}
		return inspect(args[1])
	case "validate-config":
		return validateConfig(os.Stdin)
	case "metadata":
		return json.NewEncoder(os.Stdout).Encode(map[string]string{"go": runtime.Version(), "os": runtime.GOOS, "arch": runtime.GOARCH})
	default:
		return fmt.Errorf("unknown command %q", args[0])
	}
}
func main() {
	if err := command(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

// The fresh held request must produce exactly one post-barrier completion,
// caused by caller cancellation. A prior timeout/replacement cannot stand in
// for a signal even if the latest completion happens to say context canceled.
func shutdownRun(r io.Reader, source string, before int) error {
	runs, err := completedRunLogs(r, source)
	if err != nil {
		return err
	}
	if before < 0 || len(runs) != before+1 {
		return errors.New("expected exactly one post-barrier shutdown completion")
	}
	completed := runs[before]
	if completed.Status != "error" || !strings.Contains(completed.Error, "caller context: context canceled") || strings.Contains(completed.Error, "context deadline exceeded") {
		return fmt.Errorf("shutdown completion must be caller cancellation, got %s: %s", completed.Status, completed.Error)
	}
	return nil
}
