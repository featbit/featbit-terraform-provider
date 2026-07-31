package main

import (
	"strings"
	"testing"

	"github.com/featbit/terraform-provider-featbit/tools/api-probe/internal/probe"
)

func TestPrintableIdentityNeverReturnsTheExactValue(t *testing.T) {
	t.Parallel()

	tests := []probe.ResourceIdentity{
		{ID: "11111111-1111-1111-1111-111111111111"},
		{Key: "tfp0-synthetic-exact-key"},
		{
			ID:  "22222222-2222-2222-2222-222222222222",
			Key: "tfp0-synthetic-exact-key",
		},
	}
	for _, identity := range tests {
		printed := printableIdentity(identity)
		if printed == identity.ID ||
			printed == identity.Key ||
			(identity.ID != "" && strings.Contains(printed, identity.ID)) ||
			(identity.Key != "" && strings.Contains(printed, identity.Key)) {
			t.Fatalf(
				"printable identity exposed an exact value: %q",
				printed,
			)
		}
	}
}
