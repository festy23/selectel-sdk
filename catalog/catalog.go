// Package catalog exposes embedded public API contracts without credentials or I/O.
package catalog

import (
	"embed"
	"encoding/json"
	"sort"
	"strings"

	"github.com/festy23/selectel-sdk/types"
)

//go:generate go run ../internal/cataloggen
//go:embed manifest.json service-registry.json sources/*.json contracts/*.json
var assets embed.FS

type Source struct {
	URI              string `json:"uri"`
	Revision         string `json:"revision"`
	OperationLocator string `json:"operation_locator"`
}
type Auth struct {
	Scheme   types.AuthMode `json:"scheme"`
	Audience string         `json:"audience"`
}
type Implementation struct {
	Status  string  `json:"status"`
	Binding *string `json:"binding"`
}
type Verification struct {
	Level                  string  `json:"level"`
	ContractRevision       *string `json:"contract_revision"`
	ImplementationRevision *string `json:"implementation_revision"`
	EvidenceRef            *string `json:"evidence_ref"`
}
type ParameterSupport struct {
	CoveredSchemaRef *string  `json:"covered_schema_ref"`
	ExcludedVariants []string `json:"excluded_variants"`
}
type Capability struct {
	OperationID               string            `json:"operation_id"`
	Service                   string            `json:"service"`
	APIGeneration             string            `json:"api_generation"`
	Resource                  string            `json:"resource"`
	Action                    string            `json:"action"`
	Effect                    string            `json:"effect"`
	Availability              string            `json:"availability"`
	Source                    Source            `json:"source"`
	ContractRevision          string            `json:"contract_revision"`
	ContractArtifactRevisions map[string]string `json:"contract_artifact_revisions,omitempty"`
	GoMethod                  *string           `json:"go_method"`
	InputType                 *string           `json:"input_type"`
	OutputType                *string           `json:"output_type"`
	InputSchemaRef            *string           `json:"input_schema_ref"`
	OutputSchemaRef           *string           `json:"output_schema_ref"`
	ScopeRequirements         []string          `json:"scope_requirements"`
	Auth                      Auth              `json:"auth"`
	EndpointRecipeRef         *string           `json:"endpoint_recipe_ref"`
	RequestBindingRef         *string           `json:"request_binding_ref"`
	ResponseBindingRef        *string           `json:"response_binding_ref"`
	ErrorBindingRef           *string           `json:"error_binding_ref"`
	ReplayPolicyRef           *string           `json:"replay_policy_ref"`
	PaginationContractRef     *string           `json:"pagination_contract_ref"`
	WaitGoals                 []string          `json:"wait_goals"`
	RecoveryContractRef       *string           `json:"recovery_contract_ref"`
	SensitiveFields           []string          `json:"sensitive_fields"`
	ParameterSupport          ParameterSupport  `json:"parameter_support"`
	Implementation            Implementation    `json:"implementation"`
	Verification              Verification      `json:"verification"`
	Limitations               []string          `json:"limitations"`
}

// DiscoverySource binds an offline factual extract to its owning public source.
type DiscoverySource struct {
	URI      string   `json:"uri"`
	Revision string   `json:"revision"`
	Locator  string   `json:"locator"`
	Role     string   `json:"role"`
	Facts    []string `json:"facts"`
}
type DiscoveryVersion struct {
	Component       string `json:"component"`
	Release         string `json:"release,omitempty"`
	APIVersion      string `json:"api_version,omitempty"`
	MaxMicroversion string `json:"max_microversion,omitempty"`
}

// DiscoveryEvidence never establishes callable operations or access rights.
type DiscoveryEvidence struct {
	EvidenceRef      string             `json:"evidence_ref"`
	Revision         string             `json:"revision"`
	Sources          []DiscoverySource  `json:"sources"`
	Versions         []DiscoveryVersion `json:"versions"`
	InventoryCount   *uint              `json:"inventory_count"`
	MissingContracts []string           `json:"missing_contracts"`
	FollowupIssues   []string           `json:"followup_issues"`
}
type Service struct {
	Discovery         *DiscoveryEvidence `json:"discovery,omitempty"`
	ID                string             `json:"id"`
	SourceURIs        []string           `json:"source_uris"`
	InventoryComplete bool               `json:"inventory_complete"`
	Availability      string             `json:"availability"`
}
type Filter struct{ Service, APIGeneration, Effect, Availability, Verification string }
type Catalog struct {
	records  []Capability
	services []Service
}

func Default() Catalog {
	var c Catalog
	raw, err := assets.ReadFile("manifest.json")
	if err != nil {
		panic("embedded catalog missing")
	}
	if json.Unmarshal(raw, &c.records) != nil {
		panic("embedded catalog invalid")
	}
	raw, err = assets.ReadFile("service-registry.json")
	if err != nil {
		panic("embedded service registry missing")
	}
	if json.Unmarshal(raw, &c.services) != nil {
		panic("embedded service registry invalid")
	}
	return c
}

