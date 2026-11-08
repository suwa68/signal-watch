package main

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"strings"
	"sync/atomic"
	"syscall"
	"testing"
	"time"

	"github.com/suwa68/signal-watch/internal/application"
	"github.com/suwa68/signal-watch/internal/config"
	"github.com/suwa68/signal-watch/internal/notification"
	"github.com/suwa68/signal-watch/internal/query"
	monitor "github.com/suwa68/signal-watch/internal/runtime"
	"github.com/suwa68/signal-watch/internal/state"
)

type beforeMarkStore struct{ *state.SQLiteStateStore }

func (s *beforeMarkStore) MarkItemSeen(context.Context, string, string) error {
	// This barrier is reached only after Pipeline's Send call has returned nil,
	// before any real MarkItemSeen mutation. It is intentionally helper tier.
	fmt.Println("before_mark")
	select {}
}
func TestDaemonProcessHelper(t *testing.T) {
	mode := os.Getenv("SIGNALWATCH_TEST_HELPER")
	if mode == "" {
		return
	}
	var ctx context.Context
	d := testDependencies(context.Background())
	d.signalContext = func() (context.Context, context.CancelFunc) {
		var stop context.CancelFunc
		ctx, stop = (dependencies{}).defaults().signalContext()
		return ctx, stop
	}
	d.openStore = func(c context.Context, dir string) (ownedStore, error) {
		if mode == "crash" {
			store, err := state.OpenSQLite(c, dir)
			return &beforeMarkStore{store}, err
		}
		return &testStore{state.NewMemoryStateStore(), func() error {
			fmt.Println("store_closed")
			if mode == "cleanup" {
				return errors.New("cleanup failed")
			}
			return nil
		}}, nil
	}
	if mode == "init" {
		d.newNotifier = func(string) (notifier, error) { fmt.Println("init_started"); <-ctx.Done(); return nil, nil }
		d.schedule = func(context.Context, []monitor.ScheduledSource, func(config.SourceConfig, error)) error {
			panic("scheduled after initialization cancellation")
		}
	}
	if mode == "noncooperative" || mode == "fatal" || mode == "cleanup" {
		d.schedule = func(c context.Context, _ []monitor.ScheduledSource, _ func(config.SourceConfig, error)) error {
			fmt.Println("worker_started")
			if mode == "noncooperative" {
				select {}
			}
			<-c.Done()
			fmt.Println("worker_finished")
			if mode == "fatal" {
				return errors.Join(c.Err(), state.ErrFatal)
			}
			return c.Err()
		}
	}
	if mode == "timer" || mode == "request" {
		d.schedule = func(c context.Context, s []monitor.ScheduledSource, report func(config.SourceConfig, error)) error {
			if mode == "request" {
				fmt.Println("worker_started")
			}
			return monitor.Run(c, s, func(cfg config.SourceConfig, err error) {
				report(cfg, err)
				if mode == "timer" {
					fmt.Println("worker_started")
				}
			})
		}
	}
	if mode == "crash" {
		d.schedule = func(c context.Context, s []monitor.ScheduledSource, _ func(config.SourceConfig, error)) error {
			if err := s[0].Runner.RunOnce(c, s[0].Config); err != nil {
				return err
			}
			return s[0].Runner.RunOnce(c, s[0].Config)
		}
	}
	code := run([]string{"--sources-dir", os.Getenv("SIGNALWATCH_TEST_SOURCES"), "--state-dir", os.Getenv("SIGNALWATCH_TEST_STATE")}, env, os.Stdout, os.Stderr, d)
	os.Exit(code)
}

type helperProcess struct {
	cmd     *exec.Cmd
	scanner *bufio.Scanner
	cancel  context.CancelFunc
}

