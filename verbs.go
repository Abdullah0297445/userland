package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/Abdullah0297445/userland/internal/contract"
	"github.com/Abdullah0297445/userland/internal/env"
	"github.com/Abdullah0297445/userland/internal/fort"
	"github.com/Abdullah0297445/userland/internal/interview"
	"github.com/Abdullah0297445/userland/internal/manifest"
	"github.com/Abdullah0297445/userland/internal/provision"
	"github.com/Abdullah0297445/userland/internal/render"
)

func onCommand(root string) *cobra.Command {
	return verb(root, "on NAME...", "Switch containers on, then apply. A product's name opens its gate.", operate, cobra.MinimumNArgs(1), on)
}

func offCommand(root string) *cobra.Command {
	return verb(root, "off CONTAINER...", "Switch containers off, then apply. Refuses while one is blocking another; offers to reclaim what is left.", operate, cobra.MinimumNArgs(1), off)
}

func setCommand(root string) *cobra.Command {
	return verb(root, "set VAR", "Ask one variable again, or VISIBILITY, then apply.", operate, cobra.ExactArgs(1), func(m *manifest.Manifest, e *env.File, root string, args []string) error {
		return set(m, e, root, args[0])
	})
}

func reclaimCommand(root string) *cobra.Command {
	return verb(root, "reclaim CONTAINER...", "Drop the volume and the database a switched-off container left, after naming them and asking.", operate, cobra.MinimumNArgs(1), reclaim)
}

func contractCommand(root string) *cobra.Command {
	return verb(root, "contract", "Print the Contract: what is on, and how to reach it.", operate, cobra.NoArgs, func(m *manifest.Manifest, e *env.File, _ string, _ []string) error {
		say(contract.Text(m, e, e.List(interview.On)))
		return nil
	})
}

func applyCommand(root string) *cobra.Command {
	return verb(root, "apply", "Render, bring up, provision, print.", operate, cobra.NoArgs, func(m *manifest.Manifest, e *env.File, root string, _ []string) error {
		return apply(m, e, root, nil)
	})
}

func renderCommand(root string) *cobra.Command {
	return verb(root, "render", "Write compose.yml from manifest.json, the templates and .env, and stop.", operate, cobra.NoArgs, func(m *manifest.Manifest, e *env.File, root string, _ []string) error {
		return renderFile(m, e, root)
	})
}

func provisionCommand(root string) *cobra.Command {
	return verb(root, "provision", "Converge the door auth and every switched-on database, on Postgres and ClickHouse, and nothing else.", operate, cobra.NoArgs, func(m *manifest.Manifest, e *env.File, _ string, _ []string) error {
		return provisionAll(m, e)
	})
}

func on(m *manifest.Manifest, e *env.File, root string, names []string) error {
	on, off := e.List(interview.On), e.List(interview.Off)
	before := append([]string{}, on...)
	for _, name := range names {
		switch {
		case m.Container(name) != nil:
			on, off = union(on, []string{name}), without(off, []string{name})
		case m.Products[name] != nil:
			containers := m.ProductContainers(name)
			picked, err := interview.Gate(m, name, containers, on, "Ticked is on and unticked is off. What is on now is ticked already.")
			if err != nil {
				return err
			}
			for _, c := range containers {
				if contains(picked, c.Name) {
					on, off = union(on, []string{c.Name}), without(off, []string{c.Name})
				} else {
					on, off = without(on, []string{c.Name}), union(off, []string{c.Name})
				}
			}
		default:
			return fmt.Errorf("%s is neither a container nor a product the manifest knows", name)
		}
	}
	turnedOff := without(before, on)
	if err := refuseBlocking(m, on, turnedOff); err != nil {
		return err
	}
	if err := refuse(m, on); err != nil {
		return err
	}
	e.Set(interview.On, strings.Join(on, ","))
	e.Set(interview.Off, strings.Join(off, ","))
	asked, err := interview.Variables(m, e, os.Stdout)
	if err != nil {
		return err
	}
	if turnedOn := without(on, before); len(turnedOn) > 0 {
		say("switched on: " + strings.Join(turnedOn, ", "))
	} else {
		say("nothing new: everything named was on already")
	}
	if len(turnedOff) > 0 {
		say("switched off: " + strings.Join(turnedOff, ", "))
	}
	if len(asked) > 0 {
		say("asked and written: " + strings.Join(asked, ", "))
	}
	if err := e.Write(); err != nil {
		return err
	}
	if err := apply(m, e, root, asked); err != nil {
		return err
	}
	return offerReclaim(m, on, turnedOff)
}

