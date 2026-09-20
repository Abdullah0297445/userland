package contract

import (
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

func TestTextInEachVisibility(t *testing.T) {
	m, err := manifest.Load(filepath.Join("..", "..", "manifest.json"))
	if err != nil {
		t.Fatal(err)
	}
	e := env.New("")
	e.Set("VISIBILITY", "local")
	local := Text(m, e, []string{"postgres-18", "pgbouncer-transaction", "metabase", "traefik"})
	for _, want := range []string{
		"      userland_postgres:\n        external: true",
		"      userland_traefik:\n        external: true",
		"pgbouncer-transaction:5432",
		"pgbouncer-session:5432",
		"Host(`NAME.localhost`)",
		"entrypoints=web\n",
		"traefik.docker.network=userland_traefik",
		"Off right now, so no consumer reaches it until you switch it on: pgbouncer-session.",
	} {
		if !strings.Contains(local, want) {
			t.Errorf("local contract lacks %q:\n%s", want, local)
		}
	}
	if strings.Contains(local, "certresolver") {
		t.Error("local contract names a certificate resolver")
	}
	e.Set("VISIBILITY", "public")
	e.Set("DOMAIN", "example.test")
	public := Text(m, e, []string{"postgres-18", "pgbouncer-transaction", "pgbouncer-session", "metabase", "traefik"})
	for _, want := range []string{
		"Host(`NAME.example.test`)",
		"entrypoints=websecure",
		"tls.certresolver=letsencrypt",
		"DOMAIN is example.test.",
		"Postgres, both doors and traefik are on.",
	} {
		if !strings.Contains(public, want) {
			t.Errorf("public contract lacks %q:\n%s", want, public)
		}
	}
}
