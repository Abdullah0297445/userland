package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/spf13/cobra"

	"github.com/Abdullah0297445/userland/internal/env"
	"github.com/Abdullah0297445/userland/internal/interview"
	"github.com/Abdullah0297445/userland/internal/manifest"
)

const (
	operate    = "operate"
	products   = "products"
	contribute = "contribute"
)

func main() {
	root, err := findRoot()
	must(err)
	must(command(root).Execute())
}

func command(root string) *cobra.Command {
	cobra.EnableCommandSorting = false
	cmd := &cobra.Command{
		Use:   "userland",
		Short: "One host, one compose project, and containers you switch on and off.",
		Long: strings.Join([]string{
			"userland: one host, one compose project, and containers you switch on and off.",
			"",
			"With no verb it runs the interview: it asks what is new, writes .env and applies.",
			"Every verb that changes .env ends with an apply: render compose.yml, bring up, provision, print.",
		}, "\n"),
		Args:          cobra.NoArgs,
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(*cobra.Command, []string) error {
			return interviewThenApply(root)
		},
	}
	cmd.CompletionOptions.DisableDefaultCmd = true
	cmd.SetHelpCommand(&cobra.Command{Use: "no-help", Hidden: true, GroupID: contribute})
	cmd.AddGroup(
		&cobra.Group{ID: operate, Title: "Verbs:"},
		&cobra.Group{ID: products, Title: "Products, each under its own name:"},
		&cobra.Group{ID: contribute, Title: "For contributors:"},
	)
	cmd.AddCommand(
		onCommand(root), offCommand(root), setCommand(root), reclaimCommand(root), contractCommand(root),
		applyCommand(root), renderCommand(root), provisionCommand(root),
		postgresCommand(root), fortCommand(root),
		newCommand(root), checkCommand(root),
	)
	return cmd
}

type action func(m *manifest.Manifest, e *env.File, root string, args []string) error

func verb(root, use, short, group string, args cobra.PositionalArgs, run action) *cobra.Command {
	return &cobra.Command{
		Use:     use,
		Short:   short,
		GroupID: group,
		Args:    args,
		RunE: func(_ *cobra.Command, positional []string) error {
			m, e, err := load(root)
			if err != nil {
				return err
			}
			return run(m, e, root, positional)
		},
	}
}

func load(root string) (*manifest.Manifest, *env.File, error) {
	m, err := manifest.Load(filepath.Join(root, "manifest.json"))
	if err != nil {
		return nil, nil, err
	}
	e, err := env.Read(filepath.Join(root, ".env"))
	if err != nil {
		return nil, nil, fmt.Errorf("no .env at %s; run ./bootstrap with no verb and the interview writes it", root)
	}
	if err := migrate(m, e); err != nil {
		return nil, nil, err
	}
	if err := e.Require(interview.Visibility); err != nil {
		return nil, nil, err
	}
	return m, e, nil
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
	return apply(m, e, root, result.Asked)
}

func migrate(m *manifest.Manifest, e *env.File) error {
	moved := interview.Migrate(m, e)
	if len(moved) == 0 {
		return nil
	}
	say(moved...)
	return e.Write()
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
