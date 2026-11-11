//go:build unix

package state

import (
	"bufio"
	"context"
	"database/sql/driver"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"modernc.org/sqlite"
)

// The blocking SQL function and trigger exist only in the test binary/database.
// They pause the actual baseline write after its keys and before its marker commits.
func TestSQLiteProcessHelper(t *testing.T) {
	mode := os.Getenv("SIGNALWATCH_STATE_HELPER")
	if mode == "" {
		return
	}
	dir := os.Getenv("SIGNALWATCH_STATE_DIR")
	s, err := OpenSQLite(context.Background(), dir)
	if mode == "refuse" {
		if err == nil {
			s.Close()
			os.Exit(2)
		}
		fmt.Println("refused")
		os.Exit(0)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(3)
	}
	if mode == "crash" {
		err := sqlite.RegisterScalarFunction("test_baseline_barrier", 0, func(*sqlite.FunctionContext, []driver.Value) (driver.Value, error) {
			fmt.Println("ready")
			var b [1]byte
			_, err := os.Stdin.Read(b[:])
			return nil, err
		})
		if err != nil {
			os.Exit(4)
		}
		// Functions are installed on new connections only.
		s.db.SetMaxIdleConns(0)
		s.db.SetMaxIdleConns(1)
		if _, err := s.db.Exec(`CREATE TEMP TRIGGER pause_baseline BEFORE INSERT ON initialized_sources BEGIN SELECT test_baseline_barrier(); END`); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(5)
		}
		_, err = s.EstablishBaseline(context.Background(), "crash", []string{"A", "B"})
		fmt.Fprintln(os.Stderr, err)
		os.Exit(6)
	}
	fmt.Println("ready")
	var b [1]byte
	_, _ = os.Stdin.Read(b[:])
	if err := s.Close(); err != nil {
		os.Exit(7)
	}
	os.Exit(0)
}

func startStateHelper(t *testing.T, dir, mode string) (*exec.Cmd, func()) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	t.Cleanup(cancel)
	cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestSQLiteProcessHelper$")
	cmd.Env = append(os.Environ(), "SIGNALWATCH_STATE_HELPER="+mode, "SIGNALWATCH_STATE_DIR="+dir)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	stdin, err := cmd.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	cmd.Stderr = os.Stderr
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		stdin.Close()
		if cmd.ProcessState == nil {
			cmd.Process.Kill()
			cmd.Wait()
		}
	})
	scanner := bufio.NewScanner(stdout)
	if !scanner.Scan() || scanner.Text() != "ready" {
		t.Fatalf("helper failed barrier: %q %v", scanner.Text(), scanner.Err())
	}
	return cmd, func() {
		stdin.Close()
		if err := cmd.Wait(); err != nil {
			t.Fatal(err)
		}
	}
}

func TestSQLiteProcessOwnershipAndAlias(t *testing.T) {
	dir := t.TempDir()
	cmd, closeHelper := startStateHelper(t, dir, "owner")
	alias := filepath.Join(t.TempDir(), "alias")
	if err := os.Symlink(dir, alias); err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(filepath.Join(dir, "state.db"))
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{dir, alias} {
		child := exec.Command(os.Args[0], "-test.run=^TestSQLiteProcessHelper$")
		child.Env = append(os.Environ(), "SIGNALWATCH_STATE_HELPER=refuse", "SIGNALWATCH_STATE_DIR="+path)
		if out, err := child.CombinedOutput(); err != nil {
			t.Fatalf("second owner: %s %v", out, err)
		}
	}
	after, err := os.ReadFile(filepath.Join(dir, "state.db"))
	if err != nil {
		t.Fatal(err)
	}
	if string(before) != string(after) {
		t.Fatal("refused owner changed database")
	}
	original, err := os.Stat(filepath.Join(dir, ".signalwatch.lock"))
	if err != nil {
		t.Fatal(err)
	}
	closeHelper()
	_ = cmd
	s := openTestSQLite(t, alias)
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	current, err := os.Stat(filepath.Join(dir, ".signalwatch.lock"))
	if err != nil || !os.SameFile(original, current) {
		t.Fatalf("lock inode replaced: %v", err)
	}
}

func TestSQLiteLockPrecedesDatabaseCreation(t *testing.T) {
	dir := t.TempDir()
	lock, err := acquireOwnership(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer lock.Close()
	if s, err := OpenSQLite(context.Background(), dir); err == nil {
		s.Close()
		t.Fatal("accepted second owner")
	}
	if _, err := os.Stat(filepath.Join(dir, "state.db")); !os.IsNotExist(err) {
		t.Fatalf("database touched before lock: %v", err)
	}
}

func TestSQLiteKilledBaselineIsAtomicAndReleasesOwnership(t *testing.T) {
	dir := t.TempDir()
	cmd, _ := startStateHelper(t, dir, "crash")
	if err := cmd.Process.Kill(); err != nil {
		t.Fatal(err)
	}
	if err := cmd.Wait(); err == nil {
		t.Fatal("expected killed process")
	}
	s := openTestSQLite(t, dir)
	ctx := context.Background()
	if got, err := s.IsSourceInitialized(ctx, "crash"); got || err != nil {
		t.Fatalf("partial marker: %v %v", got, err)
	}
	for _, key := range []string{"A", "B"} {
		if got, err := s.HasItem(ctx, "crash", key); got || err != nil {
			t.Fatalf("partial baseline key: %v %v", got, err)
		}
	}
	if got, err := s.EstablishBaseline(ctx, "crash", []string{"C"}); !got || err != nil {
		t.Fatalf("cannot recover: %v %v", got, err)
	}
}
