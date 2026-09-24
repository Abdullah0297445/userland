package interview

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
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
		manifest.Generated: {"", "pasted", "p@ss:w/rd"},
		manifest.Hex:       {"", strings.Repeat("0f", 32), strings.Repeat("AB", 32)},
		manifest.Paths:     {dir, dir + ":" + dir},

		manifest.DatabasePassword: {"", "ABCDEFGHJKMNPQRSTVWXYZ2345", "a-b.c_d~e"},
	}
	bad := map[string][]string{
		manifest.Text:      {"", " padded", "has$dollar", "has#hash", `has"quote`, "two\nlines"},
		manifest.Hostname:  {"", "-bad.example", "bad_.example", "a..b", "with space.com"},
		manifest.Email:     {"", "nope", "Name <name@example.com>", "name@localhost"},
		manifest.URL:       {"", "example.com", "https://", "://nope"},
		manifest.Port:      {"", "0", "65536", "http"},
		manifest.Secret:    {"", "it's"},
		manifest.Generated: {"has$dollar"},
		manifest.Hex:       {"pasted", strings.Repeat("0f", 31), strings.Repeat("0g", 32), strings.Repeat("0f", 33)},
		manifest.Paths:     {"", "relative/path", filepath.Join(dir, "missing")},

		manifest.DatabasePassword: {"p@ss", "user:pass", "a/b", "50%", "a?b", "a&b", "a=b", "a+b", "has$dollar"},
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

func TestGenerateHexIsAKeyOf256Bits(t *testing.T) {
	seen := map[string]bool{}
	for i := 0; i < 100; i++ {
		v := GenerateHex()
		if Shape(manifest.Hex)(v) != nil || v == "" || seen[v] {
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
	e.Set(On, "c,retired")
	e.Set(Off, "retired")
	did := Migrate(m, e)
	if len(did) != 4 {
		t.Fatalf("did %v", did)
	}
	if e.Has("OLD_URL") || e.Has("GONE") || e.Get("NEW_ENDPOINT") != "https://example.test" || e.Get("KEEP") != "1" {
		t.Fatalf("env after migrate: NEW_ENDPOINT=%q KEEP=%q", e.Get("NEW_ENDPOINT"), e.Get("KEEP"))
	}
	if e.Get(On) != "c" || e.Get(Off) != "" {
		t.Fatalf("selection after migrate: %s=%q %s=%q", On, e.Get(On), Off, e.Get(Off))
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
		"clickhouse":            "clickhouse: required by langfuse-web, langfuse-worker; optional for fort",
		"fort":                  "fort",
		"langfuse-redis":        "langfuse-redis: required by langfuse-web, langfuse-worker",
		"langfuse-web":          "langfuse-web: required by langfuse-worker",
		"langfuse-worker":       "langfuse-worker: optional for langfuse-web",
		"metabase":              "metabase",
		"n8n":                   "n8n: required by n8n-runners",
		"n8n-runners":           "n8n-runners: optional for n8n",
		"pgadmin":               "pgadmin",
		"pgbouncer-session":     "pgbouncer-session: required by langfuse-web, twenty-server, twenty-worker",
		"pgbouncer-transaction": "pgbouncer-transaction: required by langfuse-web, langfuse-worker, metabase, n8n",
		"postgres-18":           "postgres-18: required by pgadmin, pgbouncer-session, pgbouncer-transaction; optional for fort",
		"traefik":               "traefik: optional for langfuse-web, metabase, n8n, pgadmin, twenty-server",
		"twenty-redis":          "twenty-redis: required by twenty-server, twenty-worker",
		"twenty-server":         "twenty-server: required by twenty-worker",
		"twenty-worker":         "twenty-worker: optional for twenty-server",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("labels %v", got)
	}
}

func TestBucketAndParameterNameShapes(t *testing.T) {
	ok := map[string][]string{
		manifest.BucketName:     {"userland-fort-s3-0a1b2c3d", "abc", "a.b-c", "a1b2"},
		manifest.ParameterName:  {"/userland/fort-key-0a1b2c3d", "userland/fort-key", "plain_name", "/a/b/c.d"},
		manifest.BucketEndpoint: {"https://s3.example.com", "https://s3.example.com/", "http://127.0.0.1:9000", "http://[::1]:9000/"},
	}
	bad := map[string][]string{
		manifest.BucketName:     {"", "ab", "Upper", "-starts", "ends-", "a..b", "192.168.0.1", "has_underscore", "with space"},
		manifest.ParameterName:  {"", "/aws/x", "ssm-key", "/AWS/x", "a//b", "/", "with space", "has$dollar"},
		manifest.BucketEndpoint: {"", "s3.example.com", "https://", "https://s3.example.com/bucket", "https://s3.example.com//", "ftp://s3.example.com", "HTTPS://s3.example.com", "https://s3.example.com?x=1", "https://s3.example.com#x", "https://user@s3.example.com"},
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

func TestCleanDropsAnEndpointsTrailingSlashAndNothingElse(t *testing.T) {
	for _, c := range []struct{ kind, in, want string }{
		{manifest.BucketEndpoint, "https://s3.example.com/", "https://s3.example.com"},
		{manifest.BucketEndpoint, "https://s3.example.com", "https://s3.example.com"},
		{manifest.BucketEndpoint, "http://127.0.0.1:9000/", "http://127.0.0.1:9000"},
		{manifest.URL, "https://example.com/", "https://example.com/"},
		{manifest.URL, "http://127.0.0.1:9000/path/", "http://127.0.0.1:9000/path/"},
		{manifest.Text, "ends/", "ends/"},
		{manifest.Paths, "/srv/app/", "/srv/app/"},
	} {
		if got := Clean(c.kind, c.in); got != c.want {
			t.Errorf("Clean(%s, %q) = %q, want %q", c.kind, c.in, got, c.want)
		}
	}
}

func TestGeneratedNamesCarryTheDependencyAndATail(t *testing.T) {
	b := GenerateBucketName("FORT_S3")
	if !strings.HasPrefix(b, "userland-fort-s3-") || len(b) != len("userland-fort-s3-")+8 || Shape(manifest.BucketName)(b) != nil {
		t.Fatalf("bucket %q", b)
	}
	p := GenerateParameterName("FORT_KEY")
	if !strings.HasPrefix(p, "/userland/fort-key-") || len(p) != len("/userland/fort-key-")+8 || Shape(manifest.ParameterName)(p) != nil {
		t.Fatalf("parameter %q", p)
	}
	if GenerateBucketName("X") == GenerateBucketName("X") {
		t.Fatal("tails repeat")
	}
}

func TestChecklistVariesByPropertyNotByProduct(t *testing.T) {
	plain := Checklist("DUMPS_S3", manifest.External{Kind: manifest.Bucket}, "b", "r")
	versioned := Checklist("OLD_S3", manifest.External{Kind: manifest.Bucket, Versioned: true}, "b", "r")
	deletes := Checklist("LANGFUSE_S3", manifest.External{Kind: manifest.Bucket, Delete: manifest.DeleteAll}, "b", "r")
	archive := Checklist("FORT_S3", manifest.External{Kind: manifest.Bucket, Versioned: true, Delete: "locks/*", NeverExpire: true}, "b", "r")
	store := Checklist("FORT_KEY", manifest.External{Kind: manifest.SecretStore}, "/p", "r")
	for name, text := range map[string]string{"plain": plain, "versioned": versioned, "delete": deletes, "archive": archive} {
		if !strings.Contains(text, "1. Make a bucket named b in region r.") || !strings.Contains(text, "4. Enter the endpoint URL") || !strings.Contains(text, `README.md, "Object store"`) {
			t.Errorf("%s lacks the fixed lines:\n%s", name, text)
		}
	}
	if strings.Contains(plain, "versioning") || !strings.Contains(plain, "must not be able to delete") || !strings.Contains(plain, "expires objects after the days you want") {
		t.Fatalf("plain:\n%s", plain)
	}
	if !strings.Contains(versioned, "Turn versioning on") || !strings.Contains(versioned, "must not be able to delete") || !strings.Contains(versioned, "expires old versions") {
		t.Fatalf("versioned:\n%s", versioned)
	}
	if !strings.Contains(archive, "Turn versioning on") || !strings.Contains(archive, "able to delete under locks/ and nowhere else") || !strings.Contains(archive, "Nothing may expire here") || strings.Contains(archive, "expires") {
		t.Fatalf("archive:\n%s", archive)
	}
	if strings.Contains(deletes, "versioning") || !strings.Contains(deletes, "must also be able to delete") || !strings.Contains(deletes, "deletes on its own schedule") || strings.Contains(deletes, "Nothing in userland deletes") {
		t.Fatalf("delete:\n%s", deletes)
	}
	if !strings.Contains(store, "SecureString parameter named /p in region r") || !strings.Contains(store, "ssm:GetParameter") || !strings.Contains(store, "kms:Decrypt on the aws/ssm key") || !strings.Contains(store, "3. Enter the access key id and its secret.") {
		t.Fatalf("store:\n%s", store)
	}
	if strings.Contains(plain+versioned+deletes+archive+store, "fort ") || strings.Contains(plain+versioned+deletes+archive+store, "restic") || strings.Contains(plain+versioned+deletes+archive+store, "langfuse") {
		t.Fatal("a product's name, or the tool behind it, is in a kind's text")
	}
	if !strings.Contains(Retention("X", manifest.External{Kind: manifest.Bucket}), "or accept that it grows") || !strings.Contains(Retention("X", manifest.External{Kind: manifest.Bucket, Versioned: true}), "pile up") || !strings.Contains(Retention("X", manifest.External{Kind: manifest.Bucket, Delete: manifest.DeleteAll}), "own schedule") {
		t.Fatal("Retention")
	}
	if never := Retention("X", manifest.External{Kind: manifest.Bucket, NeverExpire: true, Delete: "locks/*"}); !strings.Contains(never, "nothing may expire") || strings.Contains(never, "set a rule") {
		t.Fatalf("Retention where nothing may expire: %s", never)
	}
}
