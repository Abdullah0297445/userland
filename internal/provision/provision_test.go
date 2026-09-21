package provision

import (
	"reflect"
	"strings"
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

func TestClickHouseGrantsStayInsideTheDatabaseAndTheSystemTablesLangfuseReads(t *testing.T) {
	for _, g := range clickhouseGrants("probe") {
		inside := strings.Contains(g, ` ON "probe".* TO "probe"`)
		system := strings.Contains(g, " ON system.") && strings.HasSuffix(g, ` TO "probe"`)
		if !inside && !system {
			t.Errorf("grant reaches outside the database: %s", g)
		}
	}
	if got := clickhouseLiteral(`it's a \ back`); got != `'it\'s a \\ back'` {
		t.Fatalf("literal: %s", got)
	}
}
