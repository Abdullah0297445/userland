package manifest

import (
	"encoding/json"
	"fmt"
	"os"
	"regexp"
	"sort"
)

type Manifest struct {
	Products map[string]*Product `json:"products"`
}

type Product struct {
	Containers map[string]*Container `json:"containers"`
}

type Container struct {
	Name     string   `json:"-"`
	Product  string   `json:"-"`
	Requires []string `json:"requires"`
	Optional []string `json:"optional"`
	HTTP     *HTTP    `json:"http"`
	Tenant   *Tenant  `json:"tenant"`
	Ports    []int    `json:"ports"`
	Volumes  []string `json:"volumes"`
	Asks     []Ask    `json:"asks"`
}

type HTTP struct {
	Container int    `json:"container"`
	Host      int    `json:"host"`
	Subdomain string `json:"subdomain"`
}

type Tenant struct {
	Name     string `json:"name"`
	Password string `json:"password"`
}

type Ask struct {
	Var     string   `json:"var"`
	Type    string   `json:"type"`
	Prompt  string   `json:"prompt"`
	When    string   `json:"when"`
	Options []string `json:"options"`
	Keep    bool     `json:"keep"`
}

var identifier = regexp.MustCompile(`^[a-z][a-z0-9_]{0,62}$`)

func Load(path string) (*Manifest, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
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
			if c.Tenant != nil && !identifier.MatchString(c.Tenant.Name) {
				return nil, fmt.Errorf("manifest.json: tenant %q of %s is not a valid Postgres identifier", c.Tenant.Name, name)
			}
		}
	}
	volumes := map[string]string{}
	hostPorts := map[int]string{}
	for _, c := range m.All() {
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

func (m *Manifest) Container(name string) *Container {
	for _, p := range m.Products {
		if c, ok := p.Containers[name]; ok {
			return c
		}
	}
	return nil
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
		for _, r := range c.Requires {
			if r == name {
				dependents = append(dependents, c.Name)
			}
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
		if c.Tenant != nil && !set[Postgres] {
			v.Refusals = append(v.Refusals, fmt.Sprintf("%s is a Postgres tenant, so %s must be on", c.Name, Postgres))
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
