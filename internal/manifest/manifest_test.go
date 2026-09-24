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
	want := []string{"fort", "langfuse", "clickhouse", "metabase", "n8n", "twenty", "postgres", "traefik"}
	if got := m.ProductOrder(); !reflect.DeepEqual(got, want) {
		t.Fatalf("order %v, want %v", got, want)
	}
}

func TestLoadRefusesBadAsks(t *testing.T) {
	cases := map[string]string{
		"unknown type":            `{"products": {"p": {"containers": {"c": {"asks": [{"var": "X", "type": "number"}]}}}}}`,
		"choice without options":  `{"products": {"p": {"containers": {"c": {"asks": [{"var": "X", "type": "choice"}]}}}}}`,
		"bad when":                `{"products": {"p": {"containers": {"c": {"asks": [{"var": "X", "type": "text", "when": "sometimes"}]}}}}}`,
		"lowercase var":           `{"products": {"p": {"containers": {"c": {"asks": [{"var": "x", "type": "text"}]}}}}}`,
		"bad rename":              `{"products": {"p": {"containers": {"c": {"renamed": {"old": "NEW"}}}}}}`,
		"bad removed":             `{"products": {"p": {"containers": {"c": {"removed": ["x-y"]}}}}}`,
		"bad password var":        `{"products": {"p": {"containers": {"c": {"postgres": {"database": "c", "password": "c_pw"}}}}}}`,
		"bad setting name":        `{"products": {"p": {"containers": {"c": {"postgres": {"database": "c", "password": "C_PW", "settings": {"Statement Timeout": "5min"}}}}}}}`,
		"empty setting":           `{"products": {"p": {"containers": {"c": {"postgres": {"database": "c", "password": "C_PW", "settings": {"statement_timeout": ""}}}}}}}`,
		"settings disagree":       `{"products": {"p": {"containers": {"a": {"postgres": {"database": "c", "password": "C_PW", "settings": {"statement_timeout": "5min"}}}, "b": {"postgres": {"database": "c", "password": "C_PW"}}}}}}`,
		"clickhouse bad database": `{"products": {"p": {"containers": {"c": {"clickhouse": {"database": "1c", "password": "C_PW"}}}}}}`,
		"clickhouse bad password": `{"products": {"p": {"containers": {"c": {"clickhouse": {"database": "c", "password": "c_pw"}}}}}}`,
		"clickhouse settings":     `{"products": {"p": {"containers": {"c": {"clickhouse": {"database": "c", "password": "C_PW", "settings": {"x": "1"}}}}}}}`,
		"one variable two stores": `{"products": {"p": {"containers": {"c": {"postgres": {"database": "c", "password": "C_PW"}, "clickhouse": {"database": "c", "password": "C_PW"}}}}}}`,
		"bad port when":           `{"products": {"p": {"containers": {"c": {"ports": [{"port": 443, "when": "sometimes"}]}}}}}`,
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
	if !ok || c.Name != "metabase" || a.Type != DatabasePassword {
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

func TestClickHouseDatabaseIsAskedRefusedAndShared(t *testing.T) {
	m, err := load(t, `{"products": {
		"clickhouse": {"containers": {"clickhouse": {}}},
		"postgres": {"containers": {"postgres-18": {}}},
		"langfuse": {"containers": {
			"langfuse-web": {"postgres": {"database": "langfuse", "password": "LANGFUSE_DB_PASSWORD"}, "clickhouse": {"database": "langfuse", "password": "LANGFUSE_CH_PASSWORD"}},
			"langfuse-worker": {"requires": ["langfuse-web"], "clickhouse": {"database": "langfuse", "password": "LANGFUSE_CH_PASSWORD"}}
		}}
	}}`)
	if err != nil {
		t.Fatal(err)
	}
	web := m.Container("langfuse-web")
	var vars []string
	for _, a := range web.AllAsks() {
		vars = append(vars, a.Var+":"+a.Type)
	}
	if !reflect.DeepEqual(vars, []string{"LANGFUSE_DB_PASSWORD:database-password", "LANGFUSE_CH_PASSWORD:database-password"}) {
		t.Fatalf("asks: %v", vars)
	}
	if c, _, ok := m.AskFor("LANGFUSE_CH_PASSWORD"); !ok || c.Name != "langfuse-web" {
		t.Fatal("the ClickHouse password is an implied ask")
	}
	v := m.Validate([]string{"langfuse-web", "postgres-18"})
	if len(v.Refusals) != 1 || !strings.Contains(v.Refusals[0], "database on ClickHouse, so clickhouse must be on") {
		t.Fatalf("refusals: %v", v.Refusals)
	}
	if v := m.Validate([]string{"langfuse-web", "postgres-18", "clickhouse"}); len(v.Refusals) != 0 {
		t.Fatalf("refusals with both stores on: %v", v.Refusals)
	}
	if !m.SharesClickHouse(web, []string{"langfuse-worker"}) || m.SharesClickHouse(web, nil) || m.SharesDatabase(web, []string{"langfuse-worker"}) {
		t.Fatal("sharing is per store")
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

func TestExternalKindsLoadAndSupplyTheirVariables(t *testing.T) {
	m, err := load(t, `{"products": {
		"fort": {"containers": {"fort": {"external": {"FORT_S3": {"kind": "bucket", "versioned": true, "delete": "locks/*", "never_expire": true}, "FORT_KEY": {"kind": "secret-store"}}}}},
		"langfuse": {"containers": {
			"langfuse-web": {"external": {"LANGFUSE_S3": {"kind": "bucket", "delete": "*"}}},
			"langfuse-worker": {"requires": ["langfuse-web"], "external": {"LANGFUSE_S3": {"kind": "bucket", "delete": "*"}}}
		}}
	}}`)
	if err != nil {
		t.Fatal(err)
	}
	fort := m.Container("fort")
	if !reflect.DeepEqual(fort.Externals(), []string{"FORT_KEY", "FORT_S3"}) {
		t.Fatalf("externals %v", fort.Externals())
	}
	var vars []string
	for _, a := range fort.AllAsks() {
		vars = append(vars, a.Var+":"+a.Type)
	}
	want := []string{
		"FORT_KEY_PROVIDER:choice", "FORT_KEY_NAME:parameter-name", "FORT_KEY_REGION:text", "FORT_KEY_ACCESS_KEY_ID:secret", "FORT_KEY_SECRET_ACCESS_KEY:secret",
		"FORT_S3_BUCKET:bucket-name", "FORT_S3_REGION:text", "FORT_S3_ENDPOINT:bucket-endpoint", "FORT_S3_ACCESS_KEY_ID:secret", "FORT_S3_SECRET_ACCESS_KEY:secret",
	}
	if !reflect.DeepEqual(vars, want) {
		t.Fatalf("asks %v", vars)
	}
	if c, a, ok := m.AskFor("FORT_S3_BUCKET"); !ok || c.Name != "fort" || a.Type != BucketName {
		t.Fatal("a kind's variable is found like an ask")
	}
	if c, x, ok := m.External("LANGFUSE_S3"); !ok || c.Name != "langfuse-web" || !x.DeletesAnywhere() || x.Describe() != "bucket, delete" {
		t.Fatalf("External lookup: %v %v %v", c, x, ok)
	}
	if (External{Kind: Bucket, Versioned: true}).Describe() != "bucket, versioned" || (External{Kind: SecretStore}).Describe() != "secret-store" {
		t.Fatal("Describe")
	}
	locks, _, _ := m.External("FORT_S3")
	x := *locks.External["FORT_S3"]
	if !x.CanDelete() || x.DeletesAnywhere() || x.DeleteUnder() != "locks" || x.Describe() != "bucket, versioned, delete under locks/, never expire" {
		t.Fatalf("a prefix-scoped delete: %q", x.Describe())
	}
	if Dependency("FORT_S3") != "fort-s3" || Dependency("X") != "x" {
		t.Fatal("Dependency")
	}
}

func TestLoadRefusesBadExternals(t *testing.T) {
	cases := map[string]string{
		"unknown kind":              `{"products": {"p": {"containers": {"c": {"external": {"X": {"kind": "vault"}}}}}}}`,
		"no kind":                   `{"products": {"p": {"containers": {"c": {"external": {"X": {}}}}}}}`,
		"secret store versioned":    `{"products": {"p": {"containers": {"c": {"external": {"X": {"kind": "secret-store", "versioned": true}}}}}}}`,
		"secret store delete":       `{"products": {"p": {"containers": {"c": {"external": {"X": {"kind": "secret-store", "delete": "*"}}}}}}}`,
		"secret store never expire": `{"products": {"p": {"containers": {"c": {"external": {"X": {"kind": "secret-store", "never_expire": true}}}}}}}`,
		"delete on no prefix":       `{"products": {"p": {"containers": {"c": {"external": {"X": {"kind": "bucket", "delete": "locks"}}}}}}}`,
		"delete on a bare glob":     `{"products": {"p": {"containers": {"c": {"external": {"X": {"kind": "bucket", "delete": "/*"}}}}}}}`,
		"lowercase prefix":          `{"products": {"p": {"containers": {"c": {"external": {"x_s3": {"kind": "bucket"}}}}}}}`,
		"shared but different":      `{"products": {"p": {"containers": {"a": {"external": {"X": {"kind": "bucket"}}}, "b": {"external": {"X": {"kind": "bucket", "delete": "*"}}}}}}}`,
		"ask collides with a kind":  `{"products": {"p": {"containers": {"c": {"asks": [{"var": "X_BUCKET", "type": "text"}], "external": {"X": {"kind": "bucket"}}}}}}}`,
	}
	for name, body := range cases {
		if _, err := load(t, body); err == nil || !strings.HasPrefix(err.Error(), "manifest.json: ") {
			t.Errorf("%s: want a manifest.json refusal, got %v", name, err)
		}
	}
	if _, err := load(t, `{"products": {"p": {"containers": {"a": {"external": {"X": {"kind": "bucket", "delete": "*"}}}, "b": {"external": {"X": {"kind": "bucket", "delete": "*"}}}}}}}`); err != nil {
		t.Fatalf("two containers may share one external when they agree: %v", err)
	}
}
