// SPDX-License-Identifier: MPL-2.0

package client

import (
	"context"
	"encoding/json"
	"fmt"
)

// doc is the generic JSON:API single-resource envelope.
type doc[T any] struct {
	Data resourceObject[T] `json:"data"`
}

// listDoc is the generic JSON:API collection envelope, including the pagination
// links object HackerOne returns.
type listDoc[T any] struct {
	Data  []resourceObject[T] `json:"data"`
	Links struct {
		Next string `json:"next"`
	} `json:"links"`
}

// getAllPages fetches every page of a JSON:API collection starting at firstPath
// (relative to baseURL), following links.next until exhausted, and returns the
// concatenated data objects. This prevents silent truncation on large lists
// (e.g. program weaknesses span many pages). A bounded page cap guards against
// a pathological loop.
func getAllPages[T any](ctx context.Context, c *Client, firstPath string) ([]resourceObject[T], error) {
	const maxPages = 1000
	var all []resourceObject[T]
	path := firstPath
	for i := 0; i < maxPages; i++ {
		var page listDoc[T]
		if err := c.do(ctx, "GET", path, nil, &page); err != nil {
			return nil, err
		}
		all = append(all, page.Data...)
		if page.Links.Next == "" {
			return all, nil
		}
		// links.next is an absolute URL; strip the base so do() can rebuild it.
		next, ok := relativeToBase(page.Links.Next, c.baseURL)
		if !ok {
			return all, nil // unrecognized next link; stop rather than loop
		}
		path = next
	}
	return all, fmt.Errorf("pagination exceeded %d pages starting at %s", maxPages, firstPath)
}

type resourceObject[T any] struct {
	ID         string `json:"id,omitempty"`
	Type       string `json:"type"`
	Attributes T      `json:"attributes"`
	// Relationships is only used on requests that need it (asset->scope).
	Relationships json.RawMessage `json:"relationships,omitempty"`
}

// ---------------------------------------------------------------------------
// Program policy — PUT /programs/{id}/policy
// ---------------------------------------------------------------------------

// PolicyAttributes carries the program policy prose.
type PolicyAttributes struct {
	Policy string `json:"policy"`
}

// UpdatePolicy replaces a program's policy text. programID is the numeric
// program id (not the handle).
func (c *Client) UpdatePolicy(ctx context.Context, programID, policy string) (*PolicyAttributes, error) {
	req := doc[PolicyAttributes]{Data: resourceObject[PolicyAttributes]{
		Type:       "program-policy",
		Attributes: PolicyAttributes{Policy: policy},
	}}
	var resp doc[PolicyAttributes]
	err := c.do(ctx, "PUT", fmt.Sprintf("/programs/%s/policy", programID), req, &resp)
	if err != nil {
		return nil, err
	}
	return &resp.Data.Attributes, nil
}

// ProgramAttributes holds the program fields we surface.
type ProgramAttributes struct {
	Handle string `json:"handle"`
	Policy string `json:"policy"`
}

// programRelationships parses only the organization link off a program record.
type programRelationships struct {
	Organization struct {
		Data struct {
			ID string `json:"id"`
		} `json:"data"`
	} `json:"organization"`
}

// GetProgram fetches a program by its numeric ID. NOTE: the API's
// GET /programs/{id} accepts only the numeric ID; passing a handle returns
// HTTP 400. Use GetProgramByHandle to resolve a handle. The returned string is
// the organization ID parsed from the program's relationships (empty if
// absent).
func (c *Client) GetProgram(ctx context.Context, id string) (orgID string, attrs *ProgramAttributes, err error) {
	var resp struct {
		Data struct {
			ID            string               `json:"id"`
			Attributes    ProgramAttributes    `json:"attributes"`
			Relationships programRelationships `json:"relationships"`
		} `json:"data"`
	}
	if err := c.do(ctx, "GET", fmt.Sprintf("/programs/%s", id), nil, &resp); err != nil {
		return "", nil, err
	}
	a := resp.Data.Attributes
	return resp.Data.Relationships.Organization.Data.ID, &a, nil
}

