terraform {
  required_providers {
    hackerone = {
      source = "scottbrown/hackerone"
    }
  }
}

# Credentials come from HACKERONE_API_IDENTIFIER / HACKERONE_API_TOKEN env vars.
provider "hackerone" {}

# Resolve a program handle to its numeric program ID and organization ID. Every
# other resource references this data source rather than hardcoding IDs.
data "hackerone_program" "example" {
  handle = "example-program"
}

# Manage the program policy from a versioned Markdown file.
resource "hackerone_policy" "main" {
  program_id = data.hackerone_program.example.id
  policy     = file("${path.module}/policy.md")
}

# Out-of-scope entries.
resource "hackerone_scope_exclusion" "internal_admin" {
  program_id = data.hackerone_program.example.id
  category   = "url"
  details    = "https://admin.internal.example.com"
}

# Organization asset inventory. organization_id comes from the data source.
resource "hackerone_asset" "api" {
  organization_id = data.hackerone_program.example.organization_id
  asset_type      = "url"
  identifier      = "https://api.example.com"
  description     = "Public API gateway"
  max_severity    = "critical"
}

# Put that asset in the program's scope, eligible for bounty. Reference the
# asset's computed asset_id directly (no need to split the composite id).
resource "hackerone_asset_scope" "api_in_scope" {
  organization_id         = data.hackerone_program.example.organization_id
  asset_id                = hackerone_asset.api.asset_id
  program_id              = data.hackerone_program.example.id
  eligible_for_submission = true
  eligible_for_bounty     = true
  instruction             = "Focus on authz and IDOR."
}

# A triage automation (Enterprise-tier feature).
resource "hackerone_automation" "auto_ack" {
  organization_id = data.hackerone_program.example.organization_id
  title           = "Auto-acknowledge new reports"
  code            = file("${path.module}/auto_ack.js")
  enabled         = true
}
