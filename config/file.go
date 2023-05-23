package config

import "github.com/hashicorp/hcl/v2"

type Provider struct {
	Name string   `hcl:"name,label"`
	HCL  hcl.Body `hcl:",remain"`
}

type File struct {
	Variables []*Var      `hcl:"variable,block"`
	Providers []*Provider `hcl:"provider,block"`
}