func off(m *manifest.Manifest, e *env.File, root string, names []string) error {
	on, off := e.List(interview.On), e.List(interview.Off)
	for _, name := range names {
		if m.Container(name) == nil {
			return fmt.Errorf("%s is not a container the manifest knows", name)
		}
	}
	remaining := without(on, names)
	if err := refuseBlocking(m, remaining, names); err != nil {
		return err
	}
	if err := refuse(m, remaining); err != nil {
		return err
	}
	e.Set(interview.On, strings.Join(remaining, ","))
	e.Set(interview.Off, strings.Join(union(off, names), ","))
	if err := e.Write(); err != nil {
		return err
	}
	if err := apply(m, e, root, nil); err != nil {
		return err
	}
	return offerReclaim(m, remaining, names)
}

func set(m *manifest.Manifest, e *env.File, root string, variable string) error {
	on := e.List(interview.On)
	switch variable {
	case interview.Visibility:
		v, err := interview.SelectVisibility()
		if err != nil {
			return err
		}
		e.Set(variable, v)
	case interview.On, interview.Off:
		return fmt.Errorf("%s is the selection; ./bootstrap on and off change it", variable)
	default:
		c, a, ok := m.AskFor(variable)
		if !ok {
			return fmt.Errorf("%s is not a variable the manifest asks; VARIABLES.md lists every variable, and an optional one is a line you add to .env by hand", variable)
		}
		if !contains(on, c.Name) {
			return fmt.Errorf("%s is asked for %s, which is off", variable, c.Name)
		}
		if !a.Applies(e.Get(interview.Visibility), e.Get) {
			return fmt.Errorf("%s is asked only when %s", variable, a.Condition())
		}
		value, err := interview.Ask(a)
		if err != nil {
			return err
		}
		e.Set(variable, value)
	}
	asked, err := interview.Variables(m, e, os.Stdout)
	if err != nil {
		return err
	}
	asked = append([]string{variable}, asked...)
	say("asked and written: " + strings.Join(asked, ", "))
	if err := e.Write(); err != nil {
		return err
	}
	return apply(m, e, root, asked)
}

