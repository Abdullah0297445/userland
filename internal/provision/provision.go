package provision

import (
	"fmt"
	"os/exec"
	"strings"

	"github.com/Abdullah0297445/userland/internal/env"
	"github.com/Abdullah0297445/userland/internal/manifest"
)

const server = "postgres-18"

func Tenants(m *manifest.Manifest, on []string, e *env.File) ([]string, error) {
	set := map[string]bool{}
	for _, name := range on {
		set[name] = true
	}
	seen := map[string]bool{}
	var report []string
	for _, c := range m.All() {
		if !set[c.Name] || c.Tenant == nil || seen[c.Tenant.Name] {
			continue
		}
		seen[c.Tenant.Name] = true
		password := e.Get(c.Tenant.Password)
		if password == "" {
			return report, fmt.Errorf("tenant %s: .env lacks %s", c.Tenant.Name, c.Tenant.Password)
		}
		steps, err := converge(c.Tenant.Name, password)
		report = append(report, steps...)
		if err != nil {
			return report, err
		}
	}
	return report, nil
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
		did = append(did, "created role "+name)
	}
	db, err := query("postgres", "SELECT 1 FROM pg_database WHERE datname = "+literal(name))
	if err != nil {
		return did, err
	}
	if db != "1" {
		statements := []struct{ db, sql string }{
			{"postgres", fmt.Sprintf("CREATE DATABASE %s OWNER %s", ident(name), ident(name))},
			{"postgres", fmt.Sprintf("REVOKE CONNECT ON DATABASE %s FROM PUBLIC; GRANT CONNECT ON DATABASE %s TO %s", ident(name), ident(name), ident(name))},
			{name, "REVOKE CREATE ON SCHEMA public FROM PUBLIC; CREATE EXTENSION IF NOT EXISTS vector"},
		}
		for _, s := range statements {
			if err := run(s.db, s.sql); err != nil {
				return did, err
			}
		}
		did = append(did, "created database "+name)
	}
	if !canLogin(name, password) {
		if err := run("postgres", fmt.Sprintf("ALTER ROLE %s PASSWORD %s", ident(name), literal(password))); err != nil {
			return did, err
		}
		did = append(did, "set the password of "+name+" from .env")
	}
	if len(did) == 0 {
		did = append(did, "tenant "+name+" unchanged")
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

func canLogin(role, password string) bool {
	script := `IFS= read -r PGPASSWORD && export PGPASSWORD && exec psql -h 127.0.0.1 -U "$1" -d "$1" -tAXc "SELECT 1"`
	cmd := exec.Command("docker", "exec", "-i", server, "sh", "-c", script, "sh", role)
	cmd.Stdin = strings.NewReader(password + "\n")
	return cmd.Run() == nil
}

func ident(name string) string {
	return `"` + name + `"`
}

func literal(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "''") + "'"
}
