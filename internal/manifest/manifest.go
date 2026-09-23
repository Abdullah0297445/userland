package manifest

import (
	"encoding/json"
	"fmt"
	"maps"
	"os"
	"regexp"
	"sort"
	"strings"
)

type Manifest struct {
	Products map[string]*Product `json:"products"`
}

type Product struct {
	Containers map[string]*Container `json:"containers"`
}

type Container struct {
	Name       string               `json:"-"`
	Product    string               `json:"-"`
	Requires   []string             `json:"requires"`
	Optional   []string             `json:"optional"`
	HTTP       *HTTP                `json:"http"`
	Postgres   *Database            `json:"postgres"`
	ClickHouse *Database            `json:"clickhouse"`
	Ports      []int                `json:"ports"`
	Volumes    []string             `json:"volumes"`
	Files      string               `json:"files"`
	Asks       []Ask                `json:"asks"`
	External   map[string]*External `json:"external"`
	Renamed    map[string]string    `json:"renamed"`
	Removed    []string             `json:"removed"`
}

type HTTP struct {
	Container int    `json:"container"`
	Host      int    `json:"host"`
	Subdomain string `json:"subdomain"`
}

type Database struct {
	Database string            `json:"database"`
	Password string            `json:"password"`
	Settings map[string]string `json:"settings"`
}

type Ask struct {
	Var     string   `json:"var"`
	Type    string   `json:"type"`
	Prompt  string   `json:"prompt"`
	When    string   `json:"when"`
	Options []string `json:"options"`
	Keep    bool     `json:"keep"`
}

const (
	Text      = "text"
	Hostname  = "hostname"
	Email     = "email"
	URL       = "url"
	Port      = "port"
	Secret    = "secret"
	Generated = "generated"
	Hex       = "hex"
	Choice    = "choice"
	Paths     = "paths"

	DatabasePassword = "database-password"
)

var AskTypes = []string{Text, Hostname, Email, URL, Port, Secret, Generated, Hex, Choice, Paths}

var (
	identifierPattern = regexp.MustCompile(`^[a-z][a-z0-9_]{0,62}$`)
	variablePattern   = regexp.MustCompile(`^[A-Z][A-Z0-9_]*$`)
	namePattern       = regexp.MustCompile(`^[a-z][a-z0-9-]{0,62}$`)
	settingPattern    = regexp.MustCompile(`^[a-z_][a-z0-9_]*(\.[a-z_][a-z0-9_]*)?$`)
)

func ValidIdentifier(s string) bool { return identifierPattern.MatchString(s) }

func ValidVariable(s string) bool { return variablePattern.MatchString(s) }

func ValidName(s string) bool { return namePattern.MatchString(s) }

func ValidSetting(s string) bool { return settingPattern.MatchString(s) }

func (a Ask) Applies(visibility string, value func(string) string) bool {
	switch a.When {
	case "":
		return true
	case "public", "local":
		return a.When == visibility
	}
	name, want, _ := strings.Cut(a.When, "=")
	return value(name) == want
}

func (a Ask) Hidden() bool {
	return a.Type == Secret || a.Type == Generated || a.Type == Hex || a.Type == DatabasePassword
}

func (a Ask) Condition() string {
	switch a.When {
	case "":
		return "always"
	case "public", "local":
		return "visibility is " + a.When
	}
	name, want, _ := strings.Cut(a.When, "=")
	return name + " is " + want
}

func Load(path string) (*Manifest, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return Parse(raw)
}