func refuseBlocking(m *manifest.Manifest, remaining, turnedOff []string) error {
	for _, name := range turnedOff {
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
	return nil
}

func refuse(m *manifest.Manifest, on []string) error {
	if verdict := m.Validate(on); len(verdict.Refusals) > 0 {
		return fmt.Errorf("refused:\n  %s", strings.Join(verdict.Refusals, "\n  "))
	}
	return nil
}

type leftover struct {
	volumes    []string
	notes      map[string]string
	databases  []string
	clickhouse []string
}

func leftBehind(m *manifest.Manifest, on []string, c *manifest.Container) leftover {
	l := leftover{notes: map[string]string{}}
	for _, v := range c.Volumes {
		name := render.VolumeName(v)
		l.volumes = append(l.volumes, name)
		switch v {
		case manifest.PostgresData:
			l.notes[name] = "every database on Postgres lives in it"
		case manifest.ClickHouseData:
			l.notes[name] = "every database on ClickHouse lives in it"
		}
	}
	if c.Postgres != nil && !m.SharesDatabase(c, on) {
		l.databases = append(l.databases, c.Postgres.Database)
	}
	if c.ClickHouse != nil && !m.SharesClickHouse(c, on) {
		l.clickhouse = append(l.clickhouse, c.ClickHouse.Database)
	}
	return l
}

func (l leftover) empty() bool {
	return len(l.volumes) == 0 && len(l.databases) == 0 && len(l.clickhouse) == 0
}

func (l leftover) merge(o leftover) leftover {
	out := leftover{notes: map[string]string{}}
	out.volumes = once(l.volumes, o.volumes)
	out.databases = once(l.databases, o.databases)
	out.clickhouse = once(l.clickhouse, o.clickhouse)
	for k, v := range l.notes {
		out.notes[k] = v
	}
	for k, v := range o.notes {
		out.notes[k] = v
	}
	return out
}

func once(list, add []string) []string {
	out := append([]string{}, list...)
	for _, x := range add {
		if !contains(out, x) {
			out = append(out, x)
		}
	}
	return out
}

func (l leftover) String() string {
	var parts []string
	for _, v := range l.volumes {
		part := "volume " + v
		if n := l.notes[v]; n != "" {
			part += " (" + n + ")"
		}
		parts = append(parts, part)
	}
	for _, d := range l.databases {
		parts = append(parts, "database "+d+" and its user on Postgres")
	}
	for _, d := range l.clickhouse {
		parts = append(parts, "database "+d+" and its user on ClickHouse")
	}
	return strings.Join(parts, ", ")
}

func (l leftover) storeOff(on []string) string {
	switch {
	case len(l.databases) > 0 && !contains(on, manifest.Postgres):
		return manifest.Postgres
	case len(l.clickhouse) > 0 && !contains(on, manifest.ClickHouse):
		return manifest.ClickHouse
	}
	return ""
}

func offerReclaim(m *manifest.Manifest, on, names []string) error {
	total := leftover{notes: map[string]string{}}
	for _, name := range names {
		l := leftBehind(m, on, m.Container(name))
		if l.empty() {
			continue
		}
		say(fmt.Sprintf("%s is off and left behind: %s. Nothing is dropped unless you reclaim it.", name, l))
		total = total.merge(l)
	}
	if total.empty() {
		return nil
	}
	later := "./bootstrap reclaim " + strings.Join(names, " ")
	if store := total.storeOff(on); store != "" {
		say(fmt.Sprintf("%s is off now, so no database can be dropped from here: switch it on and run %s, or reclaim %s itself.", store, later, store))
		return nil
	}
	present, _, err := existing(total)
	if err != nil {
		return err
	}
	if present.empty() {
		return nil
	}
	yes, err := interview.Confirm("Reclaim now: drop "+present.String()+"?", "This cannot be undone. Answer no and it stays; "+later+" drops it later.")
	if err != nil || !yes {
		say("kept. " + later + " drops it when you want.")
		return nil
	}
	return drop(present)
}

func reclaim(m *manifest.Manifest, e *env.File, _ string, names []string) error {
	on := e.List(interview.On)
	total := leftover{notes: map[string]string{}}
	for _, name := range names {
		c := m.Container(name)
		if c == nil {
			return fmt.Errorf("%s is not a container the manifest knows", name)
		}
		if contains(on, name) {
			return fmt.Errorf("%s is on; switch it off first, then reclaim what it left", name)
		}
		total = total.merge(leftBehind(m, on, c))
	}
	if total.empty() {
		say(fmt.Sprintf("nothing to reclaim: %s declares no volume and no database", strings.Join(names, ", ")))
		return nil
	}
	if store := total.storeOff(on); store != "" {
		databases := total.databases
		if store == manifest.ClickHouse {
			databases = total.clickhouse
		}
		return fmt.Errorf("database %s lives on %s, which is off; switch it on first, or reclaim %s and its volume takes every database with it", strings.Join(databases, ", "), store, store)
	}
	present, gone, err := existing(total)
	if err != nil {
		return err
	}
	if !gone.empty() {
		say("already gone: " + gone.String())
	}
	if present.empty() {
		say("nothing left to reclaim")
		return nil
	}
	yes, err := interview.Confirm("Drop "+present.String()+"?", "This cannot be undone. Nothing is dropped unless you answer yes.")
	if err != nil {
		return err
	}
	if !yes {
		say("kept; nothing was dropped")
		return nil
	}
	return drop(present)
}

func existing(l leftover) (leftover, leftover, error) {
	present, gone := leftover{notes: l.notes}, leftover{notes: l.notes}
	for _, v := range l.volumes {
		if exec.Command("docker", "volume", "inspect", v).Run() == nil {
			present.volumes = append(present.volumes, v)
		} else {
			gone.volumes = append(gone.volumes, v)
		}
	}
	for _, d := range l.databases {
		p, err := provision.Exists(d)
		if err != nil {
			return present, gone, err
		}
		if p.Empty() {
			gone.databases = append(gone.databases, d)
		} else {
			present.databases = append(present.databases, d)
		}
	}
	for _, d := range l.clickhouse {
		p, err := provision.ClickHouseExists(d)
		if err != nil {
			return present, gone, err
		}
		if p.Empty() {
			gone.clickhouse = append(gone.clickhouse, d)
		} else {
			present.clickhouse = append(present.clickhouse, d)
		}
	}
	return present, gone, nil
}

func drop(l leftover) error {
	for _, d := range l.databases {
		did, err := provision.Drop(d)
		say(did...)
		if err != nil {
			return err
		}
	}
	for _, d := range l.clickhouse {
		did, err := provision.ClickHouseDrop(d)
		say(did...)
		if err != nil {
			return err
		}
	}
	for _, v := range l.volumes {
		out, err := exec.Command("docker", "volume", "rm", v).CombinedOutput()
		if err != nil {
			return fmt.Errorf("docker volume rm %s: %s", v, strings.TrimSpace(string(out)))
		}
		say("removed volume " + v)
	}
	return nil
}

func renderFile(m *manifest.Manifest, e *env.File, root string) error {
	on := e.List(interview.On)
	verdict := m.Validate(on)
	if len(verdict.Refusals) > 0 {
		return fmt.Errorf("refused:\n  %s", strings.Join(verdict.Refusals, "\n  "))
	}
	out, err := render.Render(render.Input{Manifest: m, On: on, Visibility: e.Get(interview.Visibility), Env: e, Root: root})
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

func apply(m *manifest.Manifest, e *env.File, root string, asked []string) error {
	on := e.List(interview.On)
	if contains(on, fort.Container) {
		if err := seedFort(e, root); err != nil {
			return err
		}
	}
	if err := renderFile(m, e, root); err != nil {
		return err
	}
	if stores := intersect([]string{manifest.Postgres, manifest.ClickHouse}, on); len(stores) > 0 {
		if err := compose(root, append([]string{"up", "-d", "--wait", "--remove-orphans"}, stores...)...); err != nil {
			return err
		}
		if err := provisionAll(m, e); err != nil {
			return err
		}
	}
	if err := compose(root, "up", "-d", "--remove-orphans", "--build"); err != nil {
		return err
	}
	if err := compose(root, "ps", "--format", "table {{.Name}}\t{{.Status}}\t{{.Ports}}"); err != nil {
		return err
	}
	closing(m, e, on, asked)
	if contains(on, fort.Container) {
		return backUp(e, root)
	}
	return nil
}

func provisionAll(m *manifest.Manifest, e *env.File) error {
	on := e.List(interview.On)
	if contains(on, manifest.Postgres) {
		report, err := provision.Door(e)
		say(report...)
		if err != nil {
			return err
		}
		report, err = provision.Databases(m, on, e)
		say(report...)
		if err != nil {
			return err
		}
	}
	if contains(on, manifest.ClickHouse) {
		report, err := provision.ClickHouse(m, on, e)
		say(report...)
		if err != nil {
			return err
		}
	}
	if !contains(on, manifest.Postgres) && !contains(on, manifest.ClickHouse) {
		say(fmt.Sprintf("nothing to provision: %s and %s are off", manifest.Postgres, manifest.ClickHouse))
	}
	return nil
}

func closing(m *manifest.Manifest, e *env.File, on []string, asked []string) {
	var keep []string
	said := map[string]bool{}
	for _, c := range m.All() {
		if !contains(on, c.Name) {
			continue
		}
		for _, prefix := range c.Externals() {
			x := c.External[prefix]
			if x.Kind == manifest.Bucket && contains(asked, prefix+"_"+manifest.Name) && !said[prefix] {
				say(interview.Retention(prefix, *x))
				said[prefix] = true
			}
		}
		for _, a := range c.Asks {
			if a.Keep {
				keep = append(keep, a.Var)
			}
		}
	}
	say("")
	say(contract.Text(m, e, on))
	if len(keep) > 0 {
		say("")
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
