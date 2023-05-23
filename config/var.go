package config

import (
	"github.com/zclconf/go-cty/cty"
)

// Var struct
type Var struct {
	Name  string    `hcl:"name,label"`
	Value cty.Value `hcl:"value,attr"`
}
