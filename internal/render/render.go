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

const (
	Project   = "userland"
	Anchor    = "userland-environment"
	MergeLine = "<<: *" + Anchor
	Resolver  = "letsencrypt"
	Web       = "web"
	Secure    = "websecure"
)

var Owned = []string{"container_name", "restart", "depends_on", "networks", "labels", "ports", "mem_limit"}

type Input struct {
	Manifest   *manifest.Manifest
	On         []string
	Visibility string
	Env        *env.File
	Root       string
}

type Templates struct {
	set *template.Template
}

type data struct {
	Name       string
	Product    string
	Visibility string
	Postgres   *manifest.Database
	HTTP       *manifest.HTTP
}

func Load(root string) (*Templates, error) {
	funcs := template.FuncMap{"ref": func(v string) string { return "${" + v + "}" }}
	set, err := template.New("").Funcs(funcs).ParseGlob(filepath.Join(root, "compose", "*.yml"))
	if err != nil {
		return nil, err
	}
	return &Templates{set: set}, nil
}

func (t *Templates) Names() []string {
	var names []string
	for _, tmpl := range t.set.Templates() {
		if name := tmpl.Name(); name != "" && !strings.HasSuffix(name, ".yml") {
			names = append(names, name)
		}
	}
	sort.Strings(names)
	return names
}

func (t *Templates) Has(name string) bool {
	return t.set.Lookup(name) != nil
}

func (t *Templates) Body(c *manifest.Container, visibility string) (string, error) {
	if !t.Has(c.Name) {
		return "", fmt.Errorf("compose/%s.yml defines no template %q", c.Product, c.Name)
	}
	var body bytes.Buffer
	if err := t.set.ExecuteTemplate(&body, c.Name, data{c.Name, c.Product, visibility, c.Postgres, c.HTTP}); err != nil {
		return "", err
	}
	var lines []string
	for _, l := range strings.Split(strings.Trim(body.String(), "\n"), "\n") {
		if strings.TrimSpace(l) != "" {
			lines = append(lines, l)
		}
	}
	return strings.Join(lines, "\n"), nil
}

func Render(in Input) ([]byte, error) {
	templates, err := Load(in.Root)
	if err != nil {
		return nil, err
	}
	on := map[string]bool{}
	for _, name := range in.On {
		on[name] = true
	}
	var b strings.Builder
	fmt.Fprintf(&b, "name: %s\n\nx-%s: &%s\n  TZ: UTC\n\nservices:\n", Project, Anchor, Anchor)
	networks := map[string]bool{}
	volumes := map[string]bool{}
	for _, c := range in.Manifest.All() {
		if !on[c.Name] {
			continue
		}
		body, err := templates.Body(c, in.Visibility)
		if err != nil {
			return nil, err
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
		if c.HTTP != nil && on[manifest.Proxy] {
			host := fmt.Sprintf("Host(`%s.localhost`)", c.HTTP.Subdomain)
			entrypoint := Web
			if in.Visibility == "public" {
				host = fmt.Sprintf("Host(`%s.${DOMAIN}`)", c.HTTP.Subdomain)
				entrypoint = Secure
			}
			line("labels:")
			line(`  - "traefik.enable=true"`)
			line(fmt.Sprintf(`  - "traefik.http.routers.%s.rule=%s"`, c.Name, host))
			line(fmt.Sprintf(`  - "traefik.http.routers.%s.entrypoints=%s"`, c.Name, entrypoint))
			if in.Visibility == "public" {
				line(fmt.Sprintf(`  - "traefik.http.routers.%s.tls.certresolver=%s"`, c.Name, Resolver))
			}
			line(fmt.Sprintf(`  - "traefik.http.services.%s.loadbalancer.server.port=%d"`, c.Name, c.HTTP.Container))
			line(fmt.Sprintf(`  - "traefik.docker.network=%s"`, Network(in.Manifest.Container(manifest.Proxy).Product)))
		}
		var ports []string
		for _, p := range c.Ports {
			ports = append(ports, fmt.Sprintf(`"%d:%d"`, p, p))
		}
		if c.HTTP != nil {
			portVar := PortVar(c.Name)
			switch {
			case !on[manifest.Proxy]:
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
		if limit := MemLimitVar(c.Name); in.Env.Get(limit) != "" {
			line(fmt.Sprintf("mem_limit: ${%s}", limit))
		}
		for _, l := range strings.Split(body, "\n") {
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

func Network(product string) string {
	return Project + "_" + product
}

func VolumeName(volume string) string {
	return Project + "_" + volume
}

func PortVar(container string) string {
	return envName(container) + "_PORT"
}

func MemLimitVar(container string) string {
	return envName(container) + "_MEM_LIMIT"
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
