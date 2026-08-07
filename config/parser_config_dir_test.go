package config

import (
	"testing"

	"github.com/hashicorp/hcl/v2"
	"github.com/spf13/afero"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFileExt(t *testing.T) {
	cases := map[string]string{
		"config.hcl":     ".hcl",
		"a/b/config.hcl": ".hcl",
		"config.json":    "",
		"config.HCL":     "", // the check is case-sensitive
		"config":         "",
		"":               "",
		".hcl":           ".hcl",
	}

	for name, want := range cases {
		assert.Equal(t, want, fileExt(name), "fileExt(%q)", name)
	}
}

func TestIsIgnoredFile(t *testing.T) {
	ignored := []string{".hidden.hcl", ".config.hcl", "config.hcl~", "#config.hcl#"}
	kept := []string{"config.hcl", "config_override.hcl", "a.b.hcl", "#config.hcl", "config.hcl#"}

	for _, name := range ignored {
		assert.True(t, IsIgnoredFile(name), "%q should be ignored", name)
	}

	for _, name := range kept {
		assert.False(t, IsIgnoredFile(name), "%q should be kept", name)
	}
}

func TestParser_ConfigDirFiles_ClassifiesPrimaryAndOverride(t *testing.T) {
	p := NewParser(memFS(t, map[string]string{
		"/cfg/config.hcl":          `provider "a" {}`,
		"/cfg/extra.hcl":           `provider "b" {}`,
		"/cfg/override.hcl":        `provider "a" {}`,
		"/cfg/config_override.hcl": `provider "a" {}`,
		"/cfg/notes.txt":           "not hcl",
		"/cfg/.hidden.hcl":         `provider "hidden" {}`,
		"/cfg/backup.hcl~":         `provider "backup" {}`,
	}))

	primary, override, diags := p.ConfigDirFiles("/cfg")

	require.False(t, diags.HasErrors(), diags.Error())
	assert.Equal(t, []string{"/cfg/config.hcl", "/cfg/extra.hcl"}, sorted(primary))
	assert.Equal(t, []string{"/cfg/config_override.hcl", "/cfg/override.hcl"}, sorted(override))
}

// The walk is recursive, so a nested directory is part of the configuration.
func TestParser_ConfigDirFiles_IsRecursive(t *testing.T) {
	p := NewParser(memFS(t, map[string]string{
		"/cfg/a.hcl":           `provider "a" {}`,
		"/cfg/sub/b.hcl":       `provider "b" {}`,
		"/cfg/sub/deep/c.hcl":  `provider "c" {}`,
		"/cfg/sub/notes.md":    "ignored",
		"/cfg/sub/.skip.hcl":   `provider "skip" {}`,
		"/cfg/sub/deep/d.json": "ignored",
	}))

	primary, _, diags := p.ConfigDirFiles("/cfg")

	require.False(t, diags.HasErrors(), diags.Error())
	assert.Equal(t, []string{"/cfg/a.hcl", "/cfg/sub/b.hcl", "/cfg/sub/deep/c.hcl"}, sorted(primary))
}

// A wrong config path is an ordinary misconfiguration and must produce a
// diagnostic. afero.Walk reports a missing root by invoking the walk function
// once with a nil FileInfo; ignoring that error stored the nil and the caller
// then dereferenced it, crashing the process.
func TestParser_LoadConfigDir_MissingDirectoryDoesNotPanic(t *testing.T) {
	p := NewParser(afero.NewMemMapFs())

	var (
		mod   *Module
		diags hcl.Diagnostics
	)

	require.NotPanics(t, func() { mod, diags = p.LoadConfigDir("/does/not/exist") })

	require.True(t, diags.HasErrors())
	assert.Contains(t, diags.Error(), "does not exist or cannot be read")
	assert.Nil(t, mod, "no module is returned when the directory cannot be read")
}

func TestParser_LoadConfigDir(t *testing.T) {
	p := NewParser(memFS(t, map[string]string{
		"/cfg/vars.hcl": `
variable "db_port" {
  value = 3306
}`,
		"/cfg/providers.hcl": `
provider "mysql" {
  port = var.db_port
}

provider "logger" {
  level = "debug"
}`,
	}))

	mod, diags := p.LoadConfigDir("/cfg")

	require.False(t, diags.HasErrors(), diags.Error())
	require.NotNil(t, mod)
	assert.Equal(t, "/cfg", mod.SourceDir)
	assert.Equal(t, []string{"logger", "mysql"}, providerNames(mod))
}

func TestParser_LoadConfigDir_EmptyDirectoryIsNotAnError(t *testing.T) {
	fs := afero.NewMemMapFs()
	require.NoError(t, fs.MkdirAll("/cfg", 0o755))

	mod, diags := NewParser(fs).LoadConfigDir("/cfg")

	require.False(t, diags.HasErrors(), diags.Error())
	require.NotNil(t, mod)
	assert.Empty(t, mod.Providers)
}

func TestParser_LoadConfigDir_PropagatesParseErrors(t *testing.T) {
	p := NewParser(memFS(t, map[string]string{
		"/cfg/good.hcl":   `provider "a" {}`,
		"/cfg/broken.hcl": `provider "b" {`,
	}))

	_, diags := p.LoadConfigDir("/cfg")

	assert.True(t, diags.HasErrors())
}

// Two files declaring the same provider name is a conflict: the first wins and a
// diagnostic reports the duplicate, rather than one file silently shadowing the
// other.
func TestParser_LoadConfigDir_DuplicateProvider(t *testing.T) {
	p := NewParser(memFS(t, map[string]string{
		"/cfg/a.hcl": `provider "dup" { value = "first" }`,
		"/cfg/b.hcl": `provider "dup" { value = "second" }`,
	}))

	mod, diags := p.LoadConfigDir("/cfg")

	require.True(t, diags.HasErrors())
	assert.Contains(t, diags.Error(), "Duplicate provider")
	assert.Equal(t, []string{"dup"}, providerNames(mod))
}

// KNOWN LIMITATION, pinned deliberately: override files are discovered and
// parsed, but Module.mergeFile is an empty stub, so their content is silently
// discarded. Anyone implementing the merge should expect this test to fail and
// update it, rather than discovering the gap in production.
func TestParser_LoadConfigDir_OverrideFilesAreParsedButNotMerged(t *testing.T) {
	p := NewParser(memFS(t, map[string]string{
		"/cfg/config.hcl":          `provider "a" { value = "primary" }`,
		"/cfg/config_override.hcl": `provider "a" { value = "override" }`,
	}))

	mod, diags := p.LoadConfigDir("/cfg")
	require.False(t, diags.HasErrors(), diags.Error())

	var target struct {
		Value string `hcl:"value"`
	}

	require.False(t, decode(t, p, mod.Providers["a"], &target).HasErrors())
	assert.Equal(t, "primary", target.Value, "override merging is not implemented")
}

func TestParser_IsConfigDir(t *testing.T) {
	withConfig := NewParser(memFS(t, map[string]string{"/cfg/a.hcl": `provider "a" {}`}))
	assert.True(t, withConfig.IsConfigDir("/cfg"))

	onlyOverride := NewParser(memFS(t, map[string]string{"/cfg/override.hcl": `provider "a" {}`}))
	assert.True(t, onlyOverride.IsConfigDir("/cfg"))

	noHCL := NewParser(memFS(t, map[string]string{"/cfg/notes.txt": "nope"}))
	assert.False(t, noHCL.IsConfigDir("/cfg"))

	assert.False(t, NewParser(afero.NewMemMapFs()).IsConfigDir("/missing"))
}

func TestIsEmptyDir(t *testing.T) {
	empty, err := IsEmptyDir("./testdata/empty")
	require.NoError(t, err)
	assert.True(t, empty, "a directory with no .hcl file counts as empty")

	notEmpty, err := IsEmptyDir("./testdata/valid")
	require.NoError(t, err)
	assert.False(t, notEmpty)

	// A path that does not exist is reported as empty rather than as an error.
	missing, err := IsEmptyDir("./testdata/does-not-exist")
	require.NoError(t, err)
	assert.True(t, missing)
}
