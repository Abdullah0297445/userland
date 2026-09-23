package main

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/Abdullah0297445/userland/internal/contract"
	"github.com/Abdullah0297445/userland/internal/env"
	"github.com/Abdullah0297445/userland/internal/interview"
	"github.com/Abdullah0297445/userland/internal/manifest"
	"github.com/Abdullah0297445/userland/internal/provision"
	"github.com/Abdullah0297445/userland/internal/render"
)

func postgresCommand(root string) *cobra.Command {
	postgres := &cobra.Command{
		Use:     "postgres",
		Short:   "Postgres: a consumer's database and user.",
		GroupID: products,
	}
	database := &cobra.Command{
		Use:   "database",
		Short: "A consumer's database, owned by a user of the same name.",
	}
	var session, api bool
	add := &cobra.Command{
		Use:   "add NAME",
		Short: "Make the database and its user for a consumer, and print the DSN once.",
		Args:  cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			m, e, err := load(root)
			if err != nil {
				return err
			}
			return addDatabase(m, e, args[0], session, api)
		},
	}
	add.Flags().BoolVar(&session, "session", false, "the printed DSN names the session door")
	add.Flags().BoolVar(&api, "api", false, "add the PostgREST recipe, and print its DSN too")
	remove := &cobra.Command{
		Use:   "remove NAME",
		Short: "Drop a consumer's database and its users, after asking.",
		Args:  cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			m, e, err := load(root)
			if err != nil {
				return err
			}
			return removeDatabase(m, e, args[0])
		},
	}
	database.AddCommand(add, remove)
	postgres.AddCommand(database)
	return postgres
}

func addDatabase(m *manifest.Manifest, e *env.File, name string, session, api bool) error {
	if !manifest.ValidIdentifier(name) {
		return fmt.Errorf("%q is not a database name: lower-case letters, digits and underscores, starting with a letter, at most 63 characters", name)
	}
	if api && len(provision.Authenticator(name)) > 63 {
		return fmt.Errorf("%s is too long for --api: the user %s would exceed 63 characters", name, provision.Authenticator(name))
	}
	if c := m.Database(name); c != nil {
		return fmt.Errorf("%s is %s's database; switch %s on and provisioning makes it", name, c.Name, c.Name)
	}
	on := e.List(interview.On)
	if !contains(on, manifest.Postgres) {
		return fmt.Errorf("%s is off; switch it on first", manifest.Postgres)
	}
	door, doorName := manifest.TransactionDoor, "transaction"
	if session {
		door, doorName = manifest.SessionDoor, "session"
	}
	c, err := provision.AddConsumer(name, api)
	if err != nil {
		return err
	}
	say(fmt.Sprintf("Database %s and user %s are made. The password is shown once, here, and nowhere else: userland keeps no copy you can read back.", name, name))
	say("")
	say(fmt.Sprintf("  For the application, through the %s door. Paste into the consumer's own gitignored .env:", doorName))
	say("")
	say("    DATABASE_URL=" + contract.DSN(name, c.Password, door, name))
	if session {
		say("")
		say("  This DSN names the session door. It holds one Postgres connection for as long as the consumer holds its own,")
		say("  so the consumer must release connections promptly: with Django, CONN_MAX_AGE = 0, which is its default.")
	}
	if api {
		say("")
		say("  For PostgREST, direct to Postgres and never through a door. Paste into the same .env:")
		say("")
		say("    PGRST_DB_URI=" + contract.DSN(c.Authenticator, c.APIPassword, manifest.Postgres, name))
		say("    PGRST_DB_SCHEMAS=" + provision.APISchema)
		say("    PGRST_DB_ANON_ROLE=" + c.Anon)
		say("")
		say(fmt.Sprintf("  Installed in %s: the schema %s, owned by %s; the user %s, which holds no table rights and inherits none;", name, provision.APISchema, name, c.Authenticator))
		say(fmt.Sprintf("  the user %s, which cannot log in; and an event trigger that tells PostgREST to reload its schema cache after a migration.", c.Anon))
		say("")
		say(fmt.Sprintf("  The two DSNs are not interchangeable. DATABASE_URL names a door; PGRST_DB_URI names %s. Through the transaction", manifest.Postgres))
		say("  door PostgREST's LISTEN reload breaks silently while every health check passes, so before you paste PGRST_DB_URI,")
		say(fmt.Sprintf("  check that it holds _authenticator and @%s.", manifest.Postgres))
	}
	say("")
	say(fmt.Sprintf("  To reach it, the consumer's compose file declares %s external and joins it.", render.Network(m.Container(door).Product)))
	if !contains(on, door) {
		say("")
		say(fmt.Sprintf("warning: %s is off, and the DSN names it; switch it on before the consumer starts", door))
	}
	return nil
}

func removeDatabase(m *manifest.Manifest, e *env.File, name string) error {
	if !manifest.ValidIdentifier(name) {
		return fmt.Errorf("%q is not a database name", name)
	}
	if c := m.Database(name); c != nil {
		return fmt.Errorf("%s is %s's database; ./bootstrap reclaim %s drops it once %s is off", name, c.Name, c.Name, c.Name)
	}
	if !contains(e.List(interview.On), manifest.Postgres) {
		return fmt.Errorf("%s is off; switch it on first", manifest.Postgres)
	}
	p, err := provision.Exists(name)
	if err != nil {
		return err
	}
	if p.Empty() {
		say(fmt.Sprintf("nothing named %s on Postgres: no database and no user", name))
		return nil
	}
	yes, err := interview.Confirm(fmt.Sprintf("Drop %s: %s?", name, p), "Every row in the database is lost. This cannot be undone.")
	if err != nil {
		return err
	}
	if !yes {
		say("kept; nothing was dropped")
		return nil
	}
	did, err := provision.Drop(name)
	say(did...)
	return err
}
