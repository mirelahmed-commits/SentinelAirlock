package adapters

import (
	"fmt"
	"sort"
	"strings"
)

type Registry struct {
	adapters map[string]Adapter
}

func NewRegistry() *Registry {
	r := &Registry{adapters: map[string]Adapter{}}
	r.Register(NewGenericShellAdapter())
	r.Register(NewCodexAdapter())
	r.Register(NewOllamaAdapter())
	return r
}

func (r *Registry) Register(a Adapter) {
	r.adapters[strings.ToLower(a.Name())] = a
}

func (r *Registry) Resolve(name string) (Adapter, error) {
	a, ok := r.adapters[strings.ToLower(strings.TrimSpace(name))]
	if ok {
		return a, nil
	}
	return nil, fmt.Errorf("unknown adapter %q (available: %s)", name, strings.Join(r.Available(), ", "))
}

func (r *Registry) Available() []string {
	out := make([]string, 0, len(r.adapters))
	for k := range r.adapters {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
