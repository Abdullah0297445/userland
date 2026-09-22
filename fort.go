package main

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/Abdullah0297445/userland/internal/env"
	"github.com/Abdullah0297445/userland/internal/fort"
	"github.com/Abdullah0297445/userland/internal/interview"
	"github.com/Abdullah0297445/userland/internal/manifest"
)

func fortCommand(root string) *cobra.Command {
	keep := &cobra.Command{
		Use:     fort.Product,
		Short:   "fort: the files it keeps off this host, and the way back from the bucket alone.",
		GroupID: products,
	}
	add := &cobra.Command{
		Use:   "add PATH...",
		Short: "Keep more files: check they are here, apply, and back up now.",
		Args:  cobra.MinimumNArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			m, e, err := load(root)
			if err != nil {
				return err
			}
			return fortAdd(m, e, root, args)
		},
	}
	remove := &cobra.Command{
		Use:   "remove PATH...",
		Short: "Stop keeping files. What is already in the bucket stays there.",
		Args:  cobra.MinimumNArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			m, e, err := load(root)
			if err != nil {
				return err
			}
			return fortRemove(m, e, root, args)
		},
	}
	backup := &cobra.Command{
		Use:   "backup",
		Short: "Back up every kept file now, and say what the run added.",
		Args:  cobra.NoArgs,
		RunE: func(_ *cobra.Command, _ []string) error {
			m, e, err := load(root)
			if err != nil {
				return err
			}
			return fortBackup(m, e, root)
		},
	}
	var target string
	restore := &cobra.Command{
		Use:   "restore",
		Short: "Put every kept file back, from the bucket and the master key alone. Needs no .env.",
		Args:  cobra.NoArgs,
		RunE: func(_ *cobra.Command, _ []string) error {
			return fortRestore(root, target)
		},
	}
	restore.Flags().StringVar(&target, "target", "/", "extract under this directory instead of /")
	keep.AddCommand(add, remove, backup, restore)
	return keep
}

func fortAdd(m *manifest.Manifest, e *env.File, root string, paths []string) error {
	if err := fortOn(e); err != nil {
		return err
	}
	kept := fort.Kept(e)
	var fresh []string
	for _, path := range paths {
		clean, err := keepable(path)
		if err != nil {
			return err
		}
		if contains(kept, clean) {
			say(clean + " is kept already")
			continue
		}
		kept = append(kept, clean)
		fresh = append(fresh, clean)
	}
	if len(fresh) == 0 {
		return nil
	}
	fort.Keep(e, kept)
	if err := e.Write(); err != nil {
		return err
	}
	say("keeping: " + strings.Join(fresh, ", "))
	return apply(m, e, root, nil)
}

func fortRemove(m *manifest.Manifest, e *env.File, root string, paths []string) error {
	kept := fort.Kept(e)
	own := filepath.Join(root, ".env")
	var dropped []string
	for _, path := range paths {
		clean := filepath.Clean(path)
		if !contains(kept, clean) {
			return fmt.Errorf("%s is not kept; ./bootstrap fort backup prints what is", clean)
		}
		if clean == own && contains(e.List(interview.On), fort.Container) {
			return fmt.Errorf("%s holds every secret this host has, and fort is on; switch fort off if you mean to stop keeping it", own)
		}
		kept = without(kept, []string{clean})
		dropped = append(dropped, clean)
	}
	fort.Keep(e, kept)
	if err := e.Write(); err != nil {
		return err
	}
	say("no longer keeping: " + strings.Join(dropped, ", "))
	say("what is already in the bucket stays there; every snapshot taken so far still holds these files")
	return apply(m, e, root, nil)
}

func fortBackup(m *manifest.Manifest, e *env.File, root string) error {
	if err := fortOn(e); err != nil {
		return err
	}
	return backUp(e, root)
}

func fortOn(e *env.File) error {
	if !contains(e.List(interview.On), fort.Container) {
		return fmt.Errorf("%s is off; ./bootstrap on %s switches it on", fort.Container, fort.Product)
	}
	return nil
}

