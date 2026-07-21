# terraform-provider-hackerone

A Terraform provider for managing [HackerOne](https://www.hackerone.com/) bug
bounty **program configuration as code**, built on the
[HackerOne v1 REST API](https://api.hackerone.com/customer-resources/).

The goal: stop clickops-ing program policy, scope, and triage automation in the
H1 console and put it under version control + review instead.

> **Status:** proof of concept, partially validated against a live HackerOne
> account via `terraform import` (clean "N to import, 0 to change" plans).
>
> | Resource / data source | Live status |
> |---|---|
> | `hackerone_asset` | ✅ import-verified (0 drift) |
> | `hackerone_asset_scope` | ✅ import-verified (0 drift) |
> | `hackerone_policy` | ✅ import-verified (0 drift) |
> | `data.hackerone_program` | ✅ read-verified |
> | `hackerone_scope_exclusion` | ✅ import-verified (0 drift) |
> | `hackerone_automation` | ⚠️ untestable on non-Enterprise orgs — the Automations API is an **Enterprise-tier-only** feature and returns `404` otherwise (see below) |

## What it manages

| Resource / data source | H1 API | Notes |
|---|---|---|
| `hackerone_policy` | `PUT /programs/{id}/policy` | Program policy prose (Markdown). Singleton per program. |
| `hackerone_scope_exclusion` | `/programs/{id}/scope_exclusions` | Out-of-scope entries. Full CRUD. |
| `hackerone_asset` | `/organizations/{org_id}/assets` | Org asset inventory. Destroy = **archive** (no hard delete in the API). |
| `hackerone_asset_scope` | `POST /organizations/{org_id}/assets/{asset_id}/scopes` | Binds an asset into a program's scope ("in scope"). Read via the program's `structured_scopes`; destroy = archive from scope. |
| `hackerone_automation` | `/organizations/{org_id}/automations` | Triage automations. Update uses `PATCH`; destroy = **disable** (no delete endpoint). |
| `data.hackerone_program` | `GET /programs/{id}` | Resolves a program handle → numeric program ID **and** organization ID. Reference these instead of hardcoding IDs. |
| `data.hackerone_asset` | `GET /organizations/{org_id}/assets` | Looks up an existing (possibly unmanaged) asset by `identifier`, exposing its bare `asset_id` for `hackerone_asset_scope`. |
| `data.hackerone_scope_exclusions` | `GET /programs/{id}/scope_exclusions` | Lists all scope exclusions for a program (fully paginated). |
| `data.hackerone_weaknesses` | `GET /programs/{id}/weaknesses` | Lists the program's weakness/CWE catalog (fully paginated — commonly 1000+ entries). |

## Authentication

HTTP Basic auth with an API **username + token** (the H1 "API Token" settings
page). Prefer environment variables so the token stays out of Terraform state:

```bash
export HACKERONE_API_USERNAME="your-api-identity"
export HACKERONE_API_TOKEN="your-api-token"
```

Most write operations require the **"Team Management"** permission on the API
identity; without it the API returns `403`.

> **Recommendation:** create a dedicated service/API identity for CI rather than
> using a personal token. HackerOne personal tokens are per-user and
> **regenerating one revokes the prior token**, which will silently break
> automation, and every change is attributed to that human.

## Design notes & sharp edges

These reflect real constraints of the H1 public REST API, not choices:

- **Program scope vs. org scope are different API roots.** Policy and scope
  exclusions live under `/programs/{id}/...`; assets and automations live under
  `/organizations/{org_id}/...`. You need both a program ID and an org ID.
- **"In scope" is managed via the Assets API, not a program endpoint.**
  `GET /programs/{id}/structured_scopes` is read-only. Adding an asset to a
  program's scope is `POST /organizations/{org_id}/assets/{asset_id}/scopes`
  with a `programs` relationship — this is what `hackerone_asset_scope` wraps.
  Its Read side filters the program's `structured_scopes` list by scope ID
  since there is no single-item GET, and create vs. update disagree on the
  `notify_subscribers_on/of_changes` field spelling (the client sends both).
- **No hard deletes for assets or automations.** Destroy archives the asset and
  disables the automation, respectively, and emits a warning where relevant.
- **`hackerone_policy` delete is a no-op** — there is no API to clear a policy.
- **Rate limits / pagination are undocumented** in the public reference; the
  client implements bounded exponential backoff honoring `Retry-After`.
- **Bounty tables are read-only** in the public API and cannot be managed here.
- **Automations are Enterprise-tier only.** On non-Enterprise organizations the
  entire `/organizations/{org_id}/automations` tree returns `404` (not `403`),
  so `hackerone_automation` is unusable there. Confirmed by triangulation:
  every other org endpoint (`assets`, `asset_tags`, `members`) returns `200` for
  the same identity while `automations` 404s.
- **`GET /programs/{id}` requires the numeric ID, not the handle** (a handle
  returns `400`). `data.hackerone_program` resolves handles by listing
  `/me/programs` and matching, rather than hitting the program path directly.

## Building

```bash
go build ./...
go vet ./...
go install        # builds the provider into $GOBIN
```

For local testing, add a `~/.terraformrc` dev override pointing
`registry.terraform.io/scottbrown/hackerone` at your `$GOBIN`:

```hcl
provider_installation {
  dev_overrides {
    "scottbrown/hackerone" = "/path/to/your/GOBIN"
  }
  direct {}
}
```

Documentation is generated with `tfplugindocs` (`make generate`), and releases
are cut with GoReleaser via the GitHub Actions workflow (this repo is based on
the HashiCorp terraform-provider scaffolding template).

## Roadmap

- [ ] Acceptance tests against a sandbox program (`TF_ACC`)
- [ ] Asset tags / tag categories
- [ ] Publish tagged release to the Terraform Registry
