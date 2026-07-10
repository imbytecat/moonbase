// Package integration contains presentation and application purpose catalog
// vocabulary shared by infrastructure integrations. Provider execution and
// typed registries remain integration-specific.
package integration

import "slices"

type Cardinality string

const (
	Single   Cardinality = "single"
	Multiple Cardinality = "multiple"
)

type Purpose struct {
	Key         string
	Name        string
	Description string
	Cardinality Cardinality
}
type Catalog []Purpose

func (c Catalog) Known(key string) bool {
	return slices.ContainsFunc(c, func(p Purpose) bool { return p.Key == key })
}
func (c Catalog) Keys() []string {
	out := make([]string, len(c))
	for i, p := range c {
		out[i] = p.Key
	}
	return out
}

type Presentation struct {
	Name        string
	Description string
	Color       string
	IconRef     string
}
