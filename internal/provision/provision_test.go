package provision

import (
	"reflect"
	"testing"
)

func TestParseSettingsReadsOneSettingPerLine(t *testing.T) {
	got := parseSettings("statement_timeout=5min\nsearch_path=public, api\n")
	want := map[string]string{"statement_timeout": "5min", "search_path": "public, api"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	if got := parseSettings(""); len(got) != 0 {
		t.Fatalf("no rows should parse to no settings, got %v", got)
	}
}
