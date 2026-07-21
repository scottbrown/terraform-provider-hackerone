// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"os"
	"regexp"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

// regexpNumeric matches a non-empty string of digits (a HackerOne numeric ID).
var regexpNumeric = regexp.MustCompile(`^\d+$`)

// TestAccProgramDataSource is a minimal acceptance test for the program data
// source. It is gated behind TF_ACC and requires HACKERONE_API_IDENTIFIER /
// HACKERONE_API_TOKEN plus a HACKERONE_TEST_PROGRAM_HANDLE to look up. It
// verifies the handle resolves to a non-empty numeric program ID and
// organization ID.
func TestAccProgramDataSource(t *testing.T) {
	handle := os.Getenv("HACKERONE_TEST_PROGRAM_HANDLE")
	if handle == "" {
		t.Skip("HACKERONE_TEST_PROGRAM_HANDLE must be set for this acceptance test")
	}

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: `
data "hackerone_program" "test" {
  handle = "` + handle + `"
}
`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestMatchResourceAttr("data.hackerone_program.test", "id", regexpNumeric),
					resource.TestMatchResourceAttr("data.hackerone_program.test", "organization_id", regexpNumeric),
				),
			},
		},
	})
}
