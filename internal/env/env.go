package env

import (
	"fmt"
	"os"
	"strings"
)

type File struct {
	path  string
	lines []string
}

func New(path string) *File {
	return &File{path: path}
}

func Read(path string) (*File, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	text := strings.TrimRight(string(raw), "\n")
	f := &File{path: path}
	if text != "" {
		f.lines = strings.Split(text, "\n")
	}
	return f, nil
}

func (f *File) index(key string) int {
	prefix := key + "="
	for i, line := range f.lines {
		if strings.HasPrefix(line, prefix) {
			return i
		}
	}
	return -1
}

func (f *File) Has(key string) bool {
	return f.index(key) >= 0
}

func (f *File) Get(key string) string {
	if i := f.index(key); i >= 0 {
		return strings.TrimPrefix(f.lines[i], key+"=")
	}
	return ""
}

func (f *File) List(key string) []string {
	return f.ListBy(key, ",")
}

func (f *File) ListBy(key, separator string) []string {
	var out []string
	for _, item := range strings.Split(f.Get(key), separator) {
		if item = strings.TrimSpace(item); item != "" {
			out = append(out, item)
		}
	}
	return out
}

func (f *File) Set(key, value string) {
	line := key + "=" + value
	if i := f.index(key); i >= 0 {
		f.lines[i] = line
		return
	}
	f.lines = append(f.lines, line)
}

func (f *File) Remove(key string) bool {
	i := f.index(key)
	if i < 0 {
		return false
	}
	f.lines = append(f.lines[:i], f.lines[i+1:]...)
	return true
}

func (f *File) Rename(old, now string) bool {
	if !f.Has(old) {
		return false
	}
	if !f.Has(now) {
		f.Set(now, f.Get(old))
	}
	f.Remove(old)
	return true
}

func (f *File) Write() error {
	return os.WriteFile(f.path, []byte(strings.Join(f.lines, "\n")+"\n"), 0o600)
}

func (f *File) Require(keys ...string) error {
	var missing []string
	for _, k := range keys {
		if f.Get(k) == "" {
			missing = append(missing, k)
		}
	}
	if len(missing) > 0 {
		return fmt.Errorf(".env lacks %s", strings.Join(missing, ", "))
	}
	return nil
}
