package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/Abdullah0297445/userland/internal/manifest"
)

func TestLeftBehindNamesVolumesAndAnUnsharedDatabase(t *testing.T) {
	body := `{"products": {
		"postgres": {"containers": {"postgres-18": {"volumes": ["postgres_data"]}}},
		"langfuse": {"containers": {
			"langfuse-web": {"requires": ["postgres-18"], "postgres": {"database": "langfuse", "password": "LANGFUSE_DB_PASSWORD"}, "volumes": ["langfuse_media"]},
			"langfuse-worker": {"requires": ["langfuse-web"], "postgres": {"database": "langfuse", "password": "LANGFUSE_DB_PASSWORD"}}
		}}
	}}`
	path := filepath.Join(t.TempDir(), "manifest.json")
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	m, err := manifest.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	web := leftBehind(m, []string{"postgres-18", "langfuse-worker"}, m.Container("langfuse-web"))
	if web.String() != "volume userland_langfuse_media" {
		t.Fatalf("worker still on: %q", web.String())
	}
	web = leftBehind(m, []string{"postgres-18"}, m.Container("langfuse-web"))
	if web.String() != "volume userland_langfuse_media, database langfuse and its user on Postgres" {
		t.Fatalf("worker off: %q", web.String())
	}
	worker := leftBehind(m, []string{"postgres-18", "langfuse-web"}, m.Container("langfuse-worker"))
	if !worker.empty() {
		t.Fatalf("worker declares nothing of its own: %q", worker.String())
	}
	postgres := leftBehind(m, nil, m.Container("postgres-18"))
	if postgres.String() != "volume userland_postgres_data (every database on Postgres lives in it)" {
		t.Fatalf("postgres: %q", postgres.String())
	}
	both := web.merge(postgres)
	if len(both.volumes) != 2 || len(both.databases) != 1 || both.notes["userland_postgres_data"] == "" {
		t.Fatalf("merge: %+v", both)
	}
}

func TestLeftBehindNamesAClickHouseDatabaseAndTheStoreThatIsOff(t *testing.T) {
	body := `{"products": {
		"clickhouse": {"containers": {"clickhouse": {"volumes": ["clickhouse_data", "clickhouse_backups"]}}},
		"langfuse": {"containers": {
			"langfuse-web": {"requires": ["clickhouse"], "clickhouse": {"database": "langfuse", "password": "LANGFUSE_CH_PASSWORD"}},
			"langfuse-worker": {"requires": ["langfuse-web"], "clickhouse": {"database": "langfuse", "password": "LANGFUSE_CH_PASSWORD"}}
		}}
	}}`
	path := filepath.Join(t.TempDir(), "manifest.json")
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	m, err := manifest.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	web := leftBehind(m, []string{"clickhouse", "langfuse-worker"}, m.Container("langfuse-web"))
	if !web.empty() {
		t.Fatalf("worker still on, the database is shared: %q", web.String())
	}
	web = leftBehind(m, []string{"clickhouse"}, m.Container("langfuse-web"))
	if web.String() != "database langfuse and its user on ClickHouse" {
		t.Fatalf("worker off: %q", web.String())
	}
	if web.storeOff([]string{"clickhouse"}) != "" || web.storeOff(nil) != "clickhouse" {
		t.Fatalf("storeOff: %q %q", web.storeOff([]string{"clickhouse"}), web.storeOff(nil))
	}
	store := leftBehind(m, nil, m.Container("clickhouse"))
	if store.String() != "volume userland_clickhouse_data (every database on ClickHouse lives in it), volume userland_clickhouse_backups" {
		t.Fatalf("clickhouse: %q", store.String())
	}
}
