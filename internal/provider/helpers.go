// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"github.com/scottbrown/terraform-provider-hackerone/internal/client"
)

// clientFromProviderData extracts the shared *client.Client set in Configure,
// returning nil if it is absent (e.g. during early graph walks before the
// provider is configured).
func clientFromProviderData(pd any) (*client.Client, bool) {
	if pd == nil {
		return nil, false
	}
	c, ok := pd.(*client.Client)
	return c, ok
}
