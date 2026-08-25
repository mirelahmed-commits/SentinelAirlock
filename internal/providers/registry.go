package providers

import (
	"fmt"
	"sort"
	"strings"
)

type Registry struct {
	providers map[string]Provider
}

func NewRegistry() *Registry {
	r := &Registry{providers: map[string]Provider{}}
	r.Register(NewOpenAIProvider())
	r.Register(NewAnthropicProvider())
	r.Register(NewOllamaProvider())
	return r
}

func (r *Registry) Register(p Provider) {
	r.providers[strings.ToLower(p.Name())] = p
}

func (r *Registry) Resolve(name string) (Provider, error) {
	p, ok := r.providers[strings.ToLower(strings.TrimSpace(name))]
	if ok {
		return p, nil
	}
	return nil, fmt.Errorf("unknown provider %q (available: %s)", name, strings.Join(r.Available(), ", "))
}

func (r *Registry) Available() []string {
	out := make([]string, 0, len(r.providers))
	for k := range r.providers {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
