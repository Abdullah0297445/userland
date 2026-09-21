package interview

import (
	"crypto/rand"
	"errors"
	"fmt"
	"net/mail"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/Abdullah0297445/userland/internal/manifest"
)

var (
	hostname  = regexp.MustCompile(`^([a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?\.)*[a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?$`)
	bucket    = regexp.MustCompile(`^[a-z0-9][a-z0-9.-]{1,61}[a-z0-9]$`)
	parameter = regexp.MustCompile(`^/?[A-Za-z0-9_.-]+(/[A-Za-z0-9_.-]+)*$`)
	numeric   = regexp.MustCompile(`^[0-9.]+$`)
)

func Generate() string {
	return rand.Text()
}

func Shape(kind string) func(string) error {
	switch kind {
	case manifest.Generated:
		return func(v string) error {
			if v == "" {
				return nil
			}
			return safe(v)
		}
	case manifest.Hostname:
		return func(v string) error {
			if err := present(v); err != nil {
				return err
			}
			if len(v) > 253 || !hostname.MatchString(strings.ToLower(v)) {
				return errors.New("not a hostname: letters, digits and hyphens, in labels joined by dots")
			}
			return nil
		}
	case manifest.Email:
		return func(v string) error {
			if err := present(v); err != nil {
				return err
			}
			address, err := mail.ParseAddress(v)
			if err != nil || address.Address != v || !strings.Contains(v, ".") {
				return errors.New("not an email address, like name@example.com")
			}
			return nil
		}
	case manifest.URL:
		return func(v string) error {
			if err := present(v); err != nil {
				return err
			}
			u, err := url.Parse(v)
			if err != nil || u.Scheme == "" || u.Host == "" {
				return errors.New("not a URL: it needs a scheme and a host, like https://example.com")
			}
			return nil
		}
	case manifest.Port:
		return func(v string) error {
			if err := present(v); err != nil {
				return err
			}
			n, err := strconv.Atoi(v)
			if err != nil || n < 1 || n > 65535 {
				return errors.New("not a port: a number from 1 to 65535")
			}
			return nil
		}
	case manifest.BucketName:
		return func(v string) error {
			if err := present(v); err != nil {
				return err
			}
			if !bucket.MatchString(v) || strings.Contains(v, "..") || numeric.MatchString(v) {
				return errors.New("not a bucket name: 3 to 63 lower-case letters, digits, hyphens and dots, starting and ending with a letter or digit")
			}
			return nil
		}
	case manifest.ParameterName:
		return func(v string) error {
			if err := present(v); err != nil {
				return err
			}
			first := strings.ToLower(strings.TrimPrefix(v, "/"))
			if !parameter.MatchString(v) || strings.HasPrefix(first, "aws") || strings.HasPrefix(first, "ssm") {
				return errors.New("not a parameter name: letters, digits, . _ - and / as a path, like /userland/fort-key, and not starting with aws or ssm")
			}
			return nil
		}
	case manifest.Paths:
		return func(v string) error {
			if err := present(v); err != nil {
				return err
			}
			for _, p := range strings.Split(v, ":") {
				if !filepath.IsAbs(p) {
					return fmt.Errorf("%s is not an absolute path", p)
				}
				if _, err := os.Stat(p); err != nil {
					return fmt.Errorf("%s does not exist on this machine", p)
				}
			}
			return nil
		}
	default:
		return present
	}
}

func present(v string) error {
	if v == "" {
		return errors.New("this cannot be empty")
	}
	return safe(v)
}

func safe(v string) error {
	if strings.ContainsAny(v, "\n\r") {
		return errors.New("one line only")
	}
	if strings.TrimSpace(v) != v {
		return errors.New("no space at the start or the end")
	}
	if strings.ContainsAny(v, "$#\"'`") {
		return errors.New(".env is read unquoted, so a value may not contain $ # \" ' or `")
	}
	return nil
}
