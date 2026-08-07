package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zclconf/go-cty/cty"
)

func provider(name string) *Provider {
	return &Provider{Name: name}
}

func TestNewModule_Empty(t *testing.T) {
	mod, diags := NewModule(nil, nil)

	require.False(t, diags.HasErrors())
	require.NotNil(t, mod)
	assert.NotNil(t, mod.Providers, "Providers must be usable without a nil check")
	assert.NotNil(t, mod.Variables)
	assert.Empty(t, mod.Providers)
}

func TestNewModule_CombinesPrimaryFiles(t *testing.T) {
	mod, diags := NewModule([]*File{
		{Providers: []*Provider{provider("a"), provider("b")}},
		{Providers: []*Provider{provider("c")}},
	}, nil)

	require.False(t, diags.HasErrors(), diags.Error())
	assert.Equal(t, []string{"a", "b", "c"}, providerNames(mod))
}

// A duplicate declaration is reported, and the first declaration wins rather
// than being silently replaced: two blocks configuring the same provider would
// otherwise resolve by map iteration order.
func TestNewModule_DuplicateProviderKeepsFirst(t *testing.T) {
	first := provider("dup")
	second := provider("dup")

	mod, diags := NewModule([]*File{
		{Providers: []*Provider{first}},
		{Providers: []*Provider{second}},
	}, nil)

	require.True(t, diags.HasErrors())
	require.Len(t, diags, 1)
	assert.Contains(t, diags[0].Summary, `Duplicate provider "dup"`)
	assert.Same(t, first, mod.Providers["dup"])
}

func TestNewModule_DuplicateWithinASingleFile(t *testing.T) {
	_, diags := NewModule([]*File{
		{Providers: []*Provider{provider("dup"), provider("dup")}},
	}, nil)

	assert.True(t, diags.HasErrors())
}

// KNOWN LIMITATION, pinned deliberately: mergeFile is an empty stub, so an
// override file contributes nothing and raises no diagnostic. See
// TestParser_LoadConfigDir_OverrideFilesAreParsedButNotMerged.
func TestNewModule_OverrideFilesAreIgnored(t *testing.T) {
	mod, diags := NewModule(
		[]*File{{Providers: []*Provider{provider("a")}}},
		[]*File{{Providers: []*Provider{provider("b")}}},
	)

	require.False(t, diags.HasErrors())
	assert.Equal(t, []string{"a"}, providerNames(mod), "override merging is not implemented")
}

func TestFile_Schema(t *testing.T) {
	p := NewParser(nil)

	file, diags := p.Parse("test.hcl", []byte(`
variable "one" {
  value = 1
}

variable "two" {
  value = "deux"
}

provider "alpha" {
  a = 1
}

provider "beta" {
  b = 2
}
`))

	require.False(t, diags.HasErrors(), diags.Error())

	require.Len(t, file.Variables, 2)
	assert.Equal(t, "one", file.Variables[0].Name)
	// cty numbers carry a big.Float whose precision depends on how the value was
	// produced, so they must be compared with Equals rather than reflect equality.
	assert.True(t, cty.NumberIntVal(1).Equals(file.Variables[0].Value).True())
	assert.Equal(t, "two", file.Variables[1].Name)
	assert.True(t, cty.StringVal("deux").Equals(file.Variables[1].Value).True())

	require.Len(t, file.Providers, 2)
	assert.Equal(t, "alpha", file.Providers[0].Name)
	assert.Equal(t, "beta", file.Providers[1].Name)
}
