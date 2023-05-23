package fabric

import "sort"

type BootableProvider interface {
	Name() string
	Priority() int
	Start() error
	Stop() error
}

type providerSorter struct {
	providers []BootableProvider
	by        func(left, right BootableProvider) bool
}

// Len is part of sort.Interface.
func (s *providerSorter) Len() int {
	return len(s.providers)
}

// Swap is part of sort.Interface.
func (s *providerSorter) Swap(i, j int) {
	s.providers[i], s.providers[j] = s.providers[j], s.providers[i]
}

// Less is part of sort.Interface. It is implemented by calling the "by" closure in the sorter.
func (s *providerSorter) Less(i, j int) bool {
	return s.by(s.providers[i], s.providers[j])
}

// By sorter.
type By func(left, right BootableProvider) bool

// Sort is a method on the function type, By, that sorts the argument slice according to the function.
func (by By) Sort(providers []BootableProvider) {
	ps := &providerSorter{
		providers: providers,
		by:        by, // The Sort method's receiver is the function (closure) that defines the sort order.
	}

	sort.Sort(ps)
}
