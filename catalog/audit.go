package catalog

import "sort"

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

// AuditReport keeps source reconciliation separate from whole API coverage.
type AuditReport struct {
	Sources            []SourceAudit `json:"sources"`
	InventoryComplete  bool          `json:"inventory_complete"`
	CoverageRatio      *float64      `json:"coverage_ratio"`
	IncompleteFamilies []string      `json:"incomplete_families"`
}

// Audit groups canonical active operations by exact source URI and revision.
// It performs no I/O and does not infer aliases, versions or provider availability.
func (c Catalog) Audit() AuditReport {
	summary := c.Summary()
	out := AuditReport{Sources: []SourceAudit{}, InventoryComplete: summary.InventoryComplete, CoverageRatio: summary.CoverageRatio, IncompleteFamilies: summary.IncompleteFamilies}
	type sourceKey struct{ uri, revision string }
	grouped := map[sourceKey]*SourceAudit{}
	for _, r := range c.records {
		if r.Availability != "documented" {
			continue
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
	return out
}
