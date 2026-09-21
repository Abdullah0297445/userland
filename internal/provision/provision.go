package provision

import (
	"crypto/hmac"
	"crypto/pbkdf2"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"os/exec"
	"sort"
	"strconv"
	"strings"

	"github.com/Abdullah0297445/userland/internal/env"
	"github.com/Abdullah0297445/userland/internal/manifest"
)

const (
	server        = manifest.Postgres
	AuthUser      = "pgbouncer_auth"
	AuthPassword  = "PGBOUNCER_AUTH_PASSWORD"
	authFunction  = "pgbouncer_get_auth"
	APISchema     = "api"
	watchFunction = "pgrst_watch"
)

func Door(e *env.File) ([]string, error) {
	var did []string
	password := e.Get(AuthPassword)
	if password == "" {
		return did, fmt.Errorf("door auth: .env lacks %s", AuthPassword)
	}
	role, err := query("postgres", "SELECT 1 FROM pg_roles WHERE rolname = "+literal(AuthUser))
	if err != nil {
		return did, err
	}
	if role != "1" {
		sql := fmt.Sprintf("CREATE ROLE %s LOGIN NOSUPERUSER NOCREATEDB NOCREATEROLE NOINHERIT NOREPLICATION PASSWORD %s", ident(AuthUser), literal(password))
		if err := run("postgres", sql); err != nil {
			return did, err
		}
		did = append(did, "created user "+AuthUser)
	}
	function, err := query("postgres", "SELECT 1 FROM pg_proc WHERE proname = "+literal(authFunction)+" AND pronamespace = 'public'::regnamespace")
	if err != nil {
		return did, err
	}
	definition := strings.Join([]string{
		fmt.Sprintf("CREATE OR REPLACE FUNCTION public.%s(p_usename TEXT) RETURNS TABLE(usename TEXT, passwd TEXT) LANGUAGE sql SECURITY DEFINER SET search_path = pg_catalog AS $$ SELECT usename::TEXT, passwd::TEXT FROM pg_catalog.pg_shadow WHERE usename = p_usename; $$", authFunction),
		fmt.Sprintf("REVOKE EXECUTE ON FUNCTION public.%s(TEXT) FROM PUBLIC", authFunction),
		fmt.Sprintf("GRANT EXECUTE ON FUNCTION public.%s(TEXT) TO %s", authFunction, ident(AuthUser)),
	}, ";\n")
	if err := run("postgres", definition); err != nil {
		return did, err
	}
	if function != "1" {
		did = append(did, "created function "+authFunction)
	}
	matches, err := verifierMatches(AuthUser, password)
	if err != nil {
		return did, err
	}
	if !matches {
		if err := run("postgres", fmt.Sprintf("ALTER ROLE %s PASSWORD %s", ident(AuthUser), literal(password))); err != nil {
			return did, err
		}
		did = append(did, "set the password of "+AuthUser+" from .env")
	}
	if len(did) == 0 {
		did = append(did, "door auth unchanged")
	}
	return did, nil
}

func Databases(m *manifest.Manifest, on []string, e *env.File) ([]string, error) {
	set := map[string]bool{}
	for _, name := range on {
		set[name] = true
	}
	seen := map[string]bool{}
	var report []string
	for _, c := range m.All() {
		if !set[c.Name] || c.Postgres == nil || seen[c.Postgres.Database] {
			continue
		}
		seen[c.Postgres.Database] = true
		password := e.Get(c.Postgres.Password)
		if password == "" {
			return report, fmt.Errorf("database %s: .env lacks %s", c.Postgres.Database, c.Postgres.Password)
		}
		steps, err := converge(c.Postgres.Database, password)
		report = append(report, steps...)
		if err != nil {
			return report, err
		}
		steps, err = settings(c.Postgres.Database, c.Postgres.Settings)
		report = append(report, steps...)
		if err != nil {
			return report, err
		}
	}
	return report, nil
}

func settings(name string, want map[string]string) ([]string, error) {
	var did []string
	if len(want) == 0 {
		return did, nil
	}
	have, err := roleSettings(name)
	if err != nil {
		return did, err
	}
	names := make([]string, 0, len(want))
	for setting := range want {
		names = append(names, setting)
	}
	sort.Strings(names)
	for _, setting := range names {
		if have[setting] == want[setting] {
			continue
		}
		if err := run("postgres", fmt.Sprintf("ALTER ROLE %s SET %s = %s", ident(name), setting, literal(want[setting]))); err != nil {
			return did, err
		}
		did = append(did, fmt.Sprintf("set %s = %s on the user %s", setting, want[setting], name))
	}
	return did, nil
}

