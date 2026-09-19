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

func (f *File) Get(key string) string {
	prefix := key + "="
	for _, line := range f.lines {
		if strings.HasPrefix(line, prefix) {
			return strings.TrimPrefix(line, prefix)
		}
	}
	return ""
}

func (f *File) List(key string) []string {
	var out []string
	for _, item := range strings.Split(f.Get(key), ",") {
		if item = strings.TrimSpace(item); item != "" {
			out = append(out, item)
		}
	}
	return out
}

func (f *File) Set(key, value string) {
	prefix := key + "="
	for i, line := range f.lines {
		if strings.HasPrefix(line, prefix) {
			f.lines[i] = prefix + value
			return
		}
	}
	f.lines = append(f.lines, prefix+value)
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
