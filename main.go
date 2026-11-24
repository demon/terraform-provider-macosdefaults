package main

import (
	"context"
	"flag"
	"log"

	"github.com/demon/terraform-provider-macosdefaults/internal/provider"
	"github.com/hashicorp/terraform-plugin-framework/providerserver"
)

var version = "dev"

func main() {
	var debug bool
	flag.BoolVar(&debug, "debug", false, "run with support for debuggers like delve")
	flag.Parse()

	err := providerserver.Serve(context.Background(), provider.New(version), providerserver.ServeOpts{
		Address: "registry.terraform.io/demon/macosdefaults",
		Debug:   debug,
	})
	if err != nil {
		log.Fatal(err)
	}
}
