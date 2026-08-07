package config

import (
	"sort"
	"testing"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/gohcl"
	"github.com/spf13/afero"
)

// memFS builds an in-memory filesystem from a path -> content map, so the parser
// tests never touch the real disk and can describe awkward layouts (nested
// directories, hidden files, unreadable roots) inline.
func memFS(t *testing.T, files map[string]string) afero.Fs {
	t.Helper()

	fs := afero.NewMemMapFs()

	for name, body := range files {
		if err := afero.WriteFile(fs, name, []byte(body), 0o644); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}

	return fs
}

// providerNames returns the module's provider names, sorted, so assertions do
// not depend on map iteration order.
func providerNames(m *Module) []string {
	if m == nil {
		return nil
	}

	out := make([]string, 0, len(m.Providers))

	for name := range m.Providers {
		out = append(out, name)
	}

	sort.Strings(out)

	return out
}

func sorted(in []string) []string {
	out := append([]string(nil), in...)
	sort.Strings(out)

	return out
}

// decode evaluates a provider block's body into target using the parser's own
// eval context, which is how fabric.Configuration.ParseProvider consumes it.
func decode(t *testing.T, p *Parser, provider *Provider, target any) hcl.Diagnostics {
	t.Helper()

	return gohcl.DecodeBody(provider.HCL, p.Context(), target)
}
