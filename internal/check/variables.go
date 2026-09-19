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
		for _, a := range c.Asks {
			asked[a.Var] = true
		}
		if c.Tenant != nil {
			asked[c.Tenant.Password] = true
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
		if c.Tenant != nil {
			rows = append(rows, row{c.Tenant.Password, "generated", "always", "", fmt.Sprintf("Password of the `%s` tenant role. Provisioning converges the role to whatever this holds.", c.Tenant.Name)})
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
			if d := defaults[c.Name][name]; d != "" {
				meaning = fmt.Sprintf("Read by the template; defaults to `%s`.", d)
			}
			optional = append(optional, row{name, "optional", "", "", meaning})
		}
		writeTable(&b, append(rows, optional...))
	}
	return b.String()
}

func writeTable(b *strings.Builder, rows []row) {
	b.WriteString("| Variable | Kind | When | Keep | Meaning |\n|---|---|---|---|---|\n")
	for _, r := range rows {
		fmt.Fprintf(b, "| `%s` | %s | %s | %s | %s |\n", r.variable, r.kind, r.when, r.keep, r.meaning)
	}
	b.WriteString("\n")
}
