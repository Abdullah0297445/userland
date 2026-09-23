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
	var names, addresses []string
	width := 0
	for _, c := range m.All() {
		if !contains(on, c.Name) {
			continue
		}
		names = append(names, c.Name)
		addresses = append(addresses, address(c, e, on))
		width = max(width, len(c.Name))
	}
	if len(names) == 0 {
		return "The Contract: nothing is on."
	}
	lines := []string{"The Contract: what is on, and how to reach it.", ""}
	for i, name := range names {
		lines = append(lines, strings.TrimRight(fmt.Sprintf("  %-*s  %s", width, name, addresses[i]), " "))
	}
	return strings.Join(lines, "\n")
}

func address(c *manifest.Container, e *env.File, on []string) string {
	public := e.Get("VISIBILITY") == "public"
	switch {
	case c.Name == manifest.Proxy && public:
		return fmt.Sprintf("network %s, entrypoint %s, resolver %s, hosts NAME.%s", render.Network(c.Product), render.Secure, render.Resolver, e.Get("DOMAIN"))
	case c.Name == manifest.Proxy:
		return fmt.Sprintf("network %s, entrypoint %s, hosts NAME.localhost", render.Network(c.Product), render.Web)
	case c.Name == manifest.TransactionDoor || c.Name == manifest.SessionDoor:
		return fmt.Sprintf("%s:5432 on %s", c.Name, render.Network(c.Product))
	case c.HTTP == nil:
		return ""
	case !contains(on, manifest.Proxy):
		return fmt.Sprintf("http://127.0.0.1:%d", c.HTTP.Host)
	case public:
		return fmt.Sprintf("https://%s.%s", c.HTTP.Subdomain, e.Get("DOMAIN"))
	default:
		return fmt.Sprintf("http://%s.localhost", c.HTTP.Subdomain)
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