// GetProgramByHandle resolves a program handle to its numeric program ID, its
// organization ID, and attributes. It first lists the caller's programs
// (/me/programs) to map handle -> numeric ID — the only reliable handle lookup,
// since GET /programs/{handle} is rejected with a 400 — then fetches the
// program by ID to obtain the organization relationship (which /me/programs
// does not include).
func (c *Client) GetProgramByHandle(ctx context.Context, handle string) (programID, orgID string, attrs *ProgramAttributes, err error) {
	var list struct {
		Data []resourceObject[ProgramAttributes] `json:"data"`
	}
	if err := c.do(ctx, "GET", "/me/programs", nil, &list); err != nil {
		return "", "", nil, err
	}
	for _, o := range list.Data {
		if o.Attributes.Handle == handle {
			org, a, err := c.GetProgram(ctx, o.ID)
			if err != nil {
				return "", "", nil, err
			}
			return o.ID, org, a, nil
		}
	}
	return "", "", nil, fmt.Errorf("no program found with handle %q among the API identity's programs", handle)
}

// ---------------------------------------------------------------------------
// Scope exclusions — /programs/{id}/scope_exclusions
// ---------------------------------------------------------------------------

// ScopeExclusionAttributes is an out-of-scope entry.
type ScopeExclusionAttributes struct {
	Category string `json:"category"`
	Details  string `json:"details,omitempty"`
}

func (c *Client) CreateScopeExclusion(ctx context.Context, programID string, attrs ScopeExclusionAttributes) (string, *ScopeExclusionAttributes, error) {
	req := doc[ScopeExclusionAttributes]{Data: resourceObject[ScopeExclusionAttributes]{
		Type: "scope-exclusion", Attributes: attrs,
	}}
	var resp doc[ScopeExclusionAttributes]
	err := c.do(ctx, "POST", fmt.Sprintf("/programs/%s/scope_exclusions", programID), req, &resp)
	if err != nil {
		return "", nil, err
	}
	return resp.Data.ID, &resp.Data.Attributes, nil
}

func (c *Client) UpdateScopeExclusion(ctx context.Context, programID, id string, attrs ScopeExclusionAttributes) (*ScopeExclusionAttributes, error) {
	req := doc[ScopeExclusionAttributes]{Data: resourceObject[ScopeExclusionAttributes]{
		Type: "scope-exclusion", Attributes: attrs,
	}}
	var resp doc[ScopeExclusionAttributes]
	err := c.do(ctx, "PUT", fmt.Sprintf("/programs/%s/scope_exclusions/%s", programID, id), req, &resp)
	if err != nil {
		return nil, err
	}
	return &resp.Data.Attributes, nil
}

func (c *Client) DeleteScopeExclusion(ctx context.Context, programID, id string) error {
	return c.do(ctx, "DELETE", fmt.Sprintf("/programs/%s/scope_exclusions/%s", programID, id), nil, nil)
}

// GetScopeExclusions lists all out-of-scope entries for a program. The API has
// no single-item GET, so Read filters this list by id.
func (c *Client) GetScopeExclusions(ctx context.Context, programID string) (map[string]ScopeExclusionAttributes, error) {
	items, err := c.ListScopeExclusions(ctx, programID)
	if err != nil {
		return nil, err
	}
	out := make(map[string]ScopeExclusionAttributes, len(items))
	for _, it := range items {
		out[it.ID] = it.ScopeExclusionAttributes
	}
	return out, nil
}

// ScopeExclusion is a scope exclusion with its ID, for list consumers.
type ScopeExclusion struct {
	ID string
	ScopeExclusionAttributes
}

// ListScopeExclusions returns every scope exclusion for a program, in API
// order, following pagination.
func (c *Client) ListScopeExclusions(ctx context.Context, programID string) ([]ScopeExclusion, error) {
	objs, err := getAllPages[ScopeExclusionAttributes](ctx, c, fmt.Sprintf("/programs/%s/scope_exclusions?page%%5Bsize%%5D=100", programID))
	if err != nil {
		return nil, err
	}
	out := make([]ScopeExclusion, 0, len(objs))
	for _, o := range objs {
		out = append(out, ScopeExclusion{ID: o.ID, ScopeExclusionAttributes: o.Attributes})
	}
	return out, nil
}

