package manifest

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func load(t *testing.T, body string) (*Manifest, error) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "manifest.json")
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return Load(path)
}

func TestProductOrderPutsDependentsFirstAndTiesAlphabetically(t *testing.T) {
	m, err := load(t, `{"products": {
		"traefik": {"containers": {"traefik": {}}},
		"postgres": {"containers": {"postgres-18": {}, "pgbouncer-transaction": {"requires": ["postgres-18"]}}},
		"redis": {"containers": {"redis": {}}},
		"metabase": {"containers": {"metabase": {"requires": ["pgbouncer-transaction"], "optional": ["traefik"]}}},
		"langfuse": {"containers": {"langfuse-web": {"requires": ["pgbouncer-transaction", "redis"], "optional": ["traefik"]}, "langfuse-worker": {"requires": ["langfuse-web"]}}}
	}}`)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"langfuse", "metabase", "postgres", "redis", "traefik"}
	if got := m.ProductOrder(); !reflect.DeepEqual(got, want) {
		t.Fatalf("order %v, want %v", got, want)
	}
}

func TestProductOrderOnTheRealManifest(t *testing.T) {
	m, err := Load(filepath.Join("..", "..", "manifest.json"))
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"metabase", "postgres", "traefik"}
	if got := m.ProductOrder(); !reflect.DeepEqual(got, want) {
		t.Fatalf("order %v, want %v", got, want)
	}
}

func TestLoadRefusesBadAsks(t *testing.T) {
	cases := map[string]string{
		"unknown type":           `{"products": {"p": {"containers": {"c": {"asks": [{"var": "X", "type": "number"}]}}}}}`,
		"choice without options": `{"products": {"p": {"containers": {"c": {"asks": [{"var": "X", "type": "choice"}]}}}}}`,
		"bad when":               `{"products": {"p": {"containers": {"c": {"asks": [{"var": "X", "type": "text", "when": "sometimes"}]}}}}}`,
		"lowercase var":          `{"products": {"p": {"containers": {"c": {"asks": [{"var": "x", "type": "text"}]}}}}}`,
		"bad rename":             `{"products": {"p": {"containers": {"c": {"renamed": {"old": "NEW"}}}}}}`,
		"bad removed":            `{"products": {"p": {"containers": {"c": {"removed": ["x-y"]}}}}}`,
		"bad password var":       `{"products": {"p": {"containers": {"c": {"postgres": {"database": "c", "password": "c_pw"}}}}}}`,
	}
	for name, body := range cases {
		if _, err := load(t, body); err == nil || !strings.HasPrefix(err.Error(), "manifest.json: ") {
			t.Errorf("%s: want a manifest.json refusal, got %v", name, err)
		}
	}
}

func TestAskApplies(t *testing.T) {
	values := map[string]string{"DNS_PROVIDER": "route53"}
	lookup := func(k string) string { return values[k] }
	cases := []struct {
		when, visibility string
		want             bool
	}{
		{"", "local", true},
		{"public", "public", true},
		{"public", "local", false},
		{"local", "local", true},
		{"DNS_PROVIDER=route53", "public", true},
		{"DNS_PROVIDER=cloudflare", "public", false},
		{"MISSING=x", "public", false},
	}
	for _, c := range cases {
		if got := (Ask{When: c.when}).Applies(c.visibility, lookup); got != c.want {
			t.Errorf("when %q in %s: got %v, want %v", c.when, c.visibility, got, c.want)
		}
	}
}

func TestAskForFindsAskedAndImpliedVariables(t *testing.T) {
	m, err := Load(filepath.Join("..", "..", "manifest.json"))
	if err != nil {
		t.Fatal(err)
	}
	c, a, ok := m.AskFor("METABASE_DB_PASSWORD")
	if !ok || c.Name != "metabase" || a.Type != Generated {
		t.Fatalf("implied password: %v %v %v", ok, c, a)
	}
	c, a, ok = m.AskFor("CF_DNS_API_TOKEN")
	if !ok || c.Name != "traefik" || a.Condition() != "DNS_PROVIDER is cloudflare" {
		t.Fatalf("traefik ask: %v %v %q", ok, c, a.Condition())
	}
	if _, _, ok := m.AskFor("METABASE_PORT"); ok {
		t.Fatal("an optional variable is not asked")
	}
	if (Ask{When: "public"}).Condition() != "visibility is public" || (Ask{}).Condition() != "always" {
		t.Fatal("conditions read wrong")
	}
}

func TestDatabaseAndSharesDatabase(t *testing.T) {
	m, err := load(t, `{"products": {
		"postgres": {"containers": {"postgres-18": {}}},
		"langfuse": {"containers": {
			"langfuse-web": {"postgres": {"database": "langfuse", "password": "LANGFUSE_DB_PASSWORD"}},
			"langfuse-worker": {"requires": ["langfuse-web"], "postgres": {"database": "langfuse", "password": "LANGFUSE_DB_PASSWORD"}}
		}}
	}}`)
	if err != nil {
		t.Fatal(err)
	}
	if m.Database("langfuse") == nil || m.Database("nobody") != nil {
		t.Fatal("Database lookup")
	}
	web := m.Container("langfuse-web")
	if !m.SharesDatabase(web, []string{"langfuse-worker", "postgres-18"}) {
		t.Fatal("worker still on, database is shared")
	}
	if m.SharesDatabase(web, []string{"postgres-18"}) {
		t.Fatal("nobody else on, database is not shared")
	}
	if !ValidIdentifier("app_1") || ValidIdentifier("1app") || ValidIdentifier("App") || !ValidName("pgbouncer-transaction") || ValidName("a_b") {
		t.Fatal("name rules")
	}
}
