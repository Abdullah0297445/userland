package check

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/Abdullah0297445/userland/internal/env"
	"github.com/Abdullah0297445/userland/internal/manifest"
	"github.com/Abdullah0297445/userland/internal/render"
)

const VariablesFile = "VARIABLES.md"

var (
	Visibilities = []string{"local", "public"}
	reference    = regexp.MustCompile(`\$\{([A-Z][A-Z0-9_]*)(:-([^}]*))?\}`)
	topLevelKey  = regexp.MustCompile(`^([a-z_]+):`)
)

type Report struct {
	Passed   []string
	Failures []string
}

func (r *Report) pass(format string, args ...any) {
	r.Passed = append(r.Passed, fmt.Sprintf(format, args...))
}

func (r *Report) fail(format string, args ...any) {
	message := fmt.Sprintf(format, args...)
	for _, f := range r.Failures {
		if f == message {
			return
		}
	}
	r.Failures = append(r.Failures, message)
}

func Run(root string, write bool) (*Report, error) {
	r := &Report{}
	m, err := manifest.Load(filepath.Join(root, "manifest.json"))
	if err != nil {
		return r, err
	}
	r.pass("manifest.json loads, with no duplicate container, volume or loopback port")
	templates, err := render.Load(root)
	if err != nil {
		return r, err
	}
	checkTemplateNames(r, m, templates)
	bodies := checkTemplateBodies(r, m, templates)
	checkReferences(r, m, bodies)
	if err := checkVariables(r, root, m, bodies, write); err != nil {
		return r, err
	}
	for _, visibility := range Visibilities {
		if err := checkRendered(r, root, m, visibility); err != nil {
			return r, err
		}
	}
	if len(r.Failures) > 0 {
		return r, fmt.Errorf("%d check(s) failed", len(r.Failures))
	}
	return r, nil
}

func checkTemplateNames(r *Report, m *manifest.Manifest, templates *render.Templates) {
	clean := true
	for _, c := range m.All() {
		if !templates.Has(c.Name) {
			r.fail("compose/%s.yml defines no template %q", c.Product, c.Name)
			clean = false
		}
	}
	for _, name := range templates.Names() {
		if m.Container(name) == nil {
			r.fail("template %q matches no container in manifest.json", name)
			clean = false
		}
	}
	if clean {
		r.pass("every container has a template and every template is a container")
	}
}

type body struct {
	container  string
	visibility string
	text       string
}

func checkTemplateBodies(r *Report, m *manifest.Manifest, templates *render.Templates) []body {
	owned := map[string]bool{}
	for _, k := range render.Owned {
		owned[k] = true
	}
	var bodies []body
	clean := true
	all := render.Set(m.Names())
	for _, c := range m.All() {
		if !templates.Has(c.Name) {
			continue
		}
		for _, visibility := range Visibilities {
			text, err := templates.Body(c, visibility, all, render.Mounts(checkPaths(c)))
			if err != nil {
				r.fail("template %s (%s): %v", c.Name, visibility, err)
				clean = false
				continue
			}
			bodies = append(bodies, body{c.Name, visibility, text})
			lines := strings.Split(text, "\n")
			environment := -1
			for i, l := range lines {
				match := topLevelKey.FindStringSubmatch(l)
				if match == nil {
					continue
				}
				if owned[match[1]] {
					r.fail("template %s writes %s, which the generator owns", c.Name, match[1])
					clean = false
				}
				if match[1] == "environment" {
					environment = i
				}
			}
			switch {
			case environment < 0:
				r.fail("template %s (%s) has no environment block, so TZ cannot reach it", c.Name, visibility)
				clean = false
			case environment+1 >= len(lines) || strings.TrimSpace(lines[environment+1]) != render.MergeLine:
				r.fail("template %s (%s): environment must open with %q", c.Name, visibility, render.MergeLine)
				clean = false
			}
		}
	}
	if clean {
		r.pass("no template writes a generator-owned key, and every environment opens with %q", render.MergeLine)
	}
	return bodies
}

func checkReferences(r *Report, m *manifest.Manifest, bodies []body) {
	known := map[string]bool{}
	for _, c := range m.All() {
		for _, a := range c.AllAsks() {
			known[a.Var] = true
		}
	}
	clean := true
	for _, b := range bodies {
		for _, match := range reference.FindAllStringSubmatch(b.text, -1) {
			if match[2] == "" && !known[match[1]] {
				r.fail("template %s reads ${%s}, which nothing asks and no default covers", b.container, match[1])
				clean = false
			}
		}
	}
	if clean {
		r.pass("every variable a template reads without a default is asked, is a database password, or is one an external kind supplies")
	}
}

func checkVariables(r *Report, root string, m *manifest.Manifest, bodies []body, write bool) error {
	want := Variables(m, bodies)
	path := filepath.Join(root, VariablesFile)
	if write {
		if err := os.WriteFile(path, []byte(want), 0o644); err != nil {
			return err
		}
		r.pass("%s written", VariablesFile)
		return nil
	}
	have, err := os.ReadFile(path)
	if err != nil || string(have) != want {
		r.fail("%s is stale; run `./bootstrap check --write` and commit it", VariablesFile)
		return nil
	}
	r.pass("%s is current", VariablesFile)
	return nil
}

