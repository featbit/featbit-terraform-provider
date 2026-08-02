// Copyright 2026 FeatBit
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"fmt"

	"github.com/featbit/terraform-provider-featbit/internal/client"
	"github.com/hashicorp/terraform-plugin-framework/diag"
)

func clientFromProviderData(
	providerData any,
	component string,
	diagnostics *diag.Diagnostics,
) *client.Client {
	if providerData == nil {
		return nil
	}
	apiClient, ok := providerData.(*client.Client)
	if !ok {
		diagnostics.AddError(
			"Unexpected "+component+" Configure Type",
			fmt.Sprintf(
				"Expected the configured FeatBit API client, received %T. Please report this provider error.",
				providerData,
			),
		)
		return nil
	}
	return apiClient
}

func requireAPIClient(
	apiClient *client.Client,
	action string,
	diagnostics *diag.Diagnostics,
) bool {
	if apiClient != nil {
		return true
	}
	diagnostics.AddError(
		"FeatBit API Client Is Not Configured",
		"Configure the FeatBit provider before "+action+".",
	)
	return false
}
