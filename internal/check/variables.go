package check

import (
	"fmt"
	"sort"
	"strings"

	"github.com/Abdullah0297445/userland/internal/manifest"
	"github.com/Abdullah0297445/userland/internal/render"
)

type row struct {
	variable, kind, when, keep, meaning string
}

func Variables(m *manifest.Manifest, bodies []body) string {
	var b strings.Builder
	b.WriteString("# Variables\n\n")
	b.WriteString("Everything `.env` may hold, by container. Generated from `manifest.json` and the templates by `./bootstrap check --write`; `check` fails when this file is stale, so it always matches the checked-out commit.\n\n")
	b.WriteString("An asked variable is collected by the interview when its container is switched on and the answer is not already in `.env`. An optional variable is neither asked nor written: add the line to `.env` by hand, and the next apply picks it up. `.env` is the only file you own.\n\n")
	b.WriteString("## userland\n\n")
	writeTable(&b, []row{
		{"VISIBILITY", "written by the CLI", "always", "", "`local` or `public`, chosen once for the whole of userland."},
		{"USERLAND_ON", "written by the CLI", "always", "", "The selection: every container that is switched on, comma-separated."},
		{"USERLAND_OFF", "written by the CLI", "always", "", "Every container that was asked about and is off, so a re-run does not ask again."},
	})
	asked := map[string]bool{}
	for _, c := range m.All() {
		for _, a := range c.AllAsks() {
			asked[a.Var] = true
		}
	}
	defaults := map[string]map[string]string{}
	for _, body := range bodies {
		for _, match := range reference.FindAllStringSubmatch(body.text, -1) {
			if match[2] == "" || asked[match[1]] {
				continue
			}
			if defaults[body.container] == nil {
				defaults[body.container] = map[string]string{}
			}
			defaults[body.container][match[1]] = match[3]
		}
	}
	product := ""
	for _, c := range m.All() {
		if c.Product != product {
			product = c.Product
			fmt.Fprintf(&b, "## %s\n\n", product)
		}
		fmt.Fprintf(&b, "### %s\n\n", c.Name)
		var rows []row
		for _, a := range c.Asks {
			kind := a.Type
			if a.Type == "choice" {
				kind = "choice: " + strings.Join(a.Options, ", ")
			}
			when := a.When
			if when == "" {
				when = "always"
			}
			keep := ""
			if a.Keep {
				keep = "yes"
			}
			rows = append(rows, row{a.Var, kind, when, keep, a.Prompt})
		}
		if c.Postgres != nil {
			rows = append(rows, row{c.Postgres.Password, "generated", "always", "", fmt.Sprintf("Password of the `%s` user on Postgres, which owns the `%s` database. Provisioning converges it to whatever this holds.", c.Postgres.Database, c.Postgres.Database)})
		}
		if c.ClickHouse != nil {
			rows = append(rows, row{c.ClickHouse.Password, "generated", "always", "", fmt.Sprintf("Password of the `%s` user on ClickHouse, which reaches the `%s` database and nothing else. Provisioning converges it to whatever this holds.", c.ClickHouse.Database, c.ClickHouse.Database)})
		}
		for _, prefix := range c.Externals() {
			x := c.External[prefix]
			for _, a := range x.Asks(prefix) {
				rows = append(rows, row{a.Var, x.Describe(), "always", "", externalMeaning(prefix, *x, a)})
			}
		}
		var optional []row
		if c.HTTP != nil {
			optional = append(optional, row{render.PortVar(c.Name), "optional", "", "", fmt.Sprintf("Loopback port to publish on while traefik is on; nothing is published unless it is set. With traefik off the container publishes on `127.0.0.1:%d` regardless.", c.HTTP.Host)})
		}
		optional = append(optional, row{render.MemLimitVar(c.Name), "optional", "", "", "Memory limit in compose's units, such as `2g`. Unbounded unless set."})
		var names []string
		for name := range defaults[c.Name] {
			names = append(names, name)
		}
		sort.Strings(names)
		for _, name := range names {
			meaning := "Read by the template; empty unless set."
			switch {
			case name == c.Files:
				meaning = fmt.Sprintf("Absolute paths this container keeps, separated by colons, each one's directory mounted read-only under `%s`. Written by the CLI: `./bootstrap %s add PATH` and `remove PATH` change it.", render.FilesRoot, c.Product)
			case defaults[c.Name][name] != "":
				meaning = fmt.Sprintf("Read by the template; defaults to `%s`.", defaults[c.Name][name])
			}
			optional = append(optional, row{name, "optional", "", "", meaning})
		}
		writeTable(&b, append(rows, optional...))
	}
	return b.String()
}

func externalMeaning(prefix string, x manifest.External, a manifest.Ask) string {
	switch strings.TrimPrefix(a.Var, prefix+"_") {
	case manifest.Name:
		return fmt.Sprintf("Name of the bucket. Enter to generate `userland-%s-<8 hex>`, or type one you made.", manifest.Dependency(prefix))
	case manifest.Region:
		if x.Kind == manifest.Bucket {
			return "Region of the bucket, as the provider names it; `auto` on Cloudflare R2."
		}
		return "Region of the parameter."
	case manifest.Endpoint:
		return "Scheme and host the bucket is reached at, with no path; a trailing `/` is dropped as you enter it. The offer writes `https://s3.<region>.amazonaws.com`."
	case manifest.AccessKeyID:
		if x.Kind == manifest.Bucket {
			rights := "list the bucket and get and put objects, and never delete"
			switch {
			case x.DeletesAnywhere():
				rights = "list the bucket and get, put and delete objects"
			case x.CanDelete():
				rights = fmt.Sprintf("list the bucket and get and put objects, and delete under `%s/` and nowhere else", x.DeleteUnder())
			}
			return fmt.Sprintf("Access key that reaches this bucket and nothing else. It may %s.", rights)
		}
		return "Access key that may read this one parameter and nothing else."
	case manifest.SecretAccessKey:
		return "Its secret."
	case manifest.Provider:
		return "Secret store the master key lives in; `ssm` is AWS Parameter Store, the one there is."
	case manifest.Parameter:
		return fmt.Sprintf("Name of the SecureString parameter that holds the master key. Enter to generate `/userland/%s-<8 hex>`, or type one you made.", manifest.Dependency(prefix))
	}
	return a.Prompt
}

func writeTable(b *strings.Builder, rows []row) {
	b.WriteString("| Variable | Kind | When | Keep | Meaning |\n|---|---|---|---|---|\n")
	for _, r := range rows {
		fmt.Fprintf(b, "| `%s` | %s | %s | %s | %s |\n", r.variable, r.kind, r.when, r.keep, r.meaning)
	}
	b.WriteString("\n")
}
