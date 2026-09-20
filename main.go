package main

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	"github.com/Abdullah0297445/userland/internal/check"
	"github.com/Abdullah0297445/userland/internal/env"
	"github.com/Abdullah0297445/userland/internal/interview"
	"github.com/Abdullah0297445/userland/internal/manifest"
	"github.com/Abdullah0297445/userland/internal/provision"
	"github.com/Abdullah0297445/userland/internal/render"
)

const usage = `usage: userland [VERB] [ARGS]

  (no verb)              the interview: ask what is new, write .env, apply
  render                 write compose.yml from manifest.json, the templates and .env
  apply                  render, bring up, provision, print
  provision              converge the door auth and every switched-on database
  on CONTAINER...        switch containers on, then apply
  off CONTAINER...       switch containers off, then apply
  check [--write]        assert the manifest and templates hold; --write regenerates VARIABLES.md`

func main() {
	root, err := findRoot()
	must(err)
	if len(os.Args) < 2 {
		must(interviewThenApply(root))
		return
	}
	verb, args := os.Args[1], os.Args[2:]
	if verb == "check" {
		write := len(args) == 1 && args[0] == "--write"
		if len(args) > 0 && !write {
			fail(usage)
		}
		report, err := check.Run(root, write)
		for _, p := range report.Passed {
			say("ok: " + p)
		}
		for _, f := range report.Failures {
			say("FAIL: " + f)
		}
		must(err)
		return
	}
	m, err := manifest.Load(filepath.Join(root, "manifest.json"))
	must(err)
	e, err := env.Read(filepath.Join(root, ".env"))
	if err != nil {
		fail(fmt.Sprintf("no .env at %s; run ./bootstrap with no verb and the interview writes it", root))
	}
	must(migrate(m, e))
	must(e.Require("VISIBILITY"))
	switch verb {
	case "render":
		must(renderFile(m, e, root))
	case "apply":
		must(apply(m, e, root))
	case "provision":
		must(provisionAll(m, e))
	case "on":
		must(toggle(m, e, root, args, true))
	case "off":
		must(toggle(m, e, root, args, false))
	default:
		fail(usage)
	}
}

func interviewThenApply(root string) error {
	m, err := manifest.Load(filepath.Join(root, "manifest.json"))
	if err != nil {
		return err
	}
	path := filepath.Join(root, ".env")
	e, err := env.Read(path)
	if errors.Is(err, os.ErrNotExist) {
		e = env.New(path)
	} else if err != nil {
		return err
	}
	if err := migrate(m, e); err != nil {
		return err
	}
	result, err := interview.Run(m, e, os.Stdout)
	var refusal interview.Refusal
	if errors.As(err, &refusal) {
		return fmt.Errorf("%w\nnothing was written; run ./bootstrap again and pick a selection that runs", err)
	}
	if err != nil {
		return err
	}
	if result.Visibility != "" {
		say("visibility: " + result.Visibility)
	}
	if len(result.Offered) > 0 {
		say(fmt.Sprintf("switched on: %s", strings.Join(intersect(result.On, result.Offered), ", ")))
		if off := intersect(result.Off, result.Offered); len(off) > 0 {
			say(fmt.Sprintf("recorded as off: %s", strings.Join(off, ", ")))
		}
	}
	if len(result.Asked) > 0 {
		say(fmt.Sprintf("asked and written: %s", strings.Join(result.Asked, ", ")))
	}
	if !result.Changed() {
		say("nothing new: every container is decided and every variable the selection needs is in .env")
	}
	if err := e.Write(); err != nil {
		return err
	}
	return apply(m, e, root)
}

func migrate(m *manifest.Manifest, e *env.File) error {
	moved := interview.Migrate(m, e)
	if len(moved) == 0 {
		return nil
	}
	say(moved...)
	return e.Write()
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
	if contains(on, manifest.Postgres) {
		if err := compose(root, "up", "-d", "--wait", "--remove-orphans", manifest.Postgres); err != nil {
			return err
		}
		if err := provisionAll(m, e); err != nil {
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

func provisionAll(m *manifest.Manifest, e *env.File) error {
	report, err := provision.Door(e)
	say(report...)
	if err != nil {
		return err
	}
	report, err = provision.Databases(m, e.List("USERLAND_ON"), e)
	say(report...)
	return err
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
	if c.Postgres != nil {
		shared := false
		for _, other := range m.All() {
			if other.Name != c.Name && other.Postgres != nil && other.Postgres.Database == c.Postgres.Database && contains(on, other.Name) {
				shared = true
			}
		}
		if !shared {
			kept = append(kept, "database "+c.Postgres.Database)
		}
	}
	if len(kept) > 0 {
		say(fmt.Sprintf("%s is off and left behind: %s. Nothing is dropped unless you reclaim it.", c.Name, strings.Join(kept, ", ")))
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
			case !contains(on, manifest.Proxy):
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

func intersect(list, keep []string) []string {
	var out []string
	for _, x := range list {
		if contains(keep, x) {
			out = append(out, x)
		}
	}
	return out
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

func must(err error) {
	if err != nil {
		fail(err.Error())
	}
}
