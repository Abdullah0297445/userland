package render

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Abdullah0297445/userland/internal/manifest"
)

func TestTemplatesReadWhichContainersAreOn(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "compose"), 0o755); err != nil {
		t.Fatal(err)
	}
	template := "{{ define \"app\" -}}\nimage: x\nenvironment:\n  " + MergeLine + "\n{{- if .On \"sidecar\" }}\n  MODE: external\n{{- else }}\n  MODE: internal\n{{- end }}\n{{ end }}\n"
	if err := os.WriteFile(filepath.Join(root, "compose", "app.yml"), []byte(template), 0o644); err != nil {
		t.Fatal(err)
	}
	m, err := manifest.Parse([]byte(`{"products": {"app": {"containers": {"app": {"optional": ["sidecar"]}, "sidecar": {"requires": ["app"]}}}}}`))
	if err != nil {
		t.Fatal(err)
	}
	templates, err := Load(root)
	if err != nil {
		t.Fatal(err)
	}
	with, err := templates.Body(m.Container("app"), "local", Set([]string{"app", "sidecar"}))
	if err != nil {
		t.Fatal(err)
	}
	without, err := templates.Body(m.Container("app"), "local", Set([]string{"app"}))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(with, "MODE: external") || strings.Contains(with, "MODE: internal") {
		t.Fatalf("with the sidecar on:\n%s", with)
	}
	if !strings.Contains(without, "MODE: internal") || strings.Contains(without, "MODE: external") {
		t.Fatalf("with the sidecar off:\n%s", without)
	}
}