func roleSettings(name string) (map[string]string, error) {
	out, err := query("postgres", "SELECT unnest(setconfig) FROM pg_db_role_setting WHERE setdatabase = 0 AND setrole = (SELECT oid FROM pg_roles WHERE rolname = "+literal(name)+")")
	if err != nil {
		return nil, err
	}
	return parseSettings(out), nil
}

func parseSettings(lines string) map[string]string {
	have := map[string]string{}
	for _, line := range strings.Split(lines, "\n") {
		if setting, value, ok := strings.Cut(line, "="); ok {
			have[setting] = value
		}
	}
	return have
}

func converge(name, password string) ([]string, error) {
	var did []string
	role, err := query("postgres", "SELECT 1 FROM pg_roles WHERE rolname = "+literal(name))
	if err != nil {
		return did, err
	}
	if role != "1" {
		if err := run("postgres", fmt.Sprintf("CREATE ROLE %s LOGIN PASSWORD %s", ident(name), literal(password))); err != nil {
			return did, err
		}
		did = append(did, "created user "+name)
	}
	db, err := query("postgres", "SELECT 1 FROM pg_database WHERE datname = "+literal(name))
	if err != nil {
		return did, err
	}
	if db != "1" {
		if err := createDatabase(name); err != nil {
			return did, err
		}
		did = append(did, "created database "+name)
	}
	matches, err := verifierMatches(name, password)
	if err != nil {
		return did, err
	}
	if !matches {
		if err := run("postgres", fmt.Sprintf("ALTER ROLE %s PASSWORD %s", ident(name), literal(password))); err != nil {
			return did, err
		}
		did = append(did, "set the password of "+name+" from .env")
	}
	if len(did) == 0 {
		did = append(did, "database "+name+" unchanged")
	}
	return did, nil
}

func createDatabase(name string) error {
	statements := []struct{ db, sql string }{
		{"postgres", fmt.Sprintf("CREATE DATABASE %s OWNER %s", ident(name), ident(name))},
		{"postgres", fmt.Sprintf("REVOKE CONNECT ON DATABASE %s FROM PUBLIC; GRANT CONNECT ON DATABASE %s TO %s", ident(name), ident(name), ident(name))},
		{name, "REVOKE CREATE ON SCHEMA public FROM PUBLIC; CREATE EXTENSION IF NOT EXISTS vector"},
	}
	for _, s := range statements {
		if err := run(s.db, s.sql); err != nil {
			return err
		}
	}
	return nil
}

type Consumer struct {
	Name          string
	Password      string
	Authenticator string
	Anon          string
	APIPassword   string
}

func Authenticator(name string) string { return name + "_authenticator" }

func Anon(name string) string { return name + "_anon" }

func AddConsumer(name string, api bool) (*Consumer, error) {
	p, err := Exists(name)
	if err != nil {
		return nil, err
	}
	if p.Database {
		return nil, fmt.Errorf("the database %s already exists; nothing was changed", name)
	}
	if len(p.Users) > 0 {
		return nil, fmt.Errorf("the user %s exists without its database; nothing was changed", strings.Join(p.Users, ", "))
	}
	c := &Consumer{Name: name, Password: rand.Text()}
	if err := run("postgres", fmt.Sprintf("CREATE ROLE %s LOGIN PASSWORD %s", ident(name), literal(c.Password))); err != nil {
		return nil, err
	}
	if err := createDatabase(name); err != nil {
		return nil, err
	}
	if !api {
		return c, nil
	}
	c.Authenticator, c.Anon, c.APIPassword = Authenticator(name), Anon(name), rand.Text()
	recipe := strings.Join([]string{
		"BEGIN",
		fmt.Sprintf("CREATE SCHEMA IF NOT EXISTS %s AUTHORIZATION %s", APISchema, ident(name)),
		fmt.Sprintf("CREATE ROLE %s LOGIN NOINHERIT NOCREATEDB NOCREATEROLE NOSUPERUSER PASSWORD %s", ident(c.Authenticator), literal(c.APIPassword)),
		fmt.Sprintf("CREATE ROLE %s NOLOGIN", ident(c.Anon)),
		fmt.Sprintf("GRANT %s TO %s", ident(c.Anon), ident(c.Authenticator)),
		fmt.Sprintf("GRANT CONNECT ON DATABASE %s TO %s", ident(name), ident(c.Authenticator)),
		fmt.Sprintf("GRANT USAGE ON SCHEMA %s TO %s", APISchema, ident(c.Anon)),
		fmt.Sprintf("CREATE OR REPLACE FUNCTION public.%s() RETURNS event_trigger LANGUAGE plpgsql AS $$ BEGIN NOTIFY pgrst, 'reload schema'; END; $$", watchFunction),
		fmt.Sprintf("CREATE EVENT TRIGGER %s ON ddl_command_end EXECUTE FUNCTION public.%s()", watchFunction, watchFunction),
		"COMMIT",
	}, ";\n")
	if err := run(name, recipe); err != nil {
		return nil, err
	}
	return c, nil
}

