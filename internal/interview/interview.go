package interview

import (
	"errors"
	"fmt"
	"io"
	"sort"
	"strings"

	"github.com/Abdullah0297445/userland/internal/env"
	"github.com/Abdullah0297445/userland/internal/manifest"
)

const (
	Visibility = "VISIBILITY"
	On         = "USERLAND_ON"
	Off        = "USERLAND_OFF"
)

var ErrAborted = errors.New("interview aborted; nothing was written")

type Refusal struct {
	Reasons []string
}

func (r Refusal) Error() string {
	return "refused:\n  " + strings.Join(r.Reasons, "\n  ")
}

type Result struct {
	Visibility string
	Offered    []string
	On         []string
	Off        []string
	Asked      []string
	Warnings   []string
}

func (r *Result) Changed() bool {
	return r.Visibility != "" || len(r.Offered) > 0 || len(r.Asked) > 0
}

func Run(m *manifest.Manifest, e *env.File, out io.Writer) (*Result, error) {
	r := &Result{}
	if e.Get(Visibility) == "" {
		v, err := SelectVisibility()
		if err != nil {
			return nil, err
		}
		e.Set(Visibility, v)
		r.Visibility = v
	}
	on, off := e.List(On), e.List(Off)
	for _, product := range m.ProductOrder() {
		var fresh []*manifest.Container
		for _, c := range m.ProductContainers(product) {
			if contains(on, c.Name) || contains(off, c.Name) {
				continue
			}
			fresh = append(fresh, c)
		}
		if len(fresh) == 0 {
			continue
		}
		picked, err := Gate(m, product, fresh, nil, "Tick what to switch on. Anything left unticked is recorded as off and not asked again.")
		if err != nil {
			return nil, err
		}
		for _, c := range fresh {
			r.Offered = append(r.Offered, c.Name)
			if contains(picked, c.Name) {
				on = append(on, c.Name)
			} else {
				off = append(off, c.Name)
			}
		}
	}
	sort.Strings(on)
	sort.Strings(off)
	verdict := m.Validate(on)
	if len(verdict.Refusals) > 0 {
		return nil, Refusal{verdict.Refusals}
	}
	for _, w := range verdict.Warnings {
		fmt.Fprintln(out, "warning: "+w)
	}
	r.Warnings = verdict.Warnings
	r.On, r.Off = on, off
	e.Set(On, strings.Join(on, ","))
	e.Set(Off, strings.Join(off, ","))
	asked, err := Variables(m, e, out)
	r.Asked = asked
	if err != nil {
		return nil, err
	}
	return r, nil
}

func SelectVisibility() (string, error) {
	return selectOne("Visibility", "Chosen once, for the whole of userland.", []option{
		{label: "local: this machine only, on *.localhost, with no domain and no certificate", value: "local"},
		{label: "public: real hostnames under your domain, with TLS from a DNS-01 challenge", value: "public"},
	})
}

func Gate(m *manifest.Manifest, product string, containers []*manifest.Container, ticked []string, description string) ([]string, error) {
	var options []option
	for _, c := range containers {
		options = append(options, option{label(m, c), c.Name, contains(ticked, c.Name)})
	}
	return selectMany(product, description, options)
}

func Variables(m *manifest.Manifest, e *env.File, out io.Writer) ([]string, error) {
	on := e.List(On)
	var asked []string
	for _, product := range m.ProductOrder() {
		for _, c := range m.ProductContainers(product) {
			if !contains(on, c.Name) {
				continue
			}
			for _, a := range append(append([]manifest.Ask{}, c.Asks...), c.PasswordAsks()...) {
				if !a.Applies(e.Get(Visibility), e.Get) || e.Get(a.Var) != "" {
					continue
				}
				value, err := Ask(a)
				if err != nil {
					return asked, err
				}
				e.Set(a.Var, value)
				asked = append(asked, a.Var)
			}
			did, err := externals(c, e, out)
			asked = append(asked, did...)
			if err != nil {
				return asked, err
			}
		}
	}
	if admin != nil {
		fmt.Fprintln(out, "the admin credentials were used for this run and not kept")
		admin = nil
	}
	return asked, nil
}

func Ask(a manifest.Ask) (string, error) {
	title := a.Var
	if a.Prompt != "" {
		title += ": " + a.Prompt
	}
	switch a.Type {
	case manifest.Choice:
		var options []option
		for _, o := range a.Options {
			options = append(options, option{label: o, value: o})
		}
		return selectOne(title, "", options)
	case manifest.Generated:
		value, err := input(title+" (enter to generate, or paste)", true, Shape(a.Type))
		if err != nil {
			return "", err
		}
		if value == "" {
			return Generate(), nil
		}
		return value, nil
	default:
		return input(title, a.Hidden(), Shape(a.Type))
	}
}

func label(m *manifest.Manifest, c *manifest.Container) string {
	var notes []string
	if by := m.Blocking(c.Name); len(by) > 0 {
		notes = append(notes, "required by "+strings.Join(by, ", "))
	}
	if by := m.OptionalFor(c.Name); len(by) > 0 {
		notes = append(notes, "optional for "+strings.Join(by, ", "))
	}
	if len(notes) == 0 {
		return c.Name
	}
	return c.Name + ": " + strings.Join(notes, "; ")
}

func contains(list []string, item string) bool {
	for _, x := range list {
		if x == item {
			return true
		}
	}
	return false
}
