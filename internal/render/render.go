package render

import (
	"bytes"
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"text/template"

	"github.com/Abdullah0297445/userland/internal/env"
	"github.com/Abdullah0297445/userland/internal/manifest"
)

type Input struct {
	Manifest   *manifest.Manifest
	On         []string
	Visibility string
	Env        *env.File
	Root       string
}

type data struct {
	Name       string
	Product    string
	Visibility string
	Tenant     *manifest.Tenant
	HTTP       *manifest.HTTP
}

func Render(in Input) ([]byte, error) {
	funcs := template.FuncMap{"ref": func(v string) string { return "${" + v + "}" }}
	tmpl, err := template.New("").Funcs(funcs).ParseGlob(filepath.Join(in.Root, "compose", "*.yml"))
	if err != nil {
		return nil, err
	}
	on := map[string]bool{}
	for _, name := range in.On {
		on[name] = true
	}
	var b strings.Builder
	b.WriteString("name: userland\n\nx-userland-environment: &userland-environment\n  TZ: UTC\n\nservices:\n")
	networks := map[string]bool{}
	volumes := map[string]bool{}
	for _, c := range in.Manifest.All() {
		if !on[c.Name] {
			continue
		}
		if tmpl.Lookup(c.Name) == nil {
			return nil, fmt.Errorf("compose/%s.yml defines no template %q", c.Product, c.Name)
		}
		fmt.Fprintf(&b, "  %s:\n", c.Name)
		line := func(s string) { fmt.Fprintf(&b, "    %s\n", s) }
		line("container_name: " + c.Name)
		line("restart: unless-stopped")
		if len(c.Requires) > 0 {
			line("depends_on:")
			for _, r := range c.Requires {
				line("  " + r + ":")
				line("    condition: service_healthy")
			}
		}
		nets := map[string]bool{c.Product: true}
		for _, dep := range append(append([]string{}, c.Requires...), c.Optional...) {
			if on[dep] {
				nets[in.Manifest.Container(dep).Product] = true
			}
		}
		line("networks:")
		for _, n := range sorted(nets) {
			line("  - " + n)
			networks[n] = true
		}
		if c.HTTP != nil && on["traefik"] {
			host := fmt.Sprintf("Host(`%s.localhost`)", c.HTTP.Subdomain)
			entrypoint := "web"
			if in.Visibility == "public" {
				host = fmt.Sprintf("Host(`%s.${DOMAIN}`)", c.HTTP.Subdomain)
				entrypoint = "websecure"
			}
			line("labels:")
			line(`  - "traefik.enable=true"`)
			line(fmt.Sprintf(`  - "traefik.http.routers.%s.rule=%s"`, c.Name, host))
			line(fmt.Sprintf(`  - "traefik.http.routers.%s.entrypoints=%s"`, c.Name, entrypoint))
			if in.Visibility == "public" {
				line(fmt.Sprintf(`  - "traefik.http.routers.%s.tls.certresolver=letsencrypt"`, c.Name))
			}
			line(fmt.Sprintf(`  - "traefik.http.services.%s.loadbalancer.server.port=%d"`, c.Name, c.HTTP.Container))
			line(fmt.Sprintf(`  - "traefik.docker.network=userland_%s"`, in.Manifest.Container("traefik").Product))
		}
		var ports []string
		for _, p := range c.Ports {
			ports = append(ports, fmt.Sprintf(`"%d:%d"`, p, p))
		}
		if c.HTTP != nil {
			portVar := envName(c.Name) + "_PORT"
			switch {
			case !on["traefik"]:
				ports = append(ports, fmt.Sprintf(`"127.0.0.1:%d:%d"`, c.HTTP.Host, c.HTTP.Container))
			case in.Env.Get(portVar) != "":
				ports = append(ports, fmt.Sprintf(`"127.0.0.1:${%s}:%d"`, portVar, c.HTTP.Container))
			}
		}
		if len(ports) > 0 {
			line("ports:")
			for _, p := range ports {
				line("  - " + p)
			}
		}
		if limit := envName(c.Name) + "_MEM_LIMIT"; in.Env.Get(limit) != "" {
			line(fmt.Sprintf("mem_limit: ${%s}", limit))
		}
		var body bytes.Buffer
		err := tmpl.ExecuteTemplate(&body, c.Name, data{c.Name, c.Product, in.Visibility, c.Tenant, c.HTTP})
		if err != nil {
			return nil, err
		}
		for _, l := range strings.Split(strings.Trim(body.String(), "\n"), "\n") {
			if strings.TrimSpace(l) == "" {
				continue
			}
			line(l)
		}
		for _, v := range c.Volumes {
			volumes[v] = true
		}
	}
	b.WriteString("\nnetworks:\n")
	for _, n := range sorted(networks) {
		fmt.Fprintf(&b, "  %s: {}\n", n)
	}
	if len(volumes) > 0 {
		b.WriteString("\nvolumes:\n")
		for _, v := range sorted(volumes) {
			fmt.Fprintf(&b, "  %s: {}\n", v)
		}
	}
	return []byte(b.String()), nil
}

func envName(container string) string {
	return strings.ToUpper(strings.ReplaceAll(container, "-", "_"))
}

func sorted(set map[string]bool) []string {
	var out []string
	for k := range set {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
