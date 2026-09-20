package interview

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/Abdullah0297445/userland/internal/env"
	"github.com/Abdullah0297445/userland/internal/manifest"
)

func TestShapes(t *testing.T) {
	dir := t.TempDir()
	ok := map[string][]string{
		manifest.Text:      {"anything goes", "a-b_c"},
		manifest.Hostname:  {"example.com", "a.b.example.co.uk", "localhost", "Example.COM"},
		manifest.Email:     {"name@example.com"},
		manifest.URL:       {"https://s3.example.com", "http://127.0.0.1:9000/path"},
		manifest.Port:      {"1", "8080", "65535"},
		manifest.Secret:    {"abc123", "AKIA+/="},
		manifest.Generated: {"", "pasted"},
		manifest.Paths:     {dir, dir + ":" + dir},
	}
	bad := map[string][]string{
		manifest.Text:      {"", " padded", "has$dollar", "has#hash", `has"quote`, "two\nlines"},
		manifest.Hostname:  {"", "-bad.example", "bad_.example", "a..b", "with space.com"},
		manifest.Email:     {"", "nope", "Name <name@example.com>", "name@localhost"},
		manifest.URL:       {"", "example.com", "https://", "://nope"},
		manifest.Port:      {"", "0", "65536", "http"},
		manifest.Secret:    {"", "it's"},
		manifest.Generated: {"has$dollar"},
		manifest.Paths:     {"", "relative/path", filepath.Join(dir, "missing")},
	}
	for kind, values := range ok {
		for _, v := range values {
			if err := Shape(kind)(v); err != nil {
				t.Errorf("%s %q: unexpected refusal %v", kind, v, err)
			}
		}
	}
	for kind, values := range bad {
		for _, v := range values {
			if err := Shape(kind)(v); err == nil {
				t.Errorf("%s %q: accepted", kind, v)
			}
		}
	}
}

func TestGenerateIsSafeAndLong(t *testing.T) {
	seen := map[string]bool{}
	for i := 0; i < 100; i++ {
		v := Generate()
		if len(v) < 26 || safe(v) != nil || seen[v] {
			t.Fatalf("generated %q", v)
		}
		seen[v] = true
	}
}

func TestMigrate(t *testing.T) {
	dir := t.TempDir()
	body := `{"products": {"p": {"containers": {"c": {"renamed": {"OLD_URL": "NEW_ENDPOINT", "ABSENT": "ALSO_ABSENT"}, "removed": ["GONE", "NEVER_THERE"]}}}}}`
	if err := os.WriteFile(filepath.Join(dir, "manifest.json"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	m, err := manifest.Load(filepath.Join(dir, "manifest.json"))
	if err != nil {
		t.Fatal(err)
	}
	e := env.New(filepath.Join(dir, ".env"))
	e.Set("KEEP", "1")
	e.Set("OLD_URL", "https://example.test")
	e.Set("GONE", "x")
	did := Migrate(m, e)
	if len(did) != 2 {
		t.Fatalf("did %v", did)
	}
	if e.Has("OLD_URL") || e.Has("GONE") || e.Get("NEW_ENDPOINT") != "https://example.test" || e.Get("KEEP") != "1" {
		t.Fatalf("env after migrate: NEW_ENDPOINT=%q KEEP=%q", e.Get("NEW_ENDPOINT"), e.Get("KEEP"))
	}
	if again := Migrate(m, e); len(again) != 0 {
		t.Fatalf("second migrate did %v", again)
	}
}

func TestLabelNamesWhoNeedsIt(t *testing.T) {
	m, err := manifest.Load(filepath.Join("..", "..", "manifest.json"))
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]string{}
	for _, c := range m.All() {
		got[c.Name] = label(m, c)
	}
	want := map[string]string{
		"metabase":              "metabase",
		"pgbouncer-session":     "pgbouncer-session",
		"pgbouncer-transaction": "pgbouncer-transaction: required by metabase",
		"postgres-18":           "postgres-18: required by pgbouncer-session, pgbouncer-transaction",
		"traefik":               "traefik: optional for metabase",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("labels %v", got)
	}
}
