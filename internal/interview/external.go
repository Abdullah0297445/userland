package interview

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io"
	"strings"

	"github.com/Abdullah0297445/userland/internal/aws"
	"github.com/Abdullah0297445/userland/internal/env"
	"github.com/Abdullah0297445/userland/internal/manifest"
)

var admin *aws.Admin

func externals(c *manifest.Container, e *env.File, out io.Writer) ([]string, error) {
	var asked []string
	for _, prefix := range c.Externals() {
		x := *c.External[prefix]
		if complete(e, x.Variables(prefix)) {
			continue
		}
		var did []string
		var err error
		switch x.Kind {
		case manifest.Bucket:
			did, err = askBucket(prefix, x, e, out)
		case manifest.SecretStore:
			did, err = askSecretStore(prefix, x, e, out)
		}
		asked = append(asked, did...)
		if err != nil {
			return asked, err
		}
	}
	return asked, nil
}

func complete(e *env.File, names []string) bool {
	for _, n := range names {
		if e.Get(n) == "" {
			return false
		}
	}
	return true
}

type writer struct {
	prefix string
	e      *env.File
	asked  []string
}

func (w *writer) get(suffix string) string { return w.e.Get(w.prefix + "_" + suffix) }

func (w *writer) set(suffix, value string) {
	name := w.prefix + "_" + suffix
	w.e.Set(name, value)
	if !contains(w.asked, name) {
		w.asked = append(w.asked, name)
	}
}

func (w *writer) ask(x manifest.External, suffixes ...string) error {
	for _, a := range x.Asks(w.prefix) {
		if !contains(suffixes, strings.TrimPrefix(a.Var, w.prefix+"_")) || w.e.Get(a.Var) != "" {
			continue
		}
		value, err := Ask(a)
		if err != nil {
			return err
		}
		w.e.Set(a.Var, value)
		w.asked = append(w.asked, a.Var)
	}
	return nil
}

func (w *writer) keyless() bool {
	return w.get(manifest.AccessKeyID) == "" && w.get(manifest.SecretAccessKey) == ""
}

func askBucket(prefix string, x manifest.External, e *env.File, out io.Writer) ([]string, error) {
	w := &writer{prefix: prefix, e: e}
	if w.get(manifest.Name) == "" {
		name, err := askName(prefix+"_"+manifest.Name+": Name of the bucket", manifest.BucketName, func() string { return GenerateBucketName(prefix) })
		if err != nil {
			return w.asked, err
		}
		w.set(manifest.Name, name)
	}
	if err := w.ask(x, manifest.Region); err != nil {
		return w.asked, err
	}
	if w.keyless() {
		yes, err := offer(prefix, "the bucket, an AWS user named as it, its policy and an access key")
		if err != nil {
			return w.asked, err
		}
		if yes {
			return w.asked, createBucket(w, x, out)
		}
		fmt.Fprint(out, Checklist(prefix, x, w.get(manifest.Name), w.get(manifest.Region)))
	}
	return w.asked, w.ask(x, manifest.Endpoint, manifest.AccessKeyID, manifest.SecretAccessKey)
}

func askSecretStore(prefix string, x manifest.External, e *env.File, out io.Writer) ([]string, error) {
	w := &writer{prefix: prefix, e: e}
	if err := w.ask(x, manifest.Provider); err != nil {
		return w.asked, err
	}
	if w.get(manifest.Parameter) == "" {
		name, err := askName(prefix+"_"+manifest.Parameter+": Name of the parameter", manifest.ParameterName, func() string { return GenerateParameterName(prefix) })
		if err != nil {
			return w.asked, err
		}
		w.set(manifest.Parameter, name)
	}
	if err := w.ask(x, manifest.Region); err != nil {
		return w.asked, err
	}
	if w.keyless() {
		yes, err := offer(prefix, "the parameter with a generated master key, an AWS user, its read-only policy and an access key")
		if err != nil {
			return w.asked, err
		}
		if yes {
			return w.asked, createParameter(w, out)
		}
		fmt.Fprint(out, Checklist(prefix, x, w.get(manifest.Parameter), w.get(manifest.Region)))
	}
	return w.asked, w.ask(x, manifest.AccessKeyID, manifest.SecretAccessKey)
}

