package contract

import (
	"fmt"
	"strings"

	"github.com/Abdullah0297445/userland/internal/env"
	"github.com/Abdullah0297445/userland/internal/manifest"
	"github.com/Abdullah0297445/userland/internal/render"
)

func DSN(user, password, host, database string) string {
	return fmt.Sprintf("postgresql://%s:%s@%s:5432/%s", user, password, host, database)
}

func Text(m *manifest.Manifest, e *env.File, on []string) string {
	postgres := network(m, manifest.Postgres, "postgres")
	traefik := network(m, manifest.Proxy, "traefik")
	public := e.Get("VISIBILITY") == "public"
	domain := e.Get("DOMAIN")
	var b strings.Builder
	w := func(format string, args ...any) { fmt.Fprintf(&b, format+"\n", args...) }
	w("The Contract: what a consumer needs to use userland, and nothing about how userland runs.")
	w("")
	w("Networks. A consumer joins one or both, declared external in its own compose file:")
	w("")
	w("    networks:")
	w("      %s:", postgres)
	w("        external: true")
	w("      %s:", traefik)
	w("        external: true")
	w("")
	w("Postgres, through a door. Both doors listen on 5432, and the DSN names one:")
	w("")
	w("    %s:5432", manifest.TransactionDoor)
	w("        The default. For a consumer that keeps no state on a connection between transactions.")
	w("    %s:5432", manifest.SessionDoor)
	w("        For a consumer that keeps state on a connection: SET, LISTEN, a session-scoped advisory")
	w("        lock, a prepared statement it reuses. Each consumer connection pins one Postgres")
	w("        connection for as long as it is held, so the consumer must release promptly.")
	w("")
	w("    A DSN reads %s.", DSN("NAME", "PASSWORD", manifest.TransactionDoor, "NAME"))
	w("    ./bootstrap postgres database add NAME makes the database and its user and prints the DSN, once.")
	w("    pg_dump, pgadmin and PostgREST bypass the doors and name %s:5432 directly.", manifest.Postgres)
	w("")
	w("traefik. Labels on the consumer's container route it; NAME and PORT are the consumer's own:")
	w("")
	w("    labels:")
	w("      - traefik.enable=true")
	if public {
		w("      - traefik.http.routers.NAME.rule=Host(`NAME.%s`)", domain)
		w("      - traefik.http.routers.NAME.entrypoints=%s", render.Secure)
		w("      - traefik.http.routers.NAME.tls.certresolver=%s", render.Resolver)
	} else {
		w("      - traefik.http.routers.NAME.rule=Host(`NAME.localhost`)")
		w("      - traefik.http.routers.NAME.entrypoints=%s", render.Web)
	}
	w("      - traefik.http.services.NAME.loadbalancer.server.port=PORT")
	w("      - traefik.docker.network=%s", traefik)
	w("")
	if public {
		w("    Visibility is public. Entrypoint %s is port 80 and redirects to %s, port 443, which carries", render.Web, render.Secure)
		w("    TLS from the certificate resolver %s. DOMAIN is %s.", render.Resolver, domain)
	} else {
		w("    Visibility is local. Entrypoint %s is port 80, plain HTTP, and every hostname ends in", render.Web)
		w("    .localhost. There is no %s entrypoint and no certificate.", render.Secure)
	}
	w("")
	var off []string
	for _, name := range []string{manifest.Postgres, manifest.TransactionDoor, manifest.SessionDoor, manifest.Proxy} {
		if m.Container(name) != nil && !contains(on, name) {
			off = append(off, name)
		}
	}
	if len(off) > 0 {
		w("Off right now, so no consumer reaches it until you switch it on: %s.", strings.Join(off, ", "))
	} else {
		w("Postgres, both doors and traefik are on.")
	}
	return strings.TrimRight(b.String(), "\n")
}

func network(m *manifest.Manifest, container, fallback string) string {
	if c := m.Container(container); c != nil {
		return render.Network(c.Product)
	}
	return render.Network(fallback)
}

func contains(list []string, item string) bool {
	for _, x := range list {
		if x == item {
			return true
		}
	}
	return false
}