func Parse(raw []byte) (*Manifest, error) {
	var m Manifest
	if err := json.Unmarshal(raw, &m); err != nil {
		return nil, fmt.Errorf("manifest.json: %w", err)
	}
	seen := map[string]string{}
	for product, p := range m.Products {
		for name, c := range p.Containers {
			if other, dup := seen[name]; dup {
				return nil, fmt.Errorf("manifest.json: container %q is in both %s and %s", name, other, product)
			}
			seen[name] = product
			c.Name = name
			c.Product = product
			if c.HTTP != nil && c.HTTP.Subdomain == "" {
				c.HTTP.Subdomain = name
			}
			if c.Postgres != nil {
				if !ValidIdentifier(c.Postgres.Database) {
					return nil, fmt.Errorf("manifest.json: database %q of %s is not a valid Postgres identifier", c.Postgres.Database, name)
				}
				if !ValidVariable(c.Postgres.Password) {
					return nil, fmt.Errorf("manifest.json: %s names its database password %q, which is not a variable name", name, c.Postgres.Password)
				}
				for setting, value := range c.Postgres.Settings {
					if !ValidSetting(setting) {
						return nil, fmt.Errorf("manifest.json: %s sets %q on its Postgres user, which is not a setting name", name, setting)
					}
					if value == "" {
						return nil, fmt.Errorf("manifest.json: %s sets %s on its Postgres user to nothing", name, setting)
					}
				}
			}
			if c.ClickHouse != nil {
				if !ValidIdentifier(c.ClickHouse.Database) {
					return nil, fmt.Errorf("manifest.json: database %q of %s is not a valid ClickHouse identifier", c.ClickHouse.Database, name)
				}
				if !ValidVariable(c.ClickHouse.Password) {
					return nil, fmt.Errorf("manifest.json: %s names its ClickHouse password %q, which is not a variable name", name, c.ClickHouse.Password)
				}
				if len(c.ClickHouse.Settings) > 0 {
					return nil, fmt.Errorf("manifest.json: %s sets settings on its ClickHouse user; settings are Postgres's", name)
				}
				if c.Postgres != nil && c.Postgres.Password == c.ClickHouse.Password {
					return nil, fmt.Errorf("manifest.json: %s names %s as both its Postgres and its ClickHouse password", name, c.Postgres.Password)
				}
			}
			if c.Files != "" && !ValidVariable(c.Files) {
				return nil, fmt.Errorf("manifest.json: %s keeps the paths named by %q, which is not a variable name", name, c.Files)
			}
			if err := c.checkAsks(); err != nil {
				return nil, err
			}
			for _, prefix := range c.Externals() {
				if err := c.External[prefix].check(name, prefix); err != nil {
					return nil, err
				}
			}
		}
	}
	volumes := map[string]string{}
	hostPorts := map[int]string{}
	databases := map[string]*Container{}
	externals := map[string]*Container{}
	explicit := map[string]string{}
	for _, c := range m.All() {
		for _, a := range c.Asks {
			explicit[a.Var] = c.Name
		}
	}
	for _, c := range m.All() {
		for _, prefix := range c.Externals() {
			x := c.External[prefix]
			if other, shared := externals[prefix]; shared && *other.External[prefix] != *x {
				return nil, fmt.Errorf("manifest.json: %s and %s share the external dependency %s but describe it differently", other.Name, c.Name, prefix)
			}
			externals[prefix] = c
			for _, v := range x.Variables(prefix) {
				if asker, taken := explicit[v]; taken {
					return nil, fmt.Errorf("manifest.json: %s asks %s, which %s's %s supplies", asker, v, c.Name, prefix)
				}
			}
		}
		if c.Postgres != nil {
			if other, shared := databases[c.Postgres.Database]; shared && !maps.Equal(other.Postgres.Settings, c.Postgres.Settings) {
				return nil, fmt.Errorf("manifest.json: %s and %s share the database %s but set different settings on its user", other.Name, c.Name, c.Postgres.Database)
			}
			databases[c.Postgres.Database] = c
		}
		for _, dep := range append(append([]string{}, c.Requires...), c.Optional...) {
			if m.Container(dep) == nil {
				return nil, fmt.Errorf("manifest.json: %s depends on %q, which no product has", c.Name, dep)
			}
			if dep == c.Name {
				return nil, fmt.Errorf("manifest.json: %s depends on itself", c.Name)
			}
		}
		for _, v := range c.Volumes {
			if other, dup := volumes[v]; dup {
				return nil, fmt.Errorf("manifest.json: volume %q is declared by both %s and %s", v, other, c.Name)
			}
			volumes[v] = c.Name
		}
		if c.HTTP != nil {
			if other, dup := hostPorts[c.HTTP.Host]; dup {
				return nil, fmt.Errorf("manifest.json: %s and %s both publish loopback port %d", other, c.Name, c.HTTP.Host)
			}
			hostPorts[c.HTTP.Host] = c.Name
		}
	}
	return &m, nil
}

