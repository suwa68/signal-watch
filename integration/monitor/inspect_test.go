package main

import (
	"database/sql"
	"path/filepath"
	"testing"
)

// Valid pre-initialization seen marks must not be mistaken for a baseline.
// Build the actual two-table SQLite state and inspect through a read-only pool.
func TestInspectorDistinguishesInitializationFromSeenRows(t *testing.T) {
	for _, tc := range []struct {
		name        string
		initialized bool
		items       []string
		wantError   bool
	}{
		{"seen rows without initialization", false, []string{"A", "B"}, true},
		{"initialized with seen rows", true, []string{"A", "B"}, false},
		{"valid empty initialized source", true, []string{}, false},
		{"missing empty initialization", false, []string{}, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "state.db")
			db, err := sql.Open("sqlite", path)
			if err != nil {
				t.Fatal(err)
			}
			for _, stmt := range []string{"CREATE TABLE initialized_sources(source_id TEXT PRIMARY KEY)", "CREATE TABLE seen_items(source_id TEXT,item_key TEXT)", "PRAGMA user_version=1"} {
				if _, err := db.Exec(stmt); err != nil {
					t.Fatal(err)
				}
			}
			if tc.initialized {
				if _, err := db.Exec("INSERT INTO initialized_sources VALUES('alpha')"); err != nil {
					t.Fatal(err)
				}
			}
			for _, item := range tc.items {
				if _, err := db.Exec("INSERT INTO seen_items VALUES('alpha',?)", "url:http://fixture:8080/items/alpha/"+item); err != nil {
					t.Fatal(err)
				}
			}
			if err := db.Close(); err != nil {
				t.Fatal(err)
			}
			ro, err := sql.Open("sqlite", "file:"+path+"?mode=ro")
			if err != nil {
				t.Fatal(err)
			}
			defer ro.Close()
			ro.SetMaxOpenConns(1)
			err = inspectStoredState(ro, map[string][]string{"alpha": tc.items})
			if (err != nil) != tc.wantError {
				t.Fatalf("inspection error=%v wantError=%v", err, tc.wantError)
			}
		})
	}
}
