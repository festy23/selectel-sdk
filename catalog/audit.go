package catalog

import (
	"encoding/json"
	"sort"
	"strconv"
	"strings"
)

// PartialBinding retains the exact variant exclusions of a callable subset.
type PartialBinding struct {
	OperationID      string   `json:"operation_id"`
	GoMethod         string   `json:"go_method"`
	CoveredSchemaRef string   `json:"covered_schema_ref"`
	ExcludedVariants []string `json:"excluded_variants"`
}

// SourceAudit measures only the pinned operation subset, not a complete provider inventory.
type SourceAudit struct {
	URI              string           `json:"uri"`
	Revision         string           `json:"revision"`
	Documented       int              `json:"documented"`
	Implemented      int              `json:"implemented"`
	Partial          int              `json:"partial"`
	UnimplementedIDs []string         `json:"unimplemented_ids"`
	PartialBindings  []PartialBinding `json:"partial_bindings"`
}

// BindingCheck validates declaration presence and local schema references only.
// It does not prove that a declaration is reachable from a scoped Client.
type BindingCheck struct {
	OperationID    string   `json:"operation_id"`
	GoMethod       string   `json:"go_method"`
	MethodFile     string   `json:"method_file"`
	MethodRevision string   `json:"method_revision"`
	Issues         []string `json:"issues"`
}

// AuditReport keeps source reconciliation separate from whole API coverage.
type AuditReport struct {
	BindingChecks      []BindingCheck `json:"binding_checks"`
	Sources            []SourceAudit  `json:"sources"`
	InventoryComplete  bool           `json:"inventory_complete"`
	CoverageRatio      *float64       `json:"coverage_ratio"`
	IncompleteFamilies []string       `json:"incomplete_families"`
}

// Audit groups canonical active operations by exact source URI and revision.
// It performs no I/O and does not infer aliases, versions or provider availability.
func (c Catalog) Audit() AuditReport {
	summary := c.Summary()
	out := AuditReport{Sources: []SourceAudit{}, InventoryComplete: summary.InventoryComplete, CoverageRatio: summary.CoverageRatio, IncompleteFamilies: summary.IncompleteFamilies}
	type sourceKey struct{ uri, revision string }
	grouped := map[sourceKey]*SourceAudit{}
	var methods map[string]struct {
		File     string `json:"file"`
		Revision string `json:"revision"`
	}
	raw, _ := assets.ReadFile("contracts/public-methods.json")
	_ = json.Unmarshal(raw, &methods)
	out.BindingChecks = []BindingCheck{}
	for _, r := range c.records {
		if r.Availability != "documented" {
			continue
		}
		if r.Implementation.Status == "implemented" || r.Implementation.Status == "partial" {
			check := BindingCheck{OperationID: r.OperationID, Issues: []string{}}
			if r.GoMethod != nil {
				check.GoMethod = *r.GoMethod
			}
			method, ok := methods[check.GoMethod]
			if !ok {
				check.Issues = append(check.Issues, "public_method_declaration_missing")
			} else {
				check.MethodFile, check.MethodRevision = method.File, method.Revision
			}
			refs := map[string]*string{"input_schema": r.InputSchemaRef, "output_schema": r.OutputSchemaRef, "endpoint": r.EndpointRecipeRef, "request": r.RequestBindingRef, "response": r.ResponseBindingRef, "errors": r.ErrorBindingRef, "replay": r.ReplayPolicyRef, "covered_schema": r.ParameterSupport.CoveredSchemaRef}
			for name, ref := range refs {
				if ref == nil || !c.validAuditRef(*ref) {
					check.Issues = append(check.Issues, name+"_reference_missing")
				}
			}
			sort.Strings(check.Issues)
			out.BindingChecks = append(out.BindingChecks, check)
		}
		key := sourceKey{r.Source.URI, r.Source.Revision}
		row := grouped[key]
		if row == nil {
			row = &SourceAudit{URI: key.uri, Revision: key.revision, UnimplementedIDs: []string{}, PartialBindings: []PartialBinding{}}
			grouped[key] = row
		}
		row.Documented++
		switch r.Implementation.Status {
		case "implemented":
			row.Implemented++
		case "partial":
			row.Partial++
			gap := PartialBinding{OperationID: r.OperationID, ExcludedVariants: append([]string{}, r.ParameterSupport.ExcludedVariants...)}
			if r.GoMethod != nil {
				gap.GoMethod = *r.GoMethod
			}
			if r.ParameterSupport.CoveredSchemaRef != nil {
				gap.CoveredSchemaRef = *r.ParameterSupport.CoveredSchemaRef
			}
			row.PartialBindings = append(row.PartialBindings, gap)
		default:
			row.UnimplementedIDs = append(row.UnimplementedIDs, r.OperationID)
		}
	}
	for _, row := range grouped {
		sort.Strings(row.UnimplementedIDs)
		sort.Slice(row.PartialBindings, func(i, j int) bool { return row.PartialBindings[i].OperationID < row.PartialBindings[j].OperationID })
		out.Sources = append(out.Sources, *row)
	}
	sort.Slice(out.Sources, func(i, j int) bool {
		a, b := out.Sources[i], out.Sources[j]
		if a.URI != b.URI {
			return a.URI < b.URI
		}
		return a.Revision < b.Revision
	})
	sort.Slice(out.BindingChecks, func(i, j int) bool { return out.BindingChecks[i].OperationID < out.BindingChecks[j].OperationID })
	return out
}

// validAuditRef follows JSON Pointer tokens in a shipped artifact, without I/O.
func (c Catalog) validAuditRef(ref string) bool {
	raw, err := c.Artifact(ref)
	if err != nil {
		return false
	}
	_, fragment, has := strings.Cut(ref, "#")
	if !has || fragment == "" {
		return true
	}
	if !strings.HasPrefix(fragment, "/") {
		return false
	}
	var node any
	if json.Unmarshal(raw, &node) != nil {
		return false
	}
	for _, part := range strings.Split(fragment[1:], "/") {
		// RFC6901 permits only ~0 and ~1 escapes.
		for i := 0; i < len(part); i++ {
			if part[i] == '~' {
				if i+1 >= len(part) || (part[i+1] != '0' && part[i+1] != '1') {
					return false
				}
				i++
			}
		}
		part = strings.ReplaceAll(strings.ReplaceAll(part, "~1", "/"), "~0", "~")
		switch value := node.(type) {
		case map[string]any:
			var ok bool
			node, ok = value[part]
			if !ok {
				return false
			}
		case []any:
			idx, err := strconv.Atoi(part)
			if err != nil || idx < 0 || idx >= len(value) || strconv.Itoa(idx) != part {
				return false
			}
			node = value[idx]
		default:
			return false
		}
	}
	return true
}