func (c *Container) checkAsks() error {
	for _, a := range c.Asks {
		if !ValidVariable(a.Var) {
			return fmt.Errorf("manifest.json: %s asks %q, which is not a variable name", c.Name, a.Var)
		}
		if !contains(AskTypes, a.Type) {
			return fmt.Errorf("manifest.json: %s asks %s with type %q; the types are %s", c.Name, a.Var, a.Type, strings.Join(AskTypes, " "))
		}
		if a.Type == Choice && len(a.Options) == 0 {
			return fmt.Errorf("manifest.json: %s asks %s as a choice with no options", c.Name, a.Var)
		}
		if a.When != "" && a.When != "public" && a.When != "local" && !strings.Contains(a.When, "=") {
			return fmt.Errorf("manifest.json: %s asks %s when %q, which is not public, local or VAR=value", c.Name, a.Var, a.When)
		}
	}
	for old, now := range c.Renamed {
		if !ValidVariable(old) || !ValidVariable(now) {
			return fmt.Errorf("manifest.json: %s renames %q to %q; both must be variable names", c.Name, old, now)
		}
	}
	for _, name := range c.Removed {
		if !ValidVariable(name) {
			return fmt.Errorf("manifest.json: %s removes %q, which is not a variable name", c.Name, name)
		}
	}
	return nil
}

func (c *Container) PasswordAsks() []Ask {
	var asks []Ask
	if c.Postgres != nil {
		asks = append(asks, Ask{Var: c.Postgres.Password, Type: DatabasePassword, Prompt: fmt.Sprintf("Password of the %s user on Postgres", c.Postgres.Database)})
	}
	if c.ClickHouse != nil {
		asks = append(asks, Ask{Var: c.ClickHouse.Password, Type: DatabasePassword, Prompt: fmt.Sprintf("Password of the %s user on ClickHouse", c.ClickHouse.Database)})
	}
	return asks
}

func (c *Container) AllAsks() []Ask {
	return append(append(append([]Ask{}, c.Asks...), c.PasswordAsks()...), c.ExternalAsks()...)
}

func (m *Manifest) Container(name string) *Container {
	for _, p := range m.Products {
		if c, ok := p.Containers[name]; ok {
			return c
		}
	}
	return nil
}

func (m *Manifest) Database(database string) *Container {
	for _, c := range m.All() {
		if c.Postgres != nil && c.Postgres.Database == database {
			return c
		}
	}
	return nil
}

func (m *Manifest) SharesDatabase(c *Container, on []string) bool {
	return m.shares(c, on, func(x *Container) *Database { return x.Postgres })
}

func (m *Manifest) SharesClickHouse(c *Container, on []string) bool {
	return m.shares(c, on, func(x *Container) *Database { return x.ClickHouse })
}

func (m *Manifest) shares(c *Container, on []string, of func(*Container) *Database) bool {
	mine := of(c)
	if mine == nil {
		return false
	}
	for _, other := range m.All() {
		theirs := of(other)
		if other.Name != c.Name && theirs != nil && theirs.Database == mine.Database && contains(on, other.Name) {
			return true
		}
	}
	return false
}

func (m *Manifest) AskFor(name string) (*Container, Ask, bool) {
	for _, c := range m.All() {
		for _, a := range c.AllAsks() {
			if a.Var == name {
				return c, a, true
			}
		}
	}
	return nil, Ask{}, false
}

func (m *Manifest) All() []*Container {
	var all []*Container
	for _, p := range m.Products {
		for _, c := range p.Containers {
			all = append(all, c)
		}
	}
	sort.Slice(all, func(i, j int) bool {
		if all[i].Product != all[j].Product {
			return all[i].Product < all[j].Product
		}
		return all[i].Name < all[j].Name
	})
	return all
}

