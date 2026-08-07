package config

import (
	"testing"

	"github.com/spf13/afero"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zclconf/go-cty/cty"
)

func TestNewParser_NilFilesystemUsesDisk(t *testing.T) {
	p := NewParser(nil)

	require.NotNil(t, p)
	assert.NotNil(t, p.Context())

	// The real filesystem is reachable: the repository's own testdata is there.
	assert.True(t, p.IsConfigDir("./testdata/valid"))
}

func TestParser_Context_HasVarAndEnv(t *testing.T) {
	t.Setenv("FABRIC_TEST_CTX", "from-env")

	ctx := NewParser(afero.NewMemMapFs()).Context()

	require.Contains(t, ctx.Variables, "var")
	require.Contains(t, ctx.Variables, "env")

	env := ctx.Variables["env"].AsValueMap()
	assert.Equal(t, cty.StringVal("from-env"), env["FABRIC_TEST_CTX"])
}

// The eval context snapshots the environment when the parser is built, so a
// variable exported afterwards is invisible. Worth pinning: it is the reason a
// test that sets an environment variable must do so before NewParser.
func TestParser_Context_EnvIsSnapshotAtConstruction(t *testing.T) {
	p := NewParser(afero.NewMemMapFs())

	t.Setenv("FABRIC_TEST_LATE", "too-late")

	_, ok := p.Context().Variables["env"].AsValueMap()["FABRIC_TEST_LATE"]
	assert.False(t, ok, "the environment is snapshot at construction")
}

func TestParser_Parse(t *testing.T) {
	p := NewParser(afero.NewMemMapFs())

	file, diags := p.Parse("test.hcl", []byte(`
variable "region" {
  value = "eu-west-1"
}

provider "mysql" {
  host = "localhost"
  port = 3306
}
`))

	require.False(t, diags.HasErrors(), diags.Error())
	require.NotNil(t, file)

	require.Len(t, file.Variables, 1)
	assert.Equal(t, "region", file.Variables[0].Name)
	assert.Equal(t, cty.StringVal("eu-west-1"), file.Variables[0].Value)

	require.Len(t, file.Providers, 1)
	assert.Equal(t, "mysql", file.Providers[0].Name)
	assert.NotNil(t, file.Providers[0].HCL)
}

func TestParser_Parse_SyntaxError(t *testing.T) {
	p := NewParser(afero.NewMemMapFs())

	file, diags := p.Parse("broken.hcl", []byte(`provider "mysql" {`))

	assert.True(t, diags.HasErrors())
	assert.Nil(t, file)
}

func TestParser_Parse_UnknownTopLevelBlock(t *testing.T) {
	p := NewParser(afero.NewMemMapFs())

	_, diags := p.Parse("test.hcl", []byte(`
unknown "thing" {
  a = 1
}
`))

	require.True(t, diags.HasErrors())
	assert.Contains(t, diags.Error(), "Unsupported block type")
}

// A variable declared in one file must be visible to the next one parsed by the
// same parser: that is how a variables file feeds the provider blocks.
func TestParser_Parse_VariablesAccumulateAcrossFiles(t *testing.T) {
	p := NewParser(afero.NewMemMapFs())

	_, diags := p.Parse("vars.hcl", []byte(`
variable "port" {
  value = 5432
}
`))
	require.False(t, diags.HasErrors(), diags.Error())

	file, diags := p.Parse("use.hcl", []byte(`
provider "pg" {
  port = var.port
}
`))
	require.False(t, diags.HasErrors(), diags.Error())

	var target struct {
		Port int `hcl:"port"`
	}

	require.False(t, decode(t, p, file.Providers[0], &target).HasErrors())
	assert.Equal(t, 5432, target.Port)
}

func TestParser_Parse_EnvInterpolation(t *testing.T) {
	t.Setenv("FABRIC_TEST_HOST", "db.internal")

	p := NewParser(afero.NewMemMapFs())

	file, diags := p.Parse("test.hcl", []byte(`
provider "mysql" {
  host = env.FABRIC_TEST_HOST
}
`))
	require.False(t, diags.HasErrors(), diags.Error())

	var target struct {
		Host string `hcl:"host"`
	}

	require.False(t, decode(t, p, file.Providers[0], &target).HasErrors())
	assert.Equal(t, "db.internal", target.Host)
}

func TestParser_Parse_StdlibFunctions(t *testing.T) {
	p := NewParser(afero.NewMemMapFs())

	file, diags := p.Parse("test.hcl", []byte(`
provider "p" {
  upper    = upper("abc")
  joined   = join("-", ["a", "b"])
  trimmed  = trimspace("  x  ")
  computed = max(3, 7, 5)
}
`))
	require.False(t, diags.HasErrors(), diags.Error())

	var target struct {
		Upper    string `hcl:"upper"`
		Joined   string `hcl:"joined"`
		Trimmed  string `hcl:"trimmed"`
		Computed int    `hcl:"computed"`
	}

	require.False(t, decode(t, p, file.Providers[0], &target).HasErrors())
	assert.Equal(t, "ABC", target.Upper)
	assert.Equal(t, "a-b", target.Joined)
	assert.Equal(t, "x", target.Trimmed)
	assert.Equal(t, 7, target.Computed)
}

// The custom `json` function is the only non-stdlib function in the context.
func TestParser_Parse_JSONFunction(t *testing.T) {
	p := NewParser(afero.NewMemMapFs())

	file, diags := p.Parse("test.hcl", []byte(`
provider "p" {
  payload = json({ a = 1, b = "two" })
}
`))
	require.False(t, diags.HasErrors(), diags.Error())

	var target struct {
		Payload string `hcl:"payload"`
	}

	require.False(t, decode(t, p, file.Providers[0], &target).HasErrors())
	assert.JSONEq(t, `{"a":1,"b":"two"}`, target.Payload)
}

// A provider body is captured as `hcl:",remain"` and stays unevaluated until it
// is decoded, so an undefined variable inside one is NOT reported by Parse. It
// surfaces later, when fabric.Configuration.ParseProvider decodes the block.
//
// This is why a typo in a provider block cannot be caught by loading the
// configuration alone, and why ParseProvider distinguishes an absent block from
// an invalid one.
func TestParser_Parse_UndefinedVariableSurfacesAtDecodeNotParse(t *testing.T) {
	p := NewParser(afero.NewMemMapFs())

	file, diags := p.Parse("test.hcl", []byte(`
provider "p" {
  value = var.nope
}
`))

	require.False(t, diags.HasErrors(), "Parse does not evaluate provider bodies")
	require.Len(t, file.Providers, 1)

	var target struct {
		Value string `hcl:"value"`
	}

	decodeDiags := decode(t, p, file.Providers[0], &target)

	require.True(t, decodeDiags.HasErrors(), "decoding must reject the undefined variable")
	assert.Contains(t, decodeDiags.Error(), "Unsupported attribute")
}

// The same holds for a top-level variable block, which Parse *does* evaluate.
func TestParser_Parse_UndefinedVariableInVariableBlock(t *testing.T) {
	p := NewParser(afero.NewMemMapFs())

	_, diags := p.Parse("test.hcl", []byte(`
variable "a" {
  value = var.nope
}
`))

	assert.True(t, diags.HasErrors())
}

func TestParser_Parse_Empty(t *testing.T) {
	p := NewParser(afero.NewMemMapFs())

	file, diags := p.Parse("empty.hcl", []byte(``))

	require.False(t, diags.HasErrors(), diags.Error())
	require.NotNil(t, file)
	assert.Empty(t, file.Providers)
	assert.Empty(t, file.Variables)
}
