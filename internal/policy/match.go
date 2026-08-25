package policy

import (
	"path/filepath"
	"strings"
)

type Decision struct {
	Allowed bool
	Rule    string
}

func (c *Config) WriteDecision(relPath string) Decision {
	p := filepath.ToSlash(strings.TrimPrefix(relPath, "./"))

	// deny wins
	if matchAny(p, c.Policy.DenyWrite) {
		return Decision{Allowed: false, Rule: "deny_write"}
	}

	// if allow list empty -> allow everything not denied
	if len(c.Policy.AllowWrite) == 0 {
		return Decision{Allowed: true, Rule: "allow_write:<empty>"}
	}

	// otherwise must match allow list
	if matchAny(p, c.Policy.AllowWrite) {
		return Decision{Allowed: true, Rule: "allow_write"}
	}

	return Decision{Allowed: false, Rule: "allow_write:<no_match>"}
}

func matchAny(path string, globs []string) bool {
	for _, g := range globs {
		g = strings.TrimSpace(filepath.ToSlash(g))
		if g == "" {
			continue
		}

		// handle "x/**"
		if strings.HasSuffix(g, "/**") {
			prefix := strings.TrimSuffix(g, "/**")
			if path == prefix || strings.HasPrefix(path, prefix+"/") {
				return true
			}
			continue
		}

		// handle "**/x"
		if strings.HasPrefix(g, "**/") {
			suffix := strings.TrimPrefix(g, "**/")
			if path == suffix || strings.HasSuffix(path, "/"+suffix) {
				return true
			}
			continue
		}

		// fallback: filepath.Match supports "*" and "?"
		if ok, _ := filepath.Match(g, path); ok {
			return true
		}
	}
	return false
}