// ---------------------------------------------------------------------------
// Assets — /organizations/{org_id}/assets
// ---------------------------------------------------------------------------

// AssetAttributes describes an organization asset. Only fields we manage are
// modeled; the API accepts more.
type AssetAttributes struct {
	AssetType                  string `json:"asset_type,omitempty"`
	Identifier                 string `json:"identifier,omitempty"`
	Description                string `json:"description,omitempty"`
	MaxSeverity                string `json:"max_severity,omitempty"`
	ConfidentialityRequirement string `json:"confidentiality_requirement,omitempty"`
	IntegrityRequirement       string `json:"integrity_requirement,omitempty"`
	AvailabilityRequirement    string `json:"availability_requirement,omitempty"`
	Reference                  string `json:"reference,omitempty"`
}

func (c *Client) CreateAsset(ctx context.Context, orgID string, attrs AssetAttributes) (string, *AssetAttributes, error) {
	req := doc[AssetAttributes]{Data: resourceObject[AssetAttributes]{Type: "asset", Attributes: attrs}}
	var resp doc[AssetAttributes]
	err := c.do(ctx, "POST", fmt.Sprintf("/organizations/%s/assets", orgID), req, &resp)
	if err != nil {
		return "", nil, err
	}
	return resp.Data.ID, &resp.Data.Attributes, nil
}

func (c *Client) GetAsset(ctx context.Context, orgID, id string) (*AssetAttributes, error) {
	var resp doc[AssetAttributes]
	err := c.do(ctx, "GET", fmt.Sprintf("/organizations/%s/assets/%s", orgID, id), nil, &resp)
	if err != nil {
		return nil, err
	}
	return &resp.Data.Attributes, nil
}

// GetAssetByIdentifier finds an asset in an organization by its identifier
// (e.g. "example.com"), paging through the asset list. Returns the asset ID and
// attributes. There is no server-side lookup-by-identifier endpoint, so this
// scans; identifiers are unique within an org. Returns ("", nil, nil) if no
// asset matches.
func (c *Client) GetAssetByIdentifier(ctx context.Context, orgID, identifier string) (string, *AssetAttributes, error) {
	objs, err := getAllPages[AssetAttributes](ctx, c, fmt.Sprintf("/organizations/%s/assets?page%%5Bsize%%5D=100", orgID))
	if err != nil {
		return "", nil, err
	}
	for _, o := range objs {
		if o.Attributes.Identifier == identifier {
			attrs := o.Attributes
			return o.ID, &attrs, nil
		}
	}
	return "", nil, nil
}

func (c *Client) UpdateAsset(ctx context.Context, orgID, id string, attrs AssetAttributes) (*AssetAttributes, error) {
	// asset_type/identifier are immutable after creation; omit on update.
	attrs.AssetType = ""
	attrs.Identifier = ""
	req := doc[AssetAttributes]{Data: resourceObject[AssetAttributes]{Type: "asset", Attributes: attrs}}
	var resp doc[AssetAttributes]
	err := c.do(ctx, "PUT", fmt.Sprintf("/organizations/%s/assets/%s", orgID, id), req, &resp)
	if err != nil {
		return nil, err
	}
	return &resp.Data.Attributes, nil
}

// ArchiveAsset archives assets by id. HackerOne has no hard-delete for assets;
// archiving is the closest equivalent for a Terraform destroy.
func (c *Client) ArchiveAsset(ctx context.Context, orgID, id string) error {
	body := map[string]any{"data": []map[string]any{{"id": id, "type": "asset"}}}
	return c.do(ctx, "POST", fmt.Sprintf("/organizations/%s/assets/archive", orgID), body, nil)
}

// ---------------------------------------------------------------------------
// Asset scopes — /organizations/{org_id}/assets/{asset_id}/scopes
// This is how an asset is placed "in scope" for a program. The created object
// is a structured-scope; its id is what a program's structured_scopes list
// reports.
// ---------------------------------------------------------------------------

