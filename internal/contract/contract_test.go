package contract

import (
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Abdullah0297445/userland/internal/env"
	"github.com/Abdullah0297445/userland/internal/manifest"
)

func TestDSN(t *testing.T) {
	got := DSN("app", "pw", manifest.SessionDoor, "app")
	if got != "postgresql://app:pw@pgbouncer-session:5432/app" {
		t.Fatal(got)
	}
}

func TestText(t *testing.T) {
	m, err := manifest.Load(filepath.Join("..", "..", "manifest.json"))
	if err != nil {
		t.Fatal(err)
	}
	e := env.New("")
	e.Set("VISIBILITY", "local")
	local := Text(m, e, []string{"postgres-18", "pgbouncer-transaction", "metabase", "traefik"})
	want := strings.Join([]string{
		"The Contract: what is on, and how to reach it.",
		"",
		"  metabase               http://metabase.localhost",
		"  pgbouncer-transaction  pgbouncer-transaction:5432 on userland_postgres",
		"  postgres-18",
		"  traefik                network userland_traefik, entrypoint web, hosts NAME.localhost",
	}, "\n")
	if local != want {
		t.Errorf("local:\n%s\nwant:\n%s", local, want)
	}

	e.Set("VISIBILITY", "public")
	e.Set("DOMAIN", "example.test")
	public := Text(m, e, []string{"postgres-18", "pgbouncer-session", "metabase", "traefik"})
	for _, line := range []string{
		"  metabase           https://metabase.example.test",
		"  pgbouncer-session  pgbouncer-session:5432 on userland_postgres",
		"  traefik            network userland_traefik, entrypoint websecure, resolver letsencrypt, hosts NAME.example.test",
	} {
		if !strings.Contains(public+"\n", line+"\n") {
			t.Errorf("public lacks %q:\n%s", line, public)
		}
	}
	if strings.Contains(public, "pgbouncer-transaction") {
		t.Errorf("public lists a container that is off:\n%s", public)
	}

	unproxied := Text(m, e, []string{"postgres-18", "metabase"})
	if loopback := fmt.Sprintf("http://127.0.0.1:%d", m.Container("metabase").HTTP.Host); !strings.Contains(unproxied, loopback) {
		t.Errorf("without traefik, metabase is not on %s:\n%s", loopback, unproxied)
	}

	if got := Text(m, e, nil); got != "The Contract: nothing is on." {
		t.Errorf("nothing on: %q", got)
	}
}
