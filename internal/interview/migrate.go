package interview

import (
	"fmt"
	"sort"

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
	return did
}
