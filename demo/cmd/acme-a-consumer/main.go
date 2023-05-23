package main

import (
	"github.com/hyperscale/fabric/demo/cmd/acme-a-consumer/app"
)

func main() {
	a, err := app.New()
	if err != nil {
		panic(err)
	}

	if err := a.Start(); err != nil {
		panic(err)
	}

	defer func() {
		if err := a.Stop(); err != nil {
			panic(err)
		}
	}()
}
