package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	"github.com/Abdullah0297445/userland/internal/env"
	"github.com/Abdullah0297445/userland/internal/manifest"
	"github.com/Abdullah0297445/userland/internal/provision"
	"github.com/Abdullah0297445/userland/internal/render"
)

func main() {
	if len(os.Args) < 2 {
		fail("usage: userland render | apply | provision | on CONTAINER... | off CONTAINER...")
	}
	root, err := findRoot()
	check(err)
	m, err := manifest.Load(filepath.Join(root, "manifest.json"))
	check(err)
	e, err := env.Read(filepath.Join(root, ".env"))
	check(err)
	check(e.Require("VISIBILITY"))

	switch os.Args[1] {
	case "render":
		check(renderFile(m, e, root))
	case "apply":
		check(apply(m, e, root))
	case "provision":
		report, err := provision.Tenants(m, e.List("USERLAND_ON"), e)
		say(report...)
		check(err)
	case "on":
		check(toggle(m, e, root, os.Args[2:], true))
	case "off":
		check(toggle(m, e, root, os.Args[2:], false))
	default:
		fail("unknown subcommand " + os.Args[1])
	}
}

func renderFile(m *manifest.Manifest, e *env.File, root string) error {
	on := e.List("USERLAND_ON")
	verdict := m.Validate(on)
	if len(verdict.Refusals) > 0 {
		return fmt.Errorf("refused:\n  %s", strings.Join(verdict.Refusals, "\n  "))
	}
	out, err := render.Render(render.Input{Manifest: m, On: on, Visibility: e.Get("VISIBILITY"), Env: e, Root: root})
	if err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(root, "compose.yml"), out, 0o644); err != nil {
		return err
	}
	for _, w := range verdict.Warnings {
		say("warning: " + w)
	}
	say(fmt.Sprintf("rendered compose.yml with %d containers: %s", len(on), strings.Join(on, ", ")))
	return nil
}

func apply(m *manifest.Manifest, e *env.File, root string) error {
	if err := renderFile(m, e, root); err != nil {
		return err
	}
	on := e.List("USERLAND_ON")
	if hasTenant(m, on) {
		if err := compose(root, "up", "-d", "--wait", "postgres-18"); err != nil {
			return err
		}
		report, err := provision.Tenants(m, on, e)
		say(report...)
		if err != nil {
			return err
		}
	}
	if err := compose(root, "up", "-d", "--remove-orphans"); err != nil {
		return err
	}
	if err := compose(root, "ps", "--format", "table {{.Name}}\t{{.Status}}\t{{.Ports}}"); err != nil {
		return err
	}
	closing(m, e, on)
	return nil
}

func toggle(m *manifest.Manifest, e *env.File, root string, names []string, turnOn bool) error {
	if len(names) == 0 {
		return fmt.Errorf("name at least one container")
	}
	on := e.List("USERLAND_ON")
	off := e.List("USERLAND_OFF")
	for _, name := range names {
		if m.Container(name) == nil {
			return fmt.Errorf("%s is not a container the manifest knows", name)
		}
	}
	if !turnOn {
		remaining := without(on, names)
		for _, name := range names {
			var blocked []string
			for _, dependent := range m.Blocking(name) {
				if contains(remaining, dependent) {
					blocked = append(blocked, dependent)
				}
			}
			if len(blocked) > 0 {
				return fmt.Errorf("refused: %s is blocking %s; switch them off first", name, strings.Join(blocked, ", "))
			}
		}
		on, off = remaining, union(off, names)
	} else {
		on, off = union(on, names), without(off, names)
	}
	verdict := m.Validate(on)
	if len(verdict.Refusals) > 0 {
		return fmt.Errorf("refused:\n  %s", strings.Join(verdict.Refusals, "\n  "))
	}
	e.Set("USERLAND_ON", strings.Join(on, ","))
	e.Set("USERLAND_OFF", strings.Join(off, ","))
	if err := e.Write(); err != nil {
		return err
	}
	if err := apply(m, e, root); err != nil {
		return err
	}
	if !turnOn {
		for _, name := range names {
			leftBehind(m, on, m.Container(name))
		}
	}
	return nil
}

func leftBehind(m *manifest.Manifest, on []string, c *manifest.Container) {
	var kept []string
	for _, v := range c.Volumes {
		kept = append(kept, "volume "+v)
	}
	if c.Tenant != nil {
		shared := false
		for _, other := range m.All() {
			if other.Name != c.Name && other.Tenant != nil && other.Tenant.Name == c.Tenant.Name && contains(on, other.Name) {
				shared = true
			}
		}
		if !shared {
			kept = append(kept, "database "+c.Tenant.Name)
		}
	}
	if len(kept) > 0 {
		say(fmt.Sprintf("%s is off and left behind: %s. Reclaim is not part of this skeleton.", c.Name, strings.Join(kept, ", ")))
	}
}

func closing(m *manifest.Manifest, e *env.File, on []string) {
	var keep []string
	for _, c := range m.All() {
		if !contains(on, c.Name) {
			continue
		}
		if c.HTTP != nil {
			switch {
			case !contains(on, "traefik"):
				say(fmt.Sprintf("%s: http://127.0.0.1:%d", c.Name, c.HTTP.Host))
			case e.Get("VISIBILITY") == "public":
				say(fmt.Sprintf("%s: https://%s.%s", c.Name, c.HTTP.Subdomain, e.Get("DOMAIN")))
			default:
				say(fmt.Sprintf("%s: http://%s.localhost", c.Name, c.HTTP.Subdomain))
			}
		}
		for _, a := range c.Asks {
			if a.Keep {
				keep = append(keep, a.Var)
			}
		}
	}
	if len(keep) > 0 {
		say("copy these lines of .env somewhere off this machine; they cannot be regenerated: " + strings.Join(keep, ", "))
	}
}

func hasTenant(m *manifest.Manifest, on []string) bool {
	for _, name := range on {
		if c := m.Container(name); c != nil && c.Tenant != nil {
			return true
		}
	}
	return false
}

func compose(root string, args ...string) error {
	cmd := exec.Command("docker", append([]string{"compose"}, args...)...)
	cmd.Dir = root
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	say("$ docker compose " + strings.Join(args, " "))
	return cmd.Run()
}

func findRoot() (string, error) {
	dir, err := os.Getwd()
	if err != nil {
		return "", err
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "manifest.json")); err == nil {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", fmt.Errorf("no manifest.json here or above")
		}
		dir = parent
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

func without(list, drop []string) []string {
	var out []string
	for _, x := range list {
		if !contains(drop, x) {
			out = append(out, x)
		}
	}
	return out
}

func union(list, add []string) []string {
	out := append([]string{}, list...)
	for _, x := range add {
		if !contains(out, x) {
			out = append(out, x)
		}
	}
	sort.Strings(out)
	return out
}

func say(lines ...string) {
	for _, l := range lines {
		fmt.Println(l)
	}
}

func fail(msg string) {
	fmt.Fprintln(os.Stderr, msg)
	os.Exit(1)
}

func check(err error) {
	if err != nil {
		fail(err.Error())
	}
}
