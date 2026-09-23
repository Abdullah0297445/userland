package render

import (
	"os"
	"path/filepath"
	"reflect"
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
	with, err := templates.Body(m.Container("app"), "local", Set([]string{"app", "sidecar"}), nil)
	if err != nil {
		t.Fatal(err)
	}
	without, err := templates.Body(m.Container("app"), "local", Set([]string{"app"}), nil)
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

func TestAHelperTemplateIsIncludedAndIsNoContainer(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "compose"), 0o755); err != nil {
		t.Fatal(err)
	}
	template := "{{ define \"app.environment\" }}\n  SHARED: {{ .Name }}\n{{- end }}\n\n{{ define \"app\" -}}\nimage: x\nenvironment:\n  " + MergeLine + "\n{{ template \"app.environment\" . }}\n  OWN: yes\n{{ end }}\n"
	if err := os.WriteFile(filepath.Join(root, "compose", "app.yml"), []byte(template), 0o644); err != nil {
		t.Fatal(err)
	}
	m, err := manifest.Parse([]byte(`{"products": {"app": {"containers": {"app": {}}}}}`))
	if err != nil {
		t.Fatal(err)
	}
	templates, err := Load(root)
	if err != nil {
		t.Fatal(err)
	}
	if got := templates.Names(); !reflect.DeepEqual(got, []string{"app"}) {
		t.Fatalf("names %v", got)
	}
	body, err := templates.Body(m.Container("app"), "local", Set([]string{"app"}), nil)
	if err != nil {
		t.Fatal(err)
	}
	want := "image: x\nenvironment:\n  " + MergeLine + "\n  SHARED: app\n  OWN: yes"
	if body != want {
		t.Fatalf("body:\n%s", body)
	}
}

func TestMountsPutEveryListedFilesDirectoryUnderFilesReadOnly(t *testing.T) {
	got := Mounts("/srv/app/.env:/home/user/userland/.env:/srv/app/other.env: :relative")
	want := []string{
		"/home/user/userland:" + FilesRoot + "/home/user/userland:ro",
		"/srv/app:" + FilesRoot + "/srv/app:ro",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("mounts %v", got)
	}
	if Mounts("") != nil {
		t.Fatal("nothing kept is no mount")
	}
}