func askName(title, kind string, generate func() string) (string, error) {
	shape := Shape(kind)
	value, err := input(title+" (enter to generate, or type)", false, func(v string) error {
		if v == "" {
			return nil
		}
		return shape(v)
	})
	if err != nil {
		return "", err
	}
	if value == "" {
		return generate(), nil
	}
	return value, nil
}

func offer(prefix, what string) (bool, error) {
	return Confirm(prefix+": create it on AWS now?", "Yes makes "+what+", with admin credentials asked once for this run and never written. No prints the checklist to make it yourself, at any provider.")
}

func askAdmin() (*aws.Admin, error) {
	if admin != nil {
		return admin, nil
	}
	id, err := input("AWS admin access key id: used for this run and never written", true, Shape(manifest.Secret))
	if err != nil {
		return nil, err
	}
	secret, err := input("Its secret", true, Shape(manifest.Secret))
	if err != nil {
		return nil, err
	}
	token, err := input("Its session token, if it has one (enter for none)", true, Shape(manifest.Generated))
	if err != nil {
		return nil, err
	}
	admin = &aws.Admin{AccessKeyID: id, SecretAccessKey: secret, SessionToken: token}
	return admin, nil
}

func AdminUsed() bool {
	return admin != nil
}

func session(region string, out io.Writer) (aws.Session, string, error) {
	if err := aws.Pull(); err != nil {
		return aws.Session{}, "", err
	}
	a, err := askAdmin()
	if err != nil {
		return aws.Session{}, "", err
	}
	s := aws.Session{Admin: *a, Region: region}
	account, err := s.Account()
	if err != nil {
		return s, "", err
	}
	return s, account, nil
}

func createBucket(w *writer, x manifest.External, out io.Writer) error {
	s, _, err := session(w.get(manifest.Region), out)
	if err != nil {
		return err
	}
	name := w.get(manifest.Name)
	var adopted bool
	for {
		exists, err := s.HasBucket(name)
		if err == nil && exists {
			adopted = true
			break
		}
		if err == nil {
			adopted, err = s.CreateBucket(name)
			if err == nil {
				break
			}
		}
		if !aws.Taken(err) {
			return err
		}
		fmt.Fprintf(out, "%s: the bucket name %s belongs to another account; choose another\n", w.prefix, name)
		name, err = askName(w.prefix+"_"+manifest.Name+": Name of the bucket", manifest.BucketName, func() string { return GenerateBucketName(w.prefix) })
		if err != nil {
			return err
		}
		w.set(manifest.Name, name)
	}
	if x.Versioned {
		if err := s.EnableVersioning(name); err != nil {
			return err
		}
	}
	key, how, err := userAndKey(s, name, aws.BucketPolicy(name, x.Delete, x.Versioned))
	if err != nil {
		return err
	}
	w.set(manifest.Endpoint, aws.Endpoint(s.Region))
	w.set(manifest.AccessKeyID, key.ID)
	w.set(manifest.SecretAccessKey, key.Secret)
	fmt.Fprintf(out, "%s: %s bucket %s in %s%s, %s\n", w.prefix, madeOrAdopted(adopted), name, s.Region, versioningNote(x), how)
	return nil
}

func createParameter(w *writer, out io.Writer) error {
	s, account, err := session(w.get(manifest.Region), out)
	if err != nil {
		return err
	}
	name := w.get(manifest.Parameter)
	adopted, err := s.PutParameter(name, aws.MasterKey())
	if err != nil {
		return err
	}
	keyARN, err := s.ParameterKey()
	if err != nil {
		return err
	}
	key, how, err := userAndKey(s, aws.UserFromParameter(name), aws.ParameterPolicy(s.Region, account, name, keyARN))
	if err != nil {
		return err
	}
	w.set(manifest.AccessKeyID, key.ID)
	w.set(manifest.SecretAccessKey, key.Secret)
	fmt.Fprintf(out, "%s: %s parameter %s in %s, %s\n", w.prefix, madeOrAdopted(adopted), name, s.Region, how)
	return nil
}