func (c Catalog) List(f Filter) ([]Capability, error) {
	if !oneOf(f.Effect, "", "read", "mutation", "transfer") || !oneOf(f.Availability, "", "documented", "discovery_pending", "withdrawn") || !oneOf(f.Verification, "", "none", "offline", "wire", "live") {
		return nil, types.NewError(types.InvalidArgument)
	}
	out := make([]Capability, 0)
	for _, r := range c.records {
		if (f.Service != "" && f.Service != r.Service) || (f.APIGeneration != "" && f.APIGeneration != r.APIGeneration) || (f.Effect != "" && f.Effect != r.Effect) || (f.Availability != "" && f.Availability != r.Availability) || (f.Verification != "" && f.Verification != r.Verification.Level) {
			continue
		}
		out = append(out, clone(r))
	}
	sort.Slice(out, func(i, j int) bool { return out[i].OperationID < out[j].OperationID })
	return out, nil
}

func (c Catalog) Describe(id string) (Capability, error) {
	if id == "" {
		return Capability{}, types.NewError(types.InvalidArgument)
	}
	for _, r := range c.records {
		if r.OperationID == id {
			return clone(r), nil
		}
	}
	return Capability{}, types.NewError(types.CapabilityUnavailable)
}

func (c Catalog) Services() []Service {
	out := make([]Service, len(c.services))
	for i, s := range c.services {
		out[i] = s
		out[i].SourceURIs = append([]string(nil), s.SourceURIs...)
		if s.Discovery != nil {
			d := *s.Discovery
			d.Sources = append([]DiscoverySource(nil), d.Sources...)
			for j := range d.Sources {
				d.Sources[j].Facts = append([]string(nil), d.Sources[j].Facts...)
			}
			d.Versions = append([]DiscoveryVersion(nil), d.Versions...)
			d.MissingContracts = append([]string(nil), d.MissingContracts...)
			d.FollowupIssues = append([]string(nil), d.FollowupIssues...)
			if d.InventoryCount != nil {
				count := *d.InventoryCount
				d.InventoryCount = &count
			}
			out[i].Discovery = &d
		}
	}
	return out
}
func (c Catalog) HasService(id string) bool {
	for _, s := range c.services {
		if s.ID == id {
			return true
		}
	}
	return false
}

// Artifact returns a copy of the source snapshot referred to by a contract.
func (c Catalog) Artifact(ref string) ([]byte, error) {
	file, _, _ := strings.Cut(ref, "#")
	if !strings.HasPrefix(file, "sources/") && !strings.HasPrefix(file, "contracts/") {
		return nil, types.NewError(types.InvalidArgument)
	}
	b, err := assets.ReadFile(file)
	if err != nil {
		return nil, types.NewError(types.CapabilityUnavailable)
	}
	return append([]byte(nil), b...), nil
}

type Summary struct {
	Documented         int      `json:"documented"`
	Partial            int      `json:"partial"`
	Implemented        int      `json:"implemented"`
	Offline            int      `json:"offline"`
	Wire               int      `json:"wire"`
	Live               int      `json:"live"`
	InventoryComplete  bool     `json:"inventory_complete"`
	CoverageRatio      *float64 `json:"coverage_ratio"`
	IncompleteFamilies []string `json:"incomplete_families"`
	UnimplementedIDs   []string `json:"unimplemented_ids"`
}

func (c Catalog) Summary() Summary {
	s := Summary{InventoryComplete: true, IncompleteFamilies: []string{}, UnimplementedIDs: []string{}}
	for _, family := range c.services {
		if family.Availability == "documented" && !family.InventoryComplete {
			s.InventoryComplete = false
			s.IncompleteFamilies = append(s.IncompleteFamilies, family.ID)
		}
	}
	for _, r := range c.records {
		if r.Availability != "documented" {
			continue
		}
		s.Documented++
		switch r.Implementation.Status {
		case "implemented":
			s.Implemented++
		case "partial":
			s.Partial++
			s.UnimplementedIDs = append(s.UnimplementedIDs, r.OperationID)
		default:
			s.UnimplementedIDs = append(s.UnimplementedIDs, r.OperationID)
		}
		switch r.Verification.Level {
		case "offline":
			s.Offline++
		case "wire":
			s.Wire++
		case "live":
			s.Live++
		}
	}
	if s.InventoryComplete && s.Documented > 0 {
		r := float64(s.Implemented) / float64(s.Documented)
		s.CoverageRatio = &r
	}
	sort.Strings(s.IncompleteFamilies)
	sort.Strings(s.UnimplementedIDs)
	return s
}
func clone(r Capability) Capability {
	b, _ := json.Marshal(r)
	var copy Capability
	_ = json.Unmarshal(b, &copy)
	return copy
}

func (c Catalog) MarshalJSON() ([]byte, error) {
	operations, err := c.List(Filter{})
	if err != nil {
		return nil, err
	}
	return json.Marshal(struct {
		SchemaVersion int          `json:"schema_version"`
		Services      []Service    `json:"services"`
		Operations    []Capability `json:"operations"`
		Summary       Summary      `json:"summary"`
	}{1, c.Services(), operations, c.Summary()})
}
func oneOf(value string, allowed ...string) bool {
	for _, v := range allowed {
		if value == v {
			return true
		}
	}
	return false
}