type composeConfig struct {
	Services map[string]struct {
		Environment map[string]*string `json:"environment"`
		Healthcheck *struct {
			Test []string `json:"test"`
		} `json:"healthcheck"`
		Volumes []struct {
			Type   string `json:"type"`
			Source string `json:"source"`
		} `json:"volumes"`
	} `json:"services"`
	Volumes map[string]json.RawMessage `json:"volumes"`
}

func checkRendered(r *Report, root string, m *manifest.Manifest, visibility string) error {
	synthetic := env.New("")
	for _, c := range m.All() {
		if c.HTTP != nil {
			synthetic.Set(render.PortVar(c.Name), fmt.Sprint(c.HTTP.Host))
		}
		if c.Files != "" {
			synthetic.Set(c.Files, checkPaths(c))
		}
		synthetic.Set(render.MemLimitVar(c.Name), "1g")
	}
	out, err := render.Render(render.Input{Manifest: m, On: m.Names(), Visibility: visibility, Env: synthetic, Root: root})
	if err != nil {
		r.fail("render with everything on (%s): %v", visibility, err)
		return nil
	}
	config, err := composeConfigOf(root, out, synthetic)
	if err != nil {
		r.fail("docker compose config rejects the render with everything on (%s): %v", visibility, err)
		return nil
	}
	r.pass("docker compose config accepts the render with everything on (%s)", visibility)
	clean := true
	for _, name := range m.Names() {
		s, ok := config.Services[name]
		if !ok {
			r.fail("%s: rendered file has no service %s", visibility, name)
			clean = false
			continue
		}
		if tz := s.Environment["TZ"]; tz == nil || *tz != "UTC" {
			r.fail("%s: %s runs without TZ=UTC", visibility, name)
			clean = false
		}
	}
	if clean {
		r.pass("%s: TZ=UTC on every service", visibility)
	}
	clean = true
	for name := range m.Required() {
		s := config.Services[name]
		if s.Healthcheck == nil || len(s.Healthcheck.Test) == 0 {
			r.fail("%s: %s is required by another container but has no healthcheck", visibility, name)
			clean = false
		}
	}
	if clean {
		r.pass("%s: a healthcheck on every container something requires", visibility)
	}
	clean = true
	for _, c := range m.All() {
		mounted := map[string]bool{}
		for _, v := range config.Services[c.Name].Volumes {
			if v.Type == "volume" {
				mounted[v.Source] = true
			}
		}
		declared := map[string]bool{}
		for _, v := range c.Volumes {
			declared[v] = true
			if !mounted[v] {
				r.fail("%s: %s declares volume %s in manifest.json but its template does not mount it", visibility, c.Name, v)
				clean = false
			}
		}
		for _, dep := range append(append([]string{}, c.Requires...), c.Optional...) {
			for _, v := range m.Container(dep).Volumes {
				declared[v] = true
			}
		}
		for v := range mounted {
			if !declared[v] {
				r.fail("%s: %s mounts volume %s, which manifest.json declares neither on it nor on a container it depends on", visibility, c.Name, v)
				clean = false
			}
			if _, top := config.Volumes[v]; !top {
				r.fail("%s: volume %s is mounted but not in the top-level volumes block", visibility, v)
				clean = false
			}
		}
	}
	if clean {
		r.pass("%s: every named volume is declared on the container that mounts it or on one it depends on, and every declared volume is mounted", visibility)
	}
	return nil
}

func checkPaths(c *manifest.Container) string {
	if c.Files == "" {
		return ""
	}
	return "/etc/" + c.Name + "-one/kept.env:/etc/" + c.Name + "-two/kept.env"
}

func composeConfigOf(root string, rendered []byte, synthetic *env.File) (*composeConfig, error) {
	dir, err := os.MkdirTemp("", "userland-check-")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(dir)
	file := filepath.Join(dir, "compose.yml")
	if err := os.WriteFile(file, rendered, 0o644); err != nil {
		return nil, err
	}
	empty := filepath.Join(dir, "empty.env")
	if err := os.WriteFile(empty, nil, 0o644); err != nil {
		return nil, err
	}
	cmd := exec.Command("docker", "compose", "--project-directory", root, "--env-file", empty, "-f", file, "config", "--format", "json")
	cmd.Env = append(os.Environ(), placeholders(rendered, synthetic)...)
	out, err := cmd.Output()
	if err != nil {
		if exit, ok := err.(*exec.ExitError); ok {
			return nil, fmt.Errorf("%s", strings.TrimSpace(string(exit.Stderr)))
		}
		return nil, err
	}
	var config composeConfig
	if err := json.Unmarshal(out, &config); err != nil {
		return nil, fmt.Errorf("parsing docker compose config: %w", err)
	}
	return &config, nil
}

func placeholders(rendered []byte, synthetic *env.File) []string {
	seen := map[string]bool{}
	var vars []string
	for _, match := range reference.FindAllStringSubmatch(string(rendered), -1) {
		name := match[1]
		if seen[name] {
			continue
		}
		seen[name] = true
		value := synthetic.Get(name)
		if value == "" {
			value = "placeholder"
		}
		vars = append(vars, name+"="+value)
	}
	sort.Strings(vars)
	return vars
}
