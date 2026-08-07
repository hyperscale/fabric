package config

import (
	"io/fs"
	"testing"

	"github.com/hashicorp/hcl/v2"
	"github.com/spf13/afero"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParser_LoadConfigFile(t *testing.T) {
	p := NewParser(memFS(t, map[string]string{
		"/cfg/config.hcl": `
provider "mysql" {
  host = "localhost"
}`,
	}))

	file, diags := p.LoadConfigFile("/cfg/config.hcl")

	require.False(t, diags.HasErrors(), diags.Error())
	require.NotNil(t, file)
	require.Len(t, file.Providers, 1)
	assert.Equal(t, "mysql", file.Providers[0].Name)
}

func TestParser_LoadConfigFile_NotFound(t *testing.T) {
	p := NewParser(afero.NewMemMapFs())

	file, diags := p.LoadConfigFile("/cfg/missing.hcl")

	require.True(t, diags.HasErrors())
	assert.Nil(t, file)
	assert.Contains(t, diags.Error(), "Configuration file not found")
}

func TestParser_LoadConfigFile_SyntaxError(t *testing.T) {
	p := NewParser(memFS(t, map[string]string{
		"/cfg/broken.hcl": `provider "mysql" {`,
	}))

	file, diags := p.LoadConfigFile("/cfg/broken.hcl")

	require.True(t, diags.HasErrors())
	assert.Nil(t, file)
}

// LoadConfigFileOverride is documented as relaxing required-attribute
// constraints, but it currently behaves exactly like LoadConfigFile: the
// override flag is threaded through loadConfigFile and never used. Pinned so the
// equivalence is explicit rather than assumed.
func TestParser_LoadConfigFileOverride_BehavesLikeLoadConfigFile(t *testing.T) {
	files := map[string]string{
		"/cfg/config.hcl": `
provider "mysql" {
  host = "localhost"
}`,
	}

	primary, primaryDiags := NewParser(memFS(t, files)).LoadConfigFile("/cfg/config.hcl")
	override, overrideDiags := NewParser(memFS(t, files)).LoadConfigFileOverride("/cfg/config.hcl")

	require.False(t, primaryDiags.HasErrors(), primaryDiags.Error())
	require.False(t, overrideDiags.HasErrors(), overrideDiags.Error())

	require.Len(t, primary.Providers, 1)
	require.Len(t, override.Providers, 1)
	assert.Equal(t, primary.Providers[0].Name, override.Providers[0].Name)
}

// unreadableFS makes every Open fail with something other than "not exist", to
// reach the second error branch of loadConfigFile.
type unreadableFS struct {
	afero.Fs
}

func (unreadableFS) Open(string) (afero.File, error) {
	return nil, fs.ErrPermission
}

func TestParser_LoadConfigFile_ReadError(t *testing.T) {
	p := NewParser(unreadableFS{Fs: memFS(t, map[string]string{"/cfg/config.hcl": `provider "a" {}`})})

	file, diags := p.LoadConfigFile("/cfg/config.hcl")

	require.True(t, diags.HasErrors())
	assert.Nil(t, file)
	assert.Contains(t, diags.Error(), "Failed to read configuration")
	assert.NotContains(t, diags.Error(), "Configuration file not found")
}

// A directory that cannot be walked yields a diagnostic rather than a crash.
func TestParser_LoadConfigDir_UnreadableFilesReported(t *testing.T) {
	p := NewParser(unreadableFS{Fs: memFS(t, map[string]string{"/cfg/config.hcl": `provider "a" {}`})})

	var diags hcl.Diagnostics

	require.NotPanics(t, func() { _, diags = p.LoadConfigDir("/cfg") })

	assert.True(t, diags.HasErrors())
}
