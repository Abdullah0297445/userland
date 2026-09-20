package env

import (
	"os"
	"path/filepath"
	"testing"
)

func file(t *testing.T, body string) *File {
	t.Helper()
	path := filepath.Join(t.TempDir(), ".env")
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	f, err := Read(path)
	if err != nil {
		t.Fatal(err)
	}
	return f
}

func text(t *testing.T, f *File) string {
	t.Helper()
	if err := f.Write(); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(f.path)
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}

func TestSetReplacesInPlaceAndLeavesOtherLinesAlone(t *testing.T) {
	f := file(t, "# mine\nVISIBILITY=local\nMY_OWN=keep this\nUSERLAND_ON=a\n")
	f.Set("VISIBILITY", "public")
	f.Set("NEW=", "")
	f.Set("ADDED", "1")
	want := "# mine\nVISIBILITY=public\nMY_OWN=keep this\nUSERLAND_ON=a\nNEW==\nADDED=1\n"
	if got := text(t, f); got != want {
		t.Fatalf("got\n%s\nwant\n%s", got, want)
	}
}

func TestRenameMovesTheValueAndDropsTheOldLine(t *testing.T) {
	f := file(t, "A=1\nOLD=secret\nB=2\n")
	if !f.Rename("OLD", "NOW") {
		t.Fatal("rename reported nothing to do")
	}
	if f.Has("OLD") || f.Get("NOW") != "secret" {
		t.Fatalf("after rename: %q", text(t, f))
	}
	if f.Rename("OLD", "NOW") {
		t.Fatal("a second rename found something")
	}
}

func TestRenameKeepsAnExistingNewValue(t *testing.T) {
	f := file(t, "OLD=stale\nNOW=fresh\n")
	f.Rename("OLD", "NOW")
	if got := text(t, f); got != "NOW=fresh\n" {
		t.Fatalf("got %q", got)
	}
}

func TestRemove(t *testing.T) {
	f := file(t, "A=1\nGONE=x\nB=2\n")
	if !f.Remove("GONE") || f.Remove("GONE") {
		t.Fatal("remove did not report once")
	}
	if got := text(t, f); got != "A=1\nB=2\n" {
		t.Fatalf("got %q", got)
	}
}

func TestGetDoesNotMatchAPrefixOfALongerKey(t *testing.T) {
	f := file(t, "DOMAIN_SUFFIX=x\nDOMAIN=y\n")
	if f.Get("DOMAIN") != "y" {
		t.Fatalf("got %q", f.Get("DOMAIN"))
	}
}