func (m *Manifest) Names() []string {
	var names []string
	for _, c := range m.All() {
		names = append(names, c.Name)
	}
	return names
}

func (m *Manifest) ProductContainers(product string) []*Container {
	var out []*Container
	for _, c := range m.All() {
		if c.Product == product {
			out = append(out, c)
		}
	}
	return out
}

func (m *Manifest) ProductOrder() []string {
	dependents := map[string]int{}
	edges := map[string]map[string]bool{}
	for name := range m.Products {
		dependents[name] = 0
		edges[name] = map[string]bool{}
	}
	for _, c := range m.All() {
		for _, dep := range append(append([]string{}, c.Requires...), c.Optional...) {
			target := m.Container(dep).Product
			if target == c.Product || edges[c.Product][target] {
				continue
			}
			edges[c.Product][target] = true
			dependents[target]++
		}
	}
	var order []string
	for len(dependents) > 0 {
		var ready []string
		for name, n := range dependents {
			if n == 0 {
				ready = append(ready, name)
			}
		}
		if len(ready) == 0 {
			for name := range dependents {
				ready = append(ready, name)
			}
			sort.Strings(ready)
			return append(order, ready...)
		}
		sort.Strings(ready)
		next := ready[0]
		order = append(order, next)
		delete(dependents, next)
		for target := range edges[next] {
			if _, waiting := dependents[target]; waiting {
				dependents[target]--
			}
		}
	}
	return order
}

func (m *Manifest) Required() map[string]bool {
	required := map[string]bool{}
	for _, c := range m.All() {
		for _, r := range c.Requires {
			required[r] = true
		}
	}
	return required
}

func (m *Manifest) Blocking(name string) []string {
	var dependents []string
	for _, c := range m.All() {
		if contains(c.Requires, name) {
			dependents = append(dependents, c.Name)
		}
	}
	return dependents
}

func (m *Manifest) OptionalFor(name string) []string {
	var dependents []string
	for _, c := range m.All() {
		if contains(c.Optional, name) {
			dependents = append(dependents, c.Name)
		}
	}
	return dependents
}

type Verdict struct {
	Refusals []string
	Warnings []string
}

func (m *Manifest) Validate(on []string) Verdict {
	var v Verdict
	set := map[string]bool{}
	for _, name := range on {
		if m.Container(name) == nil {
			v.Refusals = append(v.Refusals, fmt.Sprintf("%s is not a container the manifest knows", name))
			continue
		}
		set[name] = true
	}
	if len(set) == 0 {
		v.Refusals = append(v.Refusals, "nothing is switched on")
		return v
	}
	for _, c := range m.All() {
		if !set[c.Name] {
			continue
		}
		for _, r := range c.Requires {
			if !set[r] {
				v.Refusals = append(v.Refusals, fmt.Sprintf("%s is blocked by %s, which is off", c.Name, r))
			}
		}
		if c.Postgres != nil && !set[Postgres] {
			v.Refusals = append(v.Refusals, fmt.Sprintf("%s has a database on Postgres, so %s must be on", c.Name, Postgres))
		}
		if c.ClickHouse != nil && !set[ClickHouse] {
			v.Refusals = append(v.Refusals, fmt.Sprintf("%s has a database on ClickHouse, so %s must be on", c.Name, ClickHouse))
		}
		for _, o := range c.Optional {
			if !set[o] {
				w := fmt.Sprintf("%s runs without %s", c.Name, o)
				if o == Proxy && c.HTTP != nil {
					w += fmt.Sprintf(", so it publishes on 127.0.0.1:%d and is reachable from this machine only", c.HTTP.Host)
				}
				v.Warnings = append(v.Warnings, w)
			}
		}
	}
	return v
}

func contains(list []string, item string) bool {
	for _, x := range list {
		if x == item {
			return true
		}
	}
	return false
}
