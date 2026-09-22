package scaffold

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Abdullah0297445/userland/internal/manifest"
)

func TestInsertAppendsAProductTheRealManifestLoads(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("..", "..", "manifest.json"))
	if err != nil {
		t.Fatal(err)
	}
	out, err := insert(raw, "whoami", []string{"whoami", "whoami-worker"})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(string(out), string(raw[:len(raw)-8])) {
		t.Fatal("the text before the new block changed")
	}
	m, err := manifest.Parse(out)
	if err != nil {
		t.Fatal(err)
	}
	if m.Container("whoami") == nil || m.Container("whoami-worker") == nil || m.Container("whoami").Product != "whoami" {
		t.Fatalf("containers after insert: %v", m.Names())
	}
	if len(m.Products) != 7 || m.Container("metabase") == nil {
		t.Fatalf("products after insert: %d", len(m.Products))
	}
	if !strings.Contains(string(out), "    },\n    \"whoami\": {\n      \"containers\": {\n        \"whoami\": {\n          \"requires\": [],") {
		t.Fatalf("block is not indented like its neighbours:\n%s", out)
	}
}

func TestInsertIntoAnEmptyProductsObject(t *testing.T) {
	out, err := insert([]byte("{\n  \"products\": {}\n}\n"), "p", []string{"c"})
	if err != nil {
		t.Fatal(err)
	}
	m, err := manifest.Parse(out)
	if err != nil {
		t.Fatalf("%v\n%s", err, out)
	}
	if m.Container("c") == nil {
		t.Fatalf("no container c in\n%s", out)
	}
}

func TestInsertIgnoresBracesInsideStrings(t *testing.T) {
	raw := []byte("{\n  \"products\": {\n    \"a\": {\"containers\": {\"a\": {\"asks\": [{\"var\": \"X\", \"type\": \"text\", \"prompt\": \"a } brace { and \\\" quote\"}]}}}\n  }\n}\n")
	out, err := insert(raw, "b", []string{"b"})
	if err != nil {
		t.Fatal(err)
	}
	m, err := manifest.Parse(out)
	if err != nil {
		t.Fatalf("%v\n%s", err, out)
	}
	if m.Container("a") == nil || m.Container("b") == nil {
		t.Fatalf("containers: %v", m.Names())
	}
}

func TestInsertRefusesAManifestWithoutProducts(t *testing.T) {
	if _, err := insert([]byte(`{"things": {}}`), "p", []string{"c"}); err == nil {
		t.Fatal("accepted a manifest with no products key")
	}
}

func TestTemplateDefinesEveryContainerWithTheMergeLine(t *testing.T) {
	text := Template([]string{"a", "b"})
	for _, want := range []string{`{{ define "a" -}}`, `{{ define "b" -}}`, "image: " + Placeholder, "  <<: *userland-environment"} {
		if !strings.Contains(text, want) {
			t.Errorf("template lacks %q:\n%s", want, text)
		}
	}
	if strings.Count(text, "{{ end }}") != 2 {
		t.Fatalf("template ends %d defines", strings.Count(text, "{{ end }}"))
	}
}

func TestProductRefusesWhatExists(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "compose"), 0o755); err != nil {
		t.Fatal(err)
	}
	body := "{\n  \"products\": {\n    \"metabase\": {\"containers\": {\"metabase\": {}}}\n  }\n}\n"
	if err := os.WriteFile(filepath.Join(root, "manifest.json"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	cases := map[string][]string{
		"existing product":   {"metabase", "other"},
		"existing container": {"other", "metabase"},
		"bad product name":   {"Meta_base", "c"},
		"bad container name": {"p", "c_1"},
		"twice":              {"p", "c", "c"},
	}
	for name, args := range cases {
		if _, err := Product(root, args[0], args[1:]); err == nil {
			t.Errorf("%s: accepted %v", name, args)
		}
	}
	files, err := Product(root, "whoami", []string{"whoami"})
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 2 {
		t.Fatalf("wrote %v", files)
	}
	if _, err := os.Stat(filepath.Join(root, "compose", "whoami.yml")); err != nil {
		t.Fatal(err)
	}
	if _, err := Product(root, "whoami", []string{"again"}); err == nil {
		t.Fatal("wrote the same product twice")
	}
}