func userAndKey(s aws.Session, user, policy string) (aws.Key, string, error) {
	adopted, err := s.CreateUser(user)
	if err != nil {
		return aws.Key{}, "", err
	}
	if err := s.PutUserPolicy(user, policy); err != nil {
		return aws.Key{}, "", err
	}
	how := madeOrAdopted(adopted) + " user " + user + " and wrote its policy, "
	if adopted {
		id, err := input("Paste this user's access key id, or enter to mint one", true, Shape(manifest.Generated))
		if err != nil {
			return aws.Key{}, "", err
		}
		if id != "" {
			secret, err := input("Its secret", true, Shape(manifest.Secret))
			if err != nil {
				return aws.Key{}, "", err
			}
			return aws.Key{ID: id, Secret: secret}, how + "kept the access key you pasted", nil
		}
	}
	key, err := s.CreateAccessKey(user)
	if aws.KeyLimit(err) {
		return aws.Key{}, "", fmt.Errorf("the user %s already holds two access keys, the most AWS allows; run again and paste one of them, or delete one in IAM first", user)
	}
	if err != nil {
		return aws.Key{}, "", err
	}
	return key, how + "minted an access key", nil
}

func madeOrAdopted(adopted bool) string {
	if adopted {
		return "adopted"
	}
	return "made"
}

func versioningNote(x manifest.External) string {
	if x.Versioned {
		return " with versioning on"
	}
	return ""
}

func GenerateBucketName(prefix string) string {
	return "userland-" + manifest.Dependency(prefix) + "-" + tail()
}

func GenerateParameterName(prefix string) string {
	return "/userland/" + manifest.Dependency(prefix) + "-" + tail()
}

func tail() string {
	b := make([]byte, 4)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	return hex.EncodeToString(b)
}

func Checklist(prefix string, x manifest.External, name, region string) string {
	var b strings.Builder
	switch x.Kind {
	case manifest.Bucket:
		fmt.Fprintf(&b, "\n%s needs a bucket. Make it yourself, at any S3-compatible provider:\n\n", prefix)
		fmt.Fprintf(&b, "1. Make a bucket named %s in region %s.\n", name, region)
		if x.Versioned {
			b.WriteString("   Turn versioning on. The container keeps history as versions and refuses to run\n   without it; the README says how to opt out, with history one deep.\n")
		}
		b.WriteString("2. Make an access key that reaches this bucket and nothing else. It must list the\n   bucket and get and put objects.\n")
		if x.Delete {
			b.WriteString("   It must also be able to delete.\n")
		} else {
			b.WriteString("   It must not be able to delete.\n")
		}
		b.WriteString("3. Decide retention. " + retention(x) + "\n")
		b.WriteString("4. Enter the endpoint URL, the access key id and its secret.\n\n")
		b.WriteString("README.md, \"Object store\": the exact AWS permission document, and notes for Backblaze B2\nand Cloudflare R2.\n\n")
	case manifest.SecretStore:
		fmt.Fprintf(&b, "\n%s needs a master key kept in AWS Parameter Store. Make it yourself:\n\n", prefix)
		fmt.Fprintf(&b, "1. Make a SecureString parameter named %s in region %s. Its value is the\n", name, region)
		b.WriteString("   master key: a long random string, such as the output of `openssl rand -base64 32`.\n   Keep it nowhere on this host. Lose the parameter and every archive is waste.\n")
		b.WriteString("2. Make an access key that may only read that parameter: ssm:GetParameter on it and\n   kms:Decrypt on the aws/ssm key, and nothing that writes.\n")
		b.WriteString("3. Enter the access key id and its secret.\n\n")
		b.WriteString("README.md, \"Secret store\": the exact AWS permission document.\n\n")
	}
	return b.String()
}

func retention(x manifest.External) string {
	switch {
	case x.Delete:
		return "The container deletes on its own schedule; a rule for what it leaves\n   behind is yours."
	case x.Versioned:
		return "Nothing in userland deletes from this bucket. Add a rule at your\n   provider that expires old versions, or accept that they pile up."
	}
	return "Nothing in userland deletes from this bucket. Add a rule at your\n   provider that expires objects after the days you want, or accept that it grows."
}

func Retention(prefix string, x manifest.External) string {
	switch {
	case x.Delete:
		return prefix + ": the container deletes from this bucket on its own schedule; a rule for what it leaves behind is yours."
	case x.Versioned:
		return prefix + ": nothing in userland deletes from this bucket; set a rule at your provider that expires old versions, or accept that they pile up."
	}
	return prefix + ": nothing in userland deletes from this bucket; set a rule at your provider that expires objects after the days you want, or accept that it grows."
}
