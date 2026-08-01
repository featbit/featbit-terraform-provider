// Copyright IBM Corp. 2021, 2025
// Copyright 2026 FeatBit
// SPDX-License-Identifier: MPL-2.0

package main

import (
	"context"
	"flag"
	"log"

	"github.com/featbit/terraform-provider-featbit/internal/provider"
	"github.com/hashicorp/terraform-plugin-framework/providerserver"
)

const providerAddress = "registry.terraform.io/featbit/featbit"

// version is set at build time for release binaries.
var version = "dev"

func main() {
	var debug bool

	flag.BoolVar(&debug, "debug", false, "run the provider with debugger support")
	flag.Parse()

	err := providerserver.Serve(
		context.Background(),
		provider.New(version),
		providerserver.ServeOpts{
			Address: providerAddress,
			Debug:   debug,
		},
	)
	if err != nil {
		log.Fatalf("provider server failed: %s", err)
	}
}