func keepable(path string) (string, error) {
	clean := filepath.Clean(path)
	if strings.Contains(clean, fort.Separator) {
		return "", fmt.Errorf("%s holds a %s, which is what separates one kept path from the next in .env", path, fort.Separator)
	}
	if err := interview.Shape(manifest.Paths)(clean); err != nil {
		return "", fmt.Errorf("%s cannot be kept: %w", path, err)
	}
	if !filepath.IsAbs(clean) {
		return "", fmt.Errorf("%s is not an absolute path; fort keeps a file by where it is, and puts it back there", path)
	}
	return clean, nil
}

func seedFort(e *env.File, root string) error {
	own := filepath.Join(root, ".env")
	kept := fort.Kept(e)
	if contains(kept, own) {
		return nil
	}
	fort.Keep(e, append([]string{own}, kept...))
	say(fmt.Sprintf("fort keeps %s, which holds every secret this host has. ./bootstrap fort add PATH keeps more.", own))
	return e.Write()
}

func backUp(e *env.File, root string) error {
	if err := fort.Build(root); err != nil {
		return err
	}
	state, err := fort.Probe(e)
	if err != nil {
		return fmt.Errorf("fort cannot read its bucket: %w", err)
	}
	switch state {
	case fort.Absent:
		out, err := fort.Init(e)
		say(out)
		if err != nil {
			return fmt.Errorf("fort could not start an archive in the bucket: %w", err)
		}
	case fort.Foreign:
		return fmt.Errorf("the bucket already holds an archive that this master key does not open; point fort at another bucket, or find the key that made this one. Nothing was written")
	}
	out, err := fort.Backup(e)
	say(out)
	if err != nil {
		return fmt.Errorf("fort's backup failed: %w", err)
	}
	return nil
}

func fortRestore(root, target string) error {
	m, err := manifest.Load(filepath.Join(root, "manifest.json"))
	if err != nil {
		return err
	}
	c := m.Container(fort.Container)
	if c == nil {
		return fmt.Errorf("manifest.json has no %s", fort.Container)
	}
	e, err := env.Read(filepath.Join(root, ".env"))
	if err != nil {
		e = env.New(filepath.Join(root, ".env"))
		say("no .env here, which is the case a restore is for. Enter where the bucket and the master key are.")
	}
	for _, a := range c.ExternalAsks() {
		if e.Get(a.Var) != "" {
			continue
		}
		value, err := interview.Ask(a)
		if err != nil {
			return err
		}
		e.Set(a.Var, value)
	}
	if err := fort.Build(root); err != nil {
		return err
	}
	state, err := fort.Probe(e)
	if err != nil {
		return fmt.Errorf("fort cannot read its bucket: %w", err)
	}
	switch state {
	case fort.Absent:
		return fmt.Errorf("the bucket holds no archive; there is nothing to restore")
	case fort.Foreign:
		return fmt.Errorf("the bucket holds an archive that this master key does not open; there is nothing this key can restore")
	}
	nodes, err := fort.Listing(e)
	if err != nil {
		return err
	}
	say("")
	say("The latest backup holds:")
	say("")
	for _, n := range nodes {
		say(fmt.Sprintf("  %s  %7d  %s", n.Modified.UTC().Format("2006-01-02 15:04:05"), n.Size, filepath.Join(target, n.Path)))
	}
	say("")
	yes, err := interview.Confirm(fmt.Sprintf("Write these %d files, each over whatever is there now?", len(nodes)),
		"The time shown is when the file was last changed, not when it was backed up. Nothing is written unless you answer yes.")
	if err != nil {
		return err
	}
	if !yes {
		say("nothing was written")
		return nil
	}
	written, err := fort.Extract(e, target)
	for _, path := range written {
		say("restored " + path)
	}
	if err != nil {
		return fmt.Errorf("restoring: %w. Run again as a user that may write these paths", err)
	}
	say("")
	say(fmt.Sprintf("%d files are back. ./bootstrap brings the stack up against them.", len(written)))
	return nil
}
