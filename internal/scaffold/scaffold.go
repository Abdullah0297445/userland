package scaffold

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/Abdullah0297445/userland/internal/manifest"
	"github.com/Abdullah0297445/userland/internal/render"
)

const Placeholder = "replace-me"

func Product(root, product string, containers []string) ([]string, error) {
	if !manifest.ValidName(product) {
		return nil, fmt.Errorf("%q is not a product name: lower-case letters, digits and hyphens, starting with a letter", product)
	}
	seen := map[string]bool{}
	for _, c := range containers {
		if !manifest.ValidName(c) {
			return nil, fmt.Errorf("%q is not a container name: lower-case letters, digits and hyphens, starting with a letter", c)
		}
		if seen[c] {
			return nil, fmt.Errorf("%s is named twice", c)
		}
		seen[c] = true
	}
	manifestPath := filepath.Join(root, "manifest.json")
	m, err := manifest.Load(manifestPath)
	if err != nil {
		return nil, err
	}
	if _, exists := m.Products[product]; exists {
		return nil, fmt.Errorf("the product %s is already in manifest.json", product)
	}
	for _, c := range containers {
		if have := m.Container(c); have != nil {
			return nil, fmt.Errorf("the container %s already belongs to %s", c, have.Product)
		}
	}
	templatePath := filepath.Join(root, "compose", product+".yml")
	if _, err := os.Stat(templatePath); err == nil {
		return nil, fmt.Errorf("compose/%s.yml already exists", product)
	}
	raw, err := os.ReadFile(manifestPath)
	if err != nil {
		return nil, err
	}
	out, err := insert(raw, product, containers)
	if err != nil {
		return nil, err
	}
	if _, err := manifest.Parse(out); err != nil {
		return nil, fmt.Errorf("the new block does not load: %w", err)
	}
	if err := os.WriteFile(manifestPath, out, 0o644); err != nil {
		return nil, err
	}
	if err := os.WriteFile(templatePath, []byte(Template(containers)), 0o644); err != nil {
		return nil, err
	}
	return []string{"manifest.json", "compose/" + product + ".yml"}, nil
}

func Block(product string, containers []string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "    %q: {\n      \"containers\": {\n", product)
	for i, c := range containers {
		fmt.Fprintf(&b, "        %q: {\n          \"requires\": [],\n          \"optional\": [],\n          \"volumes\": [],\n          \"asks\": []\n        }", c)
		if i < len(containers)-1 {
			b.WriteString(",")
		}
		b.WriteString("\n")
	}
	b.WriteString("      }\n    }")
	return b.String()
}

func Template(containers []string) string {
	var parts []string
	for _, c := range containers {
		parts = append(parts, fmt.Sprintf("{{ define %q -}}\nimage: %s\nenvironment:\n  %s\n{{ end }}\n", c, Placeholder, render.MergeLine))
	}
	return strings.Join(parts, "\n")
}

func insert(raw []byte, product string, containers []string) ([]byte, error) {
	open, close, err := productsSpan(raw)
	if err != nil {
		return nil, err
	}
	end := close
	for end > open+1 && isSpace(raw[end-1]) {
		end--
	}
	var block bytes.Buffer
	if end > open+1 {
		block.WriteString(",\n")
	} else {
		block.WriteString("\n")
	}
	block.WriteString(Block(product, containers))
	if end == open+1 {
		block.WriteString("\n  ")
	}
	out := make([]byte, 0, len(raw)+block.Len())
	out = append(out, raw[:end]...)
	out = append(out, block.Bytes()...)
	out = append(out, raw[end:]...)
	return out, nil
}

func productsSpan(raw []byte) (int, int, error) {
	key := []byte(`"products"`)
	i := bytes.Index(raw, key)
	if i < 0 {
		return 0, 0, errors.New(`manifest.json has no "products" key`)
	}
	j := i + len(key)
	for j < len(raw) && isSpace(raw[j]) {
		j++
	}
	if j >= len(raw) || raw[j] != ':' {
		return 0, 0, errors.New(`manifest.json: "products" is not followed by a colon`)
	}
	j++
	for j < len(raw) && isSpace(raw[j]) {
		j++
	}
	if j >= len(raw) || raw[j] != '{' {
		return 0, 0, errors.New(`manifest.json: "products" is not an object`)
	}
	open := j
	depth, inString, escaped := 0, false, false
	for k := open; k < len(raw); k++ {
		b := raw[k]
		if inString {
			switch {
			case escaped:
				escaped = false
			case b == '\\':
				escaped = true
			case b == '"':
				inString = false
			}
			continue
		}
		switch b {
		case '"':
			inString = true
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				return open, k, nil
			}
		}
	}
	return 0, 0, errors.New(`manifest.json: "products" is never closed`)
}

func isSpace(b byte) bool {
	return b == ' ' || b == '\n' || b == '\t' || b == '\r'
}