// AssetScopeAttributes controls eligibility and instructions for an asset that
// is in a program's scope. NOTE: the API is inconsistent between create and
// update on the notify field spelling ("on" vs "of"); both are populated on
// write and read here so whichever the API honors is set.
type AssetScopeAttributes struct {
	EligibleForSubmission     *bool  `json:"eligible_for_submission,omitempty"`
	EligibleForBounty         *bool  `json:"eligible_for_bounty,omitempty"`
	Instruction               string `json:"instruction,omitempty"`
	NotifySubscribersOnChange *bool  `json:"notify_subscribers_on_changes,omitempty"`
	NotifySubscribersOfChange *bool  `json:"notify_subscribers_of_changes,omitempty"`
}

// createAssetScopeBody adds the programs relationship required only on create.
type createAssetScopeBody struct {
	Data struct {
		Type          string               `json:"type"`
		Attributes    AssetScopeAttributes `json:"attributes"`
		Relationships struct {
			Programs struct {
				Data []struct {
					ID   json.Number `json:"id"`
					Type string      `json:"type"`
				} `json:"data"`
			} `json:"programs"`
		} `json:"relationships"`
	} `json:"data"`
}

// AddAssetToScope binds an asset to a program's scope. programID is the numeric
// program id. Returns the new structured-scope id.
func (c *Client) AddAssetToScope(ctx context.Context, orgID, assetID, programID string, attrs AssetScopeAttributes) (string, *AssetScopeAttributes, error) {
	var body createAssetScopeBody
	body.Data.Type = "structured-scope"
	body.Data.Attributes = attrs
	prog := struct {
		ID   json.Number `json:"id"`
		Type string      `json:"type"`
	}{ID: json.Number(programID), Type: "program"}
	body.Data.Relationships.Programs.Data = append(body.Data.Relationships.Programs.Data, prog)

	var resp doc[AssetScopeAttributes]
	err := c.do(ctx, "POST", fmt.Sprintf("/organizations/%s/assets/%s/scopes", orgID, assetID), body, &resp)
	if err != nil {
		return "", nil, err
	}
	return resp.Data.ID, &resp.Data.Attributes, nil
}

// UpdateAssetScope updates eligibility/instruction for an in-scope asset.
func (c *Client) UpdateAssetScope(ctx context.Context, orgID, assetID, scopeID string, attrs AssetScopeAttributes) (*AssetScopeAttributes, error) {
	req := doc[AssetScopeAttributes]{Data: resourceObject[AssetScopeAttributes]{
		Type: "structured-scope", Attributes: attrs,
	}}
	var resp doc[AssetScopeAttributes]
	err := c.do(ctx, "PUT", fmt.Sprintf("/organizations/%s/assets/%s/scopes/%s", orgID, assetID, scopeID), req, &resp)
	if err != nil {
		return nil, err
	}
	return &resp.Data.Attributes, nil
}

// ArchiveAssetScope removes an asset from a program's scope. The API archives
// by program reference on the asset's scopes/archive collection.
func (c *Client) ArchiveAssetScope(ctx context.Context, orgID, assetID, programID string) error {
	body := map[string]any{"data": []map[string]any{{"id": programID, "type": "program"}}}
	return c.do(ctx, "POST", fmt.Sprintf("/organizations/%s/assets/%s/scopes/archive", orgID, assetID), body, nil)
}

// StructuredScopeAttributes is the read view of an in-scope item, fetched via a
// program's structured_scopes list (the only documented read path). Archived
// scopes are excluded from the list entirely, so archival is detected by
// absence rather than a field here.
type StructuredScopeAttributes struct {
	AssetIdentifier       string `json:"asset_identifier"`
	AssetType             string `json:"asset_type"`
	EligibleForBounty     bool   `json:"eligible_for_bounty"`
	EligibleForSubmission bool   `json:"eligible_for_submission"`
	Instruction           string `json:"instruction"`
}