type Presence struct {
	Database bool
	Users    []string
}

func (p Presence) Empty() bool {
	return !p.Database && len(p.Users) == 0
}

func (p Presence) String() string {
	var parts []string
	if p.Database {
		parts = append(parts, "the database")
	}
	for _, u := range p.Users {
		parts = append(parts, "the user "+u)
	}
	return strings.Join(parts, ", ")
}

func Exists(name string) (Presence, error) {
	var p Presence
	db, err := query("postgres", "SELECT 1 FROM pg_database WHERE datname = "+literal(name))
	if err != nil {
		return p, err
	}
	p.Database = db == "1"
	for _, role := range []string{name, Authenticator(name), Anon(name)} {
		r, err := query("postgres", "SELECT 1 FROM pg_roles WHERE rolname = "+literal(role))
		if err != nil {
			return p, err
		}
		if r == "1" {
			p.Users = append(p.Users, role)
		}
	}
	return p, nil
}

func Drop(name string) ([]string, error) {
	var did []string
	p, err := Exists(name)
	if err != nil {
		return did, err
	}
	if p.Database {
		if err := run("postgres", fmt.Sprintf("DROP DATABASE %s WITH (FORCE)", ident(name))); err != nil {
			return did, err
		}
		did = append(did, "dropped database "+name)
	}
	for _, role := range p.Users {
		if err := run("postgres", "DROP ROLE "+ident(role)); err != nil {
			return did, err
		}
		did = append(did, "dropped user "+role)
	}
	return did, nil
}

func psql(db, sql string) (string, error) {
	cmd := exec.Command("docker", "exec", "-i", server, "psql", "-v", "ON_ERROR_STOP=1", "--no-psqlrc", "-tAX", "-U", "postgres", "-d", db)
	cmd.Stdin = strings.NewReader(sql)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("psql on %s: %s", db, strings.TrimSpace(string(out)))
	}
	return strings.TrimSpace(string(out)), nil
}

func query(db, sql string) (string, error) { return psql(db, sql) }

func run(db, sql string) error {
	_, err := psql(db, sql)
	return err
}

func verifierMatches(role, password string) (bool, error) {
	verifier, err := query("postgres", "SELECT rolpassword FROM pg_authid WHERE rolname = "+literal(role))
	if err != nil {
		return false, err
	}
	return scramMatches(verifier, password), nil
}

func scramMatches(verifier, password string) bool {
	const prefix = "SCRAM-SHA-256$"
	if !strings.HasPrefix(verifier, prefix) {
		return false
	}
	parts := strings.Split(strings.TrimPrefix(verifier, prefix), "$")
	if len(parts) != 2 {
		return false
	}
	iterationsSalt := strings.SplitN(parts[0], ":", 2)
	keys := strings.SplitN(parts[1], ":", 2)
	if len(iterationsSalt) != 2 || len(keys) != 2 {
		return false
	}
	iterations, err := strconv.Atoi(iterationsSalt[0])
	if err != nil {
		return false
	}
	salt, err := base64.StdEncoding.DecodeString(iterationsSalt[1])
	if err != nil {
		return false
	}
	storedKey, err := base64.StdEncoding.DecodeString(keys[0])
	if err != nil {
		return false
	}
	salted, err := pbkdf2.Key(sha256.New, password, salt, iterations, sha256.Size)
	if err != nil {
		return false
	}
	mac := hmac.New(sha256.New, salted)
	mac.Write([]byte("Client Key"))
	clientKey := sha256.Sum256(mac.Sum(nil))
	return hmac.Equal(clientKey[:], storedKey)
}

func ident(name string) string {
	return `"` + name + `"`
}

func literal(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "''") + "'"
}
