package fort

import (
	"archive/tar"
	"bytes"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/Abdullah0297445/userland/internal/env"
)

func coordinates(t *testing.T) *env.File {
	t.Helper()
	e := env.New(filepath.Join(t.TempDir(), ".env"))
	for name, value := range map[string]string{
		"FORT_S3_BUCKET":             "userland-fort-s3-0a1b2c3d",
		"FORT_S3_REGION":             "eu-west-1",
		"FORT_S3_ENDPOINT":           "https://s3.eu-west-1.amazonaws.com/",
		"FORT_S3_ACCESS_KEY_ID":      "bucket-id",
		"FORT_S3_SECRET_ACCESS_KEY":  "bucket-secret",
		"FORT_KEY_PROVIDER":          "ssm",
		"FORT_KEY_NAME":              "/userland/fort-key-0a1b2c3d",
		"FORT_KEY_REGION":            "eu-west-1",
		"FORT_KEY_ACCESS_KEY_ID":     "store-id",
		"FORT_KEY_SECRET_ACCESS_KEY": "store-secret",
	} {
		e.Set(name, value)
	}
	return e
}

func TestTheStoresKeyNeverTravelsUnderAWSNames(t *testing.T) {
	t.Setenv("AWS_ENDPOINT_URL", "")
	lines, err := Environment(coordinates(t))
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]string{}
	for _, line := range lines {
		name, value, _ := strings.Cut(line, "=")
		got[name] = value
	}
	want := map[string]string{
		"RESTIC_REPOSITORY":          "s3:https://s3.eu-west-1.amazonaws.com/userland-fort-s3-0a1b2c3d",
		"RESTIC_PASSWORD_COMMAND":    "/usr/local/bin/fort-key",
		"RESTIC_HOST":                "fort",
		"AWS_ACCESS_KEY_ID":          "bucket-id",
		"AWS_SECRET_ACCESS_KEY":      "bucket-secret",
		"AWS_DEFAULT_REGION":         "eu-west-1",
		"FORT_KEY_PROVIDER":          "ssm",
		"FORT_KEY_NAME":              "/userland/fort-key-0a1b2c3d",
		"FORT_KEY_REGION":            "eu-west-1",
		"FORT_KEY_ACCESS_KEY_ID":     "store-id",
		"FORT_KEY_SECRET_ACCESS_KEY": "store-secret",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("environment %v", got)
	}
}

func TestEnvironmentRefusesWithoutEveryCoordinate(t *testing.T) {
	for _, name := range append(BucketVariables(), StoreVariables()...) {
		e := coordinates(t)
		e.Remove(name)
		if _, err := Environment(e); err == nil || !strings.Contains(err.Error(), name) {
			t.Errorf("without %s: %v", name, err)
		}
	}
}

func TestKeptReadsTheColonSeparatedList(t *testing.T) {
	e := coordinates(t)
	if Kept(e) != nil {
		t.Fatal("nothing kept yet")
	}
	Keep(e, []string{"/srv/app/.env", "/home/user/userland/.env"})
	if got := Kept(e); !reflect.DeepEqual(got, []string{"/srv/app/.env", "/home/user/userland/.env"}) {
		t.Fatalf("kept %v", got)
	}
	if e.Get(Files) != "/srv/app/.env:/home/user/userland/.env" {
		t.Fatalf("written as %q", e.Get(Files))
	}
	lines, err := Environment(e)
	if err != nil {
		t.Fatal(err)
	}
	if !contains(lines, Files+"=/srv/app/.env:/home/user/userland/.env") {
		t.Fatalf("the list does not reach the container: %v", lines)
	}
}

func TestUntarPutsEachFileBackWithItsModeAndTime(t *testing.T) {
	when := time.Date(2026, 9, 20, 10, 11, 12, 0, time.UTC)
	var archive bytes.Buffer
	w := tar.NewWriter(&archive)
	for _, entry := range []struct {
		name string
		mode int64
		body string
	}{
		{"srv/app/", 0o755, ""},
		{"srv/app/.env", 0o600, "SECRET=one\n"},
		{"etc/other.env", 0o644, "TOKEN=two\n"},
	} {
		header := &tar.Header{Name: entry.name, Mode: entry.mode, Size: int64(len(entry.body)), ModTime: when, Typeflag: tar.TypeReg}
		if strings.HasSuffix(entry.name, "/") {
			header.Typeflag, header.Size = tar.TypeDir, 0
		}
		if err := w.WriteHeader(header); err != nil {
			t.Fatal(err)
		}
		if _, err := w.Write([]byte(entry.body)); err != nil {
			t.Fatal(err)
		}
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	target := t.TempDir()
	written, err := untar(&archive, target)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(written, []string{filepath.Join(target, "srv/app/.env"), filepath.Join(target, "etc/other.env")}) {
		t.Fatalf("written %v", written)
	}
	body, err := os.ReadFile(filepath.Join(target, "srv/app/.env"))
	if err != nil || string(body) != "SECRET=one\n" {
		t.Fatalf("contents %q %v", body, err)
	}
	info, err := os.Stat(filepath.Join(target, "srv/app/.env"))
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("mode %v; a restored .env must not widen", info.Mode().Perm())
	}
	if !info.ModTime().UTC().Equal(when) {
		t.Fatalf("mtime %v, want %v", info.ModTime().UTC(), when)
	}
}

func TestUntarStaysUnderTheTarget(t *testing.T) {
	var archive bytes.Buffer
	w := tar.NewWriter(&archive)
	body := "stolen\n"
	if err := w.WriteHeader(&tar.Header{Name: "../../escaped.env", Mode: 0o644, Size: int64(len(body)), Typeflag: tar.TypeReg}); err != nil {
		t.Fatal(err)
	}
	w.Write([]byte(body))
	w.Close()
	target := t.TempDir()
	written, err := untar(&archive, target)
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range written {
		if !strings.HasPrefix(path, target) {
			t.Fatalf("%s is outside %s", path, target)
		}
	}
}

func contains(list []string, item string) bool {
	for _, x := range list {
		if x == item {
			return true
		}
	}
	return false
}