// GetStructuredScope reads a single structured scope by id from a program's
// structured_scopes list. Returns (nil, nil) if not present (e.g. archived).
func (c *Client) GetStructuredScope(ctx context.Context, programID, scopeID string) (*StructuredScopeAttributes, error) {
	var resp struct {
		Data []resourceObject[StructuredScopeAttributes] `json:"data"`
	}
	err := c.do(ctx, "GET", fmt.Sprintf("/programs/%s/structured_scopes", programID), nil, &resp)
	if err != nil {
		return nil, err
	}
	for _, o := range resp.Data {
		if o.ID == scopeID {
			attrs := o.Attributes
			return &attrs, nil
		}
	}
	return nil, nil
}

// ---------------------------------------------------------------------------
// Automations — /organizations/{org_id}/automations
// ---------------------------------------------------------------------------

// AutomationAttributes models a program automation/trigger.
type AutomationAttributes struct {
	Title              string          `json:"title,omitempty"`
	Code               string          `json:"code,omitempty"`
	TemplateIdentifier string          `json:"template_identifier,omitempty"`
	Config             json.RawMessage `json:"config,omitempty"`
	Events             []string        `json:"events,omitempty"`
	Enabled            *bool           `json:"enabled,omitempty"`
	RunOncePerReport   *bool           `json:"run_once_per_report,omitempty"`
}

func (c *Client) CreateAutomation(ctx context.Context, orgID string, attrs AutomationAttributes) (string, *AutomationAttributes, error) {
	req := doc[AutomationAttributes]{Data: resourceObject[AutomationAttributes]{Type: "automation", Attributes: attrs}}
	var resp doc[AutomationAttributes]
	err := c.do(ctx, "POST", fmt.Sprintf("/organizations/%s/automations", orgID), req, &resp)
	if err != nil {
		return "", nil, err
	}
	return resp.Data.ID, &resp.Data.Attributes, nil
}

func (c *Client) GetAutomation(ctx context.Context, orgID, id string) (*AutomationAttributes, error) {
	var resp doc[AutomationAttributes]
	err := c.do(ctx, "GET", fmt.Sprintf("/organizations/%s/automations/%s", orgID, id), nil, &resp)
	if err != nil {
		return nil, err
	}
	return &resp.Data.Attributes, nil
}

// UpdateAutomation uses PATCH per the API (note: not PUT). template_identifier
// is create-only and omitted here.
func (c *Client) UpdateAutomation(ctx context.Context, orgID, id string, attrs AutomationAttributes) (*AutomationAttributes, error) {
	attrs.TemplateIdentifier = ""
	req := doc[AutomationAttributes]{Data: resourceObject[AutomationAttributes]{Type: "automation", Attributes: attrs}}
	var resp doc[AutomationAttributes]
	err := c.do(ctx, "PATCH", fmt.Sprintf("/organizations/%s/automations/%s", orgID, id), req, &resp)
	if err != nil {
		return nil, err
	}
	return &resp.Data.Attributes, nil
}

// ---------------------------------------------------------------------------
// Weaknesses — /programs/{id}/weaknesses (read-only)
// ---------------------------------------------------------------------------

// WeaknessAttributes describes a weakness (CWE/CAPEC) in a program's catalog.
type WeaknessAttributes struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	ExternalID  string `json:"external_id"`
}

// Weakness is a weakness with its ID, for list consumers.
type Weakness struct {
	ID string
	WeaknessAttributes
}

// ListWeaknesses returns every weakness in a program's catalog, in API order,
// following pagination (this list commonly spans many pages).
func (c *Client) ListWeaknesses(ctx context.Context, programID string) ([]Weakness, error) {
	objs, err := getAllPages[WeaknessAttributes](ctx, c, fmt.Sprintf("/programs/%s/weaknesses?page%%5Bsize%%5D=100", programID))
	if err != nil {
		return nil, err
	}
	out := make([]Weakness, 0, len(objs))
	for _, o := range objs {
		out = append(out, Weakness{ID: o.ID, WeaknessAttributes: o.Attributes})
	}
	return out, nil
}
