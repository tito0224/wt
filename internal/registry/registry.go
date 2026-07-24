// Package registry provides a small generic name -> value registry shared by
// the adapter packages (terminal, forge, launcher). Each of those packages
// keeps a package-level registry that implementations populate from init()
// functions; this package factors out the common mechanics (duplicate
// detection, deterministic ordering) so they aren't hand-rolled three times.
package registry

import "sort"

// Registry maps stable string names to values of type T. It is intended to be
// populated from init() functions (single-goroutine, at program start) and
// read afterwards, so it performs no locking.
type Registry[T any] struct {
	// namespace and kind are used only in the duplicate-registration panic
	// message, e.g. namespace "terminal" + kind "adapter" yields
	// "terminal: duplicate adapter <name>".
	namespace, kind string
	items           map[string]T
}

// New returns an empty registry. namespace is the owning package name and kind
// the noun for the registered values (e.g. "adapter", "launcher"); both appear
// in the duplicate-registration panic message so failures point at the right
// package.
func New[T any](namespace, kind string) *Registry[T] {
	return &Registry[T]{
		namespace: namespace,
		kind:      kind,
		items:     map[string]T{},
	}
}

// Register adds v under name. It panics on a duplicate name: registration
// happens in init() functions, so a duplicate is a programming error best
// caught the first time the binary runs.
func (r *Registry[T]) Register(name string, v T) {
	if _, dup := r.items[name]; dup {
		panic(r.namespace + ": duplicate " + r.kind + " " + name)
	}
	r.items[name] = v
}

// Names returns the registered names in sorted order, so callers that iterate
// or display them behave deterministically.
func (r *Registry[T]) Names() []string {
	names := make([]string, 0, len(r.items))
	for n := range r.items {
		names = append(names, n)
	}
	sort.Strings(names)
	return names
}

// Get returns the value registered under name.
func (r *Registry[T]) Get(name string) (T, bool) {
	v, ok := r.items[name]
	return v, ok
}

// Values returns all registered values in sorted-name order. The deterministic
// order matters to callers that pick the "first" matching value (e.g. forge
// detection) or present a list to the user.
func (r *Registry[T]) Values() []T {
	names := r.Names()
	out := make([]T, len(names))
	for i, n := range names {
		out[i] = r.items[n]
	}
	return out
}
