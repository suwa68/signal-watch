package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"reflect"
	"sort"
	"syscall"

	_ "modernc.org/sqlite"
)

// This inspector is deliberately independent of OpenSQLite: no schema creation,
// repair, writable transaction, or chown. It refuses a live owner's volume.
func inspect(expected string) error {
	var want map[string][]string
	if err := json.Unmarshal([]byte(expected), &want); err != nil {
		return err
	}
	lock, err := os.Open("/state/.signalwatch.lock")
	if err != nil {
		return err
	}
	defer lock.Close()
	if err = syscall.Flock(int(lock.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		return fmt.Errorf("inspector requires stopped monitor: %w", err)
	}
	defer syscall.Flock(int(lock.Fd()), syscall.LOCK_UN)
	for _, path := range []string{"/state", "/state/state.db", "/state/.signalwatch.lock"} {
		info, err := os.Stat(path)
		if err != nil {
			return err
		}
		st := info.Sys().(*syscall.Stat_t)
		if st.Uid != 10001 || st.Gid != 10001 {
			return fmt.Errorf("%s ownership %d:%d, want 10001:10001", path, st.Uid, st.Gid)
		}
	}
	dsn := (&url.URL{Scheme: "file", Path: "/state/state.db", RawQuery: "mode=ro"}).String()
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return err
	}
	defer db.Close()
	db.SetMaxOpenConns(1)
	var version int
	if err = db.QueryRow("PRAGMA user_version").Scan(&version); err != nil {
		return err
	}
	if version != 1 {
		return fmt.Errorf("schema version=%d", version)
	}
	got := map[string][]string{}
	rows, err := db.Query("SELECT source_id FROM initialized_sources ORDER BY source_id")
	if err != nil {
		return err
	}
	for rows.Next() {
		var s string
		if err = rows.Scan(&s); err != nil {
			rows.Close()
			return err
		}
		got[s] = []string{}
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	rows, err = db.Query("SELECT source_id,item_key FROM seen_items ORDER BY source_id,item_key")
	if err != nil {
		return err
	}
	for rows.Next() {
		var s, key string
		if err = rows.Scan(&s, &key); err != nil {
			rows.Close()
			return err
		}
		got[s] = append(got[s], key)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	for s, items := range want {
		keys := []string{}
		for _, item := range items {
			keys = append(keys, "url:http://fixture:8080/items/"+s+"/"+item)
		}
		sort.Strings(keys)
		want[s] = keys
	}
	if !reflect.DeepEqual(got, want) {
		return fmt.Errorf("stored state=%v want=%v", got, want)
	}
	fmt.Println("PASS stopped read-only state and UID/GID 10001:10001")
	return nil
}