func startHelper(t *testing.T, mode, dir, stateDir string) *helperProcess {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Second)
	cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestDaemonProcessHelper$")
	cmd.Env = append(os.Environ(), "SIGNALWATCH_TEST_HELPER="+mode, "SIGNALWATCH_TEST_SOURCES="+dir, "SIGNALWATCH_TEST_STATE="+stateDir)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	cmd.Stderr = cmd.Stdout
	if err = cmd.Start(); err != nil {
		cancel()
		t.Fatal(err)
	}
	t.Cleanup(func() { cancel(); _ = cmd.Process.Kill() })
	return &helperProcess{cmd, bufio.NewScanner(stdout), cancel}
}
func (p *helperProcess) await(t *testing.T, want string) {
	t.Helper()
	for p.scanner.Scan() {
		if p.scanner.Text() == want {
			return
		}
	}
	t.Fatalf("helper never reached %s: %v", want, p.scanner.Err())
}
func (p *helperProcess) finish(t *testing.T, want int) string {
	t.Helper()
	var lines []string
	for p.scanner.Scan() {
		lines = append(lines, p.scanner.Text())
	}
	err := p.cmd.Wait()
	p.cancel()
	code := 0
	if err != nil {
		var exit *exec.ExitError
		if !errors.As(err, &exit) {
			t.Fatal(err)
		}
		code = exit.ExitCode()
	}
	if code != want {
		t.Fatalf("exit %d want %d: %v\n%s", code, want, err, strings.Join(lines, "\n"))
	}
	return strings.Join(lines, "\n")
}
func TestSubprocessSignalLifecycle(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { io.WriteString(w, articles("A", "B")) }))
	defer server.Close()
	for _, tc := range []struct {
		mode, barrier string
		want          int
	}{{"timer", "worker_started", 0}, {"init", "init_started", 0}, {"fatal", "worker_started", 1}, {"cleanup", "worker_started", 1}, {"noncooperative", "worker_started", 1}} {
		t.Run(tc.mode, func(t *testing.T) {
			p := startHelper(t, tc.mode, definitions(t, "news", "3600", server.URL), t.TempDir())
			p.await(t, tc.barrier)
			started := time.Now()
			if err := p.cmd.Process.Signal(syscall.SIGTERM); err != nil {
				t.Fatal(err)
			}
			output := p.finish(t, tc.want)
			if tc.mode == "noncooperative" {
				if time.Since(started) < 9*time.Second || time.Since(started) > 15*time.Second {
					t.Fatalf("production grace took %v", time.Since(started))
				}
				if strings.Contains(output, "store_closed") {
					t.Fatal("closed active store")
				}
				if !strings.Contains(output, "shutdown grace exceeded") {
					t.Fatalf("missing timeout error %s", output)
				}
			}
			if tc.mode == "fatal" || tc.mode == "cleanup" {
				if !strings.Contains(output, "worker_finished\nstore_closed") {
					t.Fatalf("close ordering: %s", output)
				}
			}
		})
	}
}
func TestSubprocessSignalCancelsRequest(t *testing.T) {
	entered := make(chan struct{})
	canceled := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { close(entered); <-r.Context().Done(); close(canceled) }))
	defer server.Close()
	p := startHelper(t, "request", definitions(t, "news", "1", server.URL), t.TempDir())
	p.await(t, "worker_started")
	<-entered
	if err := p.cmd.Process.Signal(syscall.SIGTERM); err != nil {
		t.Fatal(err)
	}
	p.finish(t, 0)
	<-canceled
}
func TestSuccessfulSendBeforeMarkCrashLeavesDuplicateEligible(t *testing.T) {
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if requests.Add(1) == 1 {
			io.WriteString(w, articles("A", "B"))
		} else {
			io.WriteString(w, articles("A", "B", "C"))
		}
	}))
	defer server.Close()
	dir := definitions(t, "news", "1", server.URL)
	stateDir := t.TempDir()
	p := startHelper(t, "crash", dir, stateDir)
	p.await(t, "before_mark")
	if err := p.cmd.Process.Kill(); err != nil {
		t.Fatal(err)
	}
	p.finish(t, -1)
	ctx := context.Background()
	store, err := state.OpenSQLite(ctx, stateDir)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	catalog, collector, err := query.LoadHTMLCatalogAndRunner(ctx, dir, nil)
	if err != nil {
		t.Fatal(err)
	}
	var sent []string
	pipeline, err := application.NewPipeline(collector, store, application.NotificationSenderFunc(func(_ context.Context, n notification.Notification) error { sent = append(sent, n.Title); return nil }))
	if err != nil {
		t.Fatal(err)
	}
	if err := pipeline.RunOnce(ctx, catalog.Definitions()[0].Config); err != nil {
		t.Fatal(err)
	}
	if len(sent) != 1 || sent[0] != "C" {
		t.Fatalf("post-crash sends %v want C", sent)
	}
}
