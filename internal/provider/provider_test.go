// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"os"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/providerserver"
	"github.com/hashicorp/terraform-plugin-go/tfprotov6"
)

// testAccProtoV6ProviderFactories instantiates the provider during acceptance
// testing. The factory is called for each Terraform CLI command to create a
// provider server that the CLI connects to.
var testAccProtoV6ProviderFactories = map[string]func() (tfprotov6.ProviderServer, error){
	"hackerone": providerserver.NewProtocol6WithError(New("test")()),
}

// testAccPreCheck verifies the credentials required for acceptance tests are
// present before any test case runs. Acceptance tests talk to a real HackerOne
// account and are gated behind TF_ACC.
func testAccPreCheck(t *testing.T) {
	for _, v := range []string{"HACKERONE_API_IDENTIFIER", "HACKERONE_API_TOKEN"} {
		if os.Getenv(v) == "" {
			t.Fatalf("%s must be set for acceptance tests", v)
		}
	}
}
