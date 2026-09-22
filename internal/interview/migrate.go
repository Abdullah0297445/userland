package interview

import (
	"fmt"
	"sort"
	"strings"

	"github.com/Abdullah0297445/userland/internal/env"
	"github.com/Abdullah0297445/userland/internal/manifest"
)

func Migrate(m *manifest.Manifest, e *env.File) []string {
	var did []string
	for _, c := range m.All() {
		var olds []string
		for old := range c.Renamed {
			olds = append(olds, old)
		}
		sort.Strings(olds)
		for _, old := range olds {
			if e.Rename(old, c.Renamed[old]) {
				did = append(did, fmt.Sprintf("moved %s to %s, its name since %s renamed it", old, c.Renamed[old], c.Name))
			}
		}
		for _, name := range c.Removed {
			if e.Remove(name) {
				did = append(did, fmt.Sprintf("dropped %s, which %s no longer reads", name, c.Name))
			}
		}
	}
	for _, key := range []string{On, Off} {
		var known, gone []string
		for _, name := range e.List(key) {
			if m.Container(name) == nil {
				gone = append(gone, name)
			} else {
				known = append(known, name)
			}
		}
		if len(gone) == 0 {
			continue
		}
		e.Set(key, strings.Join(known, ","))
		for _, name := range gone {
			did = append(did, fmt.Sprintf("dropped %s from %s, since the manifest no longer has a container of that name", name, key))
		}
	}
	return did
}
