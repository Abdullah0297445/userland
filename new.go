package main

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/Abdullah0297445/userland/internal/check"
	"github.com/Abdullah0297445/userland/internal/scaffold"
)

func newCommand(root string) *cobra.Command {
	return &cobra.Command{
		Use:     "new PRODUCT CONTAINER...",
		Short:   "Append a product to manifest.json and write its template, with placeholders to fill in.",
		GroupID: contribute,
		Args:    cobra.MinimumNArgs(2),
		RunE: func(_ *cobra.Command, args []string) error {
			files, err := scaffold.Product(root, args[0], args[1:])
			if err != nil {
				return err
			}
			say("wrote " + strings.Join(files, " and "))
			if err := runCheck(root, true); err != nil {
				return err
			}
			say("")
			say(fmt.Sprintf("Now fill in %s's block in manifest.json and each define in compose/%s.yml, replacing %q.", args[0], args[0], scaffold.Placeholder))
			say("The README's Adding a container says what every field and key means; ./bootstrap check names what is missing.")
			return nil
		},
	}
}

func checkCommand(root string) *cobra.Command {
	var write bool
	cmd := &cobra.Command{
		Use:     "check",
		Short:   "Assert the manifest and templates hold. --write regenerates VARIABLES.md.",
		GroupID: contribute,
		Args:    cobra.NoArgs,
		RunE: func(*cobra.Command, []string) error {
			return runCheck(root, write)
		},
	}
	cmd.Flags().BoolVar(&write, "write", false, "regenerate VARIABLES.md instead of comparing it")
	return cmd
}

func runCheck(root string, write bool) error {
	report, err := check.Run(root, write)
	for _, p := range report.Passed {
		say("ok: " + p)
	}
	for _, f := range report.Failures {
		say("FAIL: " + f)
	}
	return err
}
