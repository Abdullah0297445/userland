package manifest

import (
	"fmt"
	"regexp"
	"sort"
	"strings"
)

const (
	Bucket      = "bucket"
	SecretStore = "secret-store"

	NoDelete  = ""
	DeleteAll = "*"

	BucketName     = "bucket-name"
	BucketEndpoint = "bucket-endpoint"
	ParameterName  = "parameter-name"

	Endpoint        = "ENDPOINT"
	Name            = "BUCKET"
	Region          = "REGION"
	AccessKeyID     = "ACCESS_KEY_ID"
	SecretAccessKey = "SECRET_ACCESS_KEY"
	Provider        = "PROVIDER"
	Parameter       = "NAME"
)

var Kinds = []string{Bucket, SecretStore}

var Providers = []string{"ssm"}

var deletePrefix = regexp.MustCompile(`^[a-z0-9][a-z0-9._-]*(/[a-z0-9._-]+)*/\*$`)

type External struct {
	Kind        string `json:"kind"`
	Delete      string `json:"delete"`
	Versioned   bool   `json:"versioned"`
	NeverExpire bool   `json:"never_expire"`
}

func (x External) CanDelete() bool { return x.Delete != NoDelete }

func (x External) DeletesAnywhere() bool { return x.Delete == DeleteAll }

func (x External) DeleteUnder() string { return strings.TrimSuffix(x.Delete, "/*") }

func (x External) Describe() string {
	parts := []string{x.Kind}
	if x.Versioned {
		parts = append(parts, "versioned")
	}
	switch {
	case x.DeletesAnywhere():
		parts = append(parts, "delete")
	case x.CanDelete():
		parts = append(parts, "delete under "+x.DeleteUnder()+"/")
	}
	if x.NeverExpire {
		parts = append(parts, "never expire")
	}
	return strings.Join(parts, ", ")
}

func (x External) Asks(prefix string) []Ask {
	v := func(suffix string) string { return prefix + "_" + suffix }
	switch x.Kind {
	case Bucket:
		return []Ask{
			{Var: v(Name), Type: BucketName, Prompt: "Name of the bucket"},
			{Var: v(Region), Type: Text, Prompt: "Region of the bucket, as the provider names it"},
			{Var: v(Endpoint), Type: BucketEndpoint, Prompt: "Endpoint URL the bucket is reached at"},
			{Var: v(AccessKeyID), Type: Secret, Prompt: "Access key id that reaches this bucket and nothing else"},
			{Var: v(SecretAccessKey), Type: Secret, Prompt: "Its secret"},
		}
	case SecretStore:
		return []Ask{
			{Var: v(Provider), Type: Choice, Options: Providers, Prompt: "Secret store the master key lives in"},
			{Var: v(Parameter), Type: ParameterName, Prompt: "Name of the parameter"},
			{Var: v(Region), Type: Text, Prompt: "Region of the parameter"},
			{Var: v(AccessKeyID), Type: Secret, Prompt: "Access key id that may read this parameter and nothing else"},
			{Var: v(SecretAccessKey), Type: Secret, Prompt: "Its secret"},
		}
	}
	return nil
}

func (x External) Variables(prefix string) []string {
	var names []string
	for _, a := range x.Asks(prefix) {
		names = append(names, a.Var)
	}
	return names
}

func (x External) check(container, prefix string) error {
	if !ValidVariable(prefix) {
		return fmt.Errorf("manifest.json: %s names an external dependency %q, which is not a variable name", container, prefix)
	}
	if !contains(Kinds, x.Kind) {
		return fmt.Errorf("manifest.json: %s's %s has kind %q; the kinds are %s", container, prefix, x.Kind, strings.Join(Kinds, " "))
	}
	if x.Kind == SecretStore && (x.CanDelete() || x.Versioned || x.NeverExpire) {
		return fmt.Errorf("manifest.json: %s's %s is a secret store, which has no use for delete, versioned or never_expire", container, prefix)
	}
	if x.CanDelete() && !x.DeletesAnywhere() && !deletePrefix.MatchString(x.Delete) {
		return fmt.Errorf("manifest.json: %s's %s allows delete on %q; it is left out for never, %q for anywhere, or a prefix like %q", container, prefix, x.Delete, DeleteAll, "locks/*")
	}
	return nil
}

func Dependency(prefix string) string {
	return strings.ReplaceAll(strings.ToLower(prefix), "_", "-")
}

func (c *Container) Externals() []string {
	var prefixes []string
	for prefix := range c.External {
		prefixes = append(prefixes, prefix)
	}
	sort.Strings(prefixes)
	return prefixes
}

func (c *Container) ExternalAsks() []Ask {
	var asks []Ask
	for _, prefix := range c.Externals() {
		asks = append(asks, c.External[prefix].Asks(prefix)...)
	}
	return asks
}

func (m *Manifest) External(prefix string) (*Container, External, bool) {
	for _, c := range m.All() {
		if x, ok := c.External[prefix]; ok {
			return c, *x, true
		}
	}
	return nil, External{}, false
}
