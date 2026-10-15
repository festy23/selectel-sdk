// Command cataloggen builds the API inventory from pinned public source assets.
// It performs no network requests and never observes credentials or cloud state.
package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

type family struct {
	Service, Generation string
	Mode                string
	Complete            bool
}

var families = map[string]family{
	"tickets":                      {"tickets", "v1", "static_token", false},
	"tickets-attachments":          {"tickets", "attachments_v1", "static_token", false},
	"balance":                      {"billing", "v3", "iam_account", false},
	"balance-statistics":           {"billing", "v1", "iam_account", false},
	"cloud-projects-and-resources": {"projects", "v2", "iam_account", true},
	"craas":                        {"registry", "v2", "iam_project", true},
	"managed-databases":            {"databases", "v1", "iam_project", false},
	"managed-kubernetes":           {"mks", "v2", "iam_project", false},
	"secrets-manager":              {"secrets", "v1", "iam_project", true},
	"audit-logs":                   {"audit", "v1", "iam_account", true},
	"compute-nova":                 {"compute", "v2_1", "iam_project", false},
	"cloudwatch-logs":              {"logs", "cloudwatch", "s3_sigv4", false},
	"prometheus-metrics":           {"metrics", "prometheus", "basic", false},
}
var overrides = map[string]string{
	"tickets-attachments GET /public/files/uuid/{file_uuid}/download": "tickets.attachments_v1.files.download",
	"tickets POST /tickets":                                            "tickets.v1.tickets.create",
	"tickets GET /tickets":                                             "tickets.v1.tickets.list",
	"tickets GET /tickets/{ticket_number}":                             "tickets.v1.tickets.get",
	"tickets PATCH /tickets/{ticket_number}":                           "tickets.v1.tickets.update",
	"tickets POST /tickets/{ticket_number}/close":                      "tickets.v1.tickets.close",
	"tickets POST /tickets/{ticket_number}/reopen":                     "tickets.v1.tickets.reopen",
	"tickets POST /tickets/{ticket_number}/comments":                   "tickets.v1.comments.create",
	"tickets GET /tickets/{ticket_number}/comments":                    "tickets.v1.comments.list",
	"tickets GET /comments/{comment_id}":                               "tickets.v1.comments.get",
	"tickets PATCH /tickets/{ticket_number}/vote":                      "tickets.v1.tickets.vote",
	"tickets POST /comments/{comment_id}/vote":                         "tickets.v1.comments.vote",
	"tickets GET /topics":                                              "tickets.v1.topics.list",
	"tickets GET /v2/tickets":                                          "tickets.v2.tickets.list",
	"tickets PATCH /v2/tickets/{number}":                               "tickets.v2.tickets.update",
	"tickets GET /v2/tickets/count":                                    "tickets.v2.tickets.count",
	"tickets GET /v2/categories":                                       "tickets.v2.categories.list",
	"tickets GET /v2/categories/{category}/fields":                     "tickets.v2.category_fields.list",
	"tickets GET /v2/products":                                         "tickets.v2.products.list",
	"tickets POST /v2/tickets":                                         "tickets.v2.tickets.create",
	"tickets-attachments GET /public/files/list":                       "tickets.attachments_v1.files.list",
	"tickets GET /v2/tickets/{number}":                                 "tickets.v2.tickets.get",
	"tickets-attachments POST /public/files/":                          "tickets.attachments_v1.files.upload",
	"tickets-attachments GET /public/files/info":                       "tickets.attachments_v1.files.get_batch",
	"tickets-attachments GET /public/files/extensions":                 "tickets.attachments_v1.extensions.list",
	"tickets-attachments GET /public/files/mimetypes":                  "tickets.attachments_v1.mimetypes.list",
	"tickets-attachments GET /public/files/uuid/{file_uuid}":           "tickets.attachments_v1.files.get",
	"cloud-projects-and-resources GET /v2/projects":                    "projects.v2.projects.list",
	"managed-kubernetes GET /v2/clusters":                              "mks.v2.clusters.list",
	"managed-databases GET /datastores":                                "databases.v1.datastores.list",
	"craas GET /v2/registries":                                         "registry.v2.registries.list",
	"balance GET /v3/balances":                                         "billing.v3.balances.get",
	"balance-statistics GET /v1/cloud_billing/statistic/summary_stats": "billing.v1.statistics.summary",
	"balance-statistics GET /v1/cloud_billing/statistic/consumption":   "billing.v1.statistics.consumption",
	"audit-logs POST /v1/logs":                                         "audit.v1.logs.query",
	"secrets-manager GET /v1/":                                         "secrets.v1.secrets.list",
	"secrets-manager GET /v1/{secret_name}/versions":                   "secrets.v1.versions.list",
	"secrets-manager GET /v1/{secret_name}":                            "secrets.v1.secrets.get",
	"secrets-manager GET /v1/{secret_name}/versions/{version_id}":      "secrets.v1.versions.get",
	"compute-nova GET /servers/detail":                                 "compute.v2_1.servers.list",
	"compute-nova POST /servers":                                       "compute.v2_1.servers.create",
	"compute-nova GET /servers/{server_id}":                            "compute.v2_1.servers.get",
	"compute-nova DELETE /servers/{server_id}":                         "compute.v2_1.servers.delete",
	"cloudwatch-logs POST /DescribeLogGroups":                          "logs.cloudwatch.log_groups.list",
	"cloudwatch-logs POST /DescribeLogStreams":                         "logs.cloudwatch.log_streams.list",
	"cloudwatch-logs POST /GetLogEvents":                               "logs.cloudwatch.events.get",
	"cloudwatch-logs POST /FilterLogEvents":                            "logs.cloudwatch.events.filter",
	"prometheus-metrics GET /prometheus/metrics":                       "metrics.prometheus.samples.scrape",
	"prometheus-metrics GET /api/v1/query":                             "metrics.prometheus.samples.query",
	"prometheus-metrics GET /api/v1/query_range":                       "metrics.prometheus.samples.query_range",
}

func main() {
	if err := generate(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
func generate() error {
	dir := "."
	if _, err := os.Stat("sources"); err != nil {
		dir = "catalog"
	}
	catalogDir, err := filepath.Abs(dir)
	if err != nil {
		return err
	}
	moduleRoot := filepath.Dir(catalogDir)
	files, err := filepath.Glob(filepath.Join(dir, "sources", "*.json"))
	if err != nil {
		return err
	}
	var bindings map[string]binding
	if raw, err := os.ReadFile(filepath.Join(dir, "bindings.json")); err == nil {
		if err := json.Unmarshal(raw, &bindings); err != nil {
			return err
		}
	} else if !os.IsNotExist(err) {
		return err
	}
	var evidence map[string]verification
	if raw, err := os.ReadFile(filepath.Join(dir, "verification.json")); err == nil {
		if err := json.Unmarshal(raw, &evidence); err != nil {
			return err
		}
	} else if !os.IsNotExist(err) {
		return err
	}
	rows := []map[string]any{}
	seen := map[string]bool{}
	complete := map[string]bool{}
	for _, file := range files {
		name := strings.TrimSuffix(filepath.Base(file), ".json")
		f, ok := families[name]
		if !ok {
			return fmt.Errorf("unknown public source family %s", name)
		}
		raw, err := os.ReadFile(file)
		if err != nil {
			return err
		}
		var spec struct {
			Paths     map[string]map[string]json.RawMessage `json:"paths"`
			SourceURI string                                `json:"x-source-uri"`
		}
		if err := json.Unmarshal(raw, &spec); err != nil {
			return err
		}
		revision := hash(raw)
		sourceURI := "https://docs.selectel.ru/api/" + name + ".json"
		if spec.SourceURI != "" {
			sourceURI = spec.SourceURI
		}
		if f.Complete {
			complete[f.Service] = true
		}
		paths := make([]string, 0, len(spec.Paths))
		for path := range spec.Paths {
			paths = append(paths, path)
		}
		sort.Strings(paths)
		for _, path := range paths {
			methods := spec.Paths[path]
			methodNames := make([]string, 0, len(methods))
			for method := range methods {
				methodNames = append(methodNames, method)
			}
			sort.Strings(methodNames)
			for _, method := range methodNames {
				if method != "get" && method != "post" && method != "put" && method != "patch" && method != "delete" && method != "head" && method != "options" {
					continue
				}
				generation := f.Generation
				parts := strings.Split(strings.Trim(path, "/"), "/")
				if len(parts) > 0 && regexp.MustCompile(`^v[0-9]+$`).MatchString(parts[0]) {
					generation = parts[0]
				}
				resource := slug(path)
				action := map[string]string{"get": "get", "post": "create", "put": "replace", "patch": "update", "delete": "delete", "head": "head", "options": "options"}[method]
				if method == "get" && !strings.HasSuffix(strings.TrimRight(path, "/"), "}") {
					action = "list"
				}
				id := f.Service + "." + generation + "." + resource + "." + action
				key := name + " " + strings.ToUpper(method) + " " + path
				if override, ok := overrides[key]; ok {
					id = override
				}
				if seen[id] {
					id += "_" + hash([]byte(key))[:8]
				}
				if seen[id] {
					return fmt.Errorf("duplicate operation id %s", id)
				}
				seen[id] = true
				idParts := strings.Split(id, ".")
				resource, action = idParts[2], idParts[3]
				pointer := "sources/" + filepath.Base(file) + "#/paths/" + strings.ReplaceAll(strings.ReplaceAll(path, "~", "~0"), "/", "~1") + "/" + method
				scope := []string{"account"}
				if f.Mode == "iam_project" {
					scope = append(scope, "project")
				}
				if f.Mode == "s3_sigv4" || f.Mode == "basic" {
					scope = append(scope, "project", "region", "pool")
				}
				effect := "mutation"
				if method == "get" || method == "head" || method == "options" || name == "audit-logs" {
					effect = "read"
				}
				var sourceOperation struct {
					Effect  string `json:"x-effect"`
					Locator string `json:"x-operation-locator"`
				}
				if err := json.Unmarshal(methods[method], &sourceOperation); err != nil {
					return err
				}
				if sourceOperation.Effect != "" {
					effect = sourceOperation.Effect
				}
				locator := strings.ToUpper(method) + " " + path
				if sourceOperation.Locator != "" {
					locator = sourceOperation.Locator
				}
				row := map[string]any{
					"operation_id": id, "service": f.Service, "api_generation": generation, "resource": resource, "action": action, "effect": effect, "availability": "documented",
					"source":    map[string]any{"uri": sourceURI, "revision": revision, "operation_locator": locator},
					"go_method": nil, "input_type": nil, "output_type": nil, "input_schema_ref": pointer, "output_schema_ref": pointer + "/responses",
					"scope_requirements": scope, "auth": map[string]any{"scheme": f.Mode, "audience": "selectel"},
					"endpoint_recipe_ref": nil, "request_binding_ref": nil, "response_binding_ref": nil, "error_binding_ref": pointer + "/responses", "replay_policy_ref": nil,
					"pagination_contract_ref": nil, "wait_goals": []string{}, "recovery_contract_ref": nil, "sensitive_fields": []string{},
					"parameter_support": map[string]any{"covered_schema_ref": nil, "excluded_variants": []string{}}, "limitations": []string{"No callable service binding"},
				}
				b, implemented := bindings[id]
				if implemented {
					contractRaw, err := os.ReadFile(filepath.Join(dir, b.ContractRef))
					if err != nil {
						return err
					}
					row["contract_artifact_revisions"] = map[string]string{b.ContractRef: hash(contractRaw)}
					var contract struct {
						Response struct {
							SensitiveFields []string `json:"sensitive_fields"`
						} `json:"response"`
						Pagination json.RawMessage `json:"pagination"`
					}
					if err := json.Unmarshal(contractRaw, &contract); err != nil {
						return err
					}
					if len(contract.Response.SensitiveFields) > 0 {
						row["sensitive_fields"] = contract.Response.SensitiveFields
					}
					if len(contract.Pagination) > 0 && string(contract.Pagination) != "null" {
						row["pagination_contract_ref"] = b.ContractRef + "#/pagination"
					}
					row["go_method"], row["input_type"], row["output_type"] = b.GoMethod, b.InputType, b.OutputType
					row["endpoint_recipe_ref"], row["request_binding_ref"], row["response_binding_ref"], row["error_binding_ref"], row["replay_policy_ref"] = b.ContractRef+"#/endpoint", b.ContractRef+"#/request", b.ContractRef+"#/response", b.ContractRef+"#/errors", b.ContractRef+"#/retry"
					row["parameter_support"] = map[string]any{"covered_schema_ref": pointer, "excluded_variants": []string{}}
					if b.CoveredSchemaRef != "" {
						row["parameter_support"] = map[string]any{"covered_schema_ref": b.CoveredSchemaRef, "excluded_variants": b.ExcludedVariants}
					}
					if len(b.WaitGoals) > 0 {
						row["wait_goals"] = b.WaitGoals
					}
					if b.Recovery {
						row["recovery_contract_ref"] = b.ContractRef + "#/recovery"
					}
					row["limitations"] = []string{"Verification does not establish live availability"}
				}
				canonical, err := json.Marshal(row)
				if err != nil {
					return err
				}
				row["contract_revision"] = hash(canonical)
				row["implementation"] = map[string]any{"status": "unimplemented", "binding": nil}
				row["verification"] = map[string]any{"level": "none", "contract_revision": nil, "implementation_revision": nil, "evidence_ref": nil}
				if implemented {
					implHash, err := fileHash(moduleRoot, append(append([]string(nil), b.Files...), filepath.Join("catalog", b.ContractRef)))
					if err != nil {
						return err
					}
					row["implementation"] = map[string]any{"status": b.Status, "binding": b.GoMethod}
					for _, evidence := range b.EvidenceFiles {
						if _, err := os.Stat(filepath.Join(moduleRoot, evidence)); err != nil {
							return err
						}
					}
					row["verification"] = map[string]any{"level": "none", "contract_revision": nil, "implementation_revision": implHash, "evidence_ref": nil}
					evidenceHash, err := fileHash(moduleRoot, append([]string(nil), b.EvidenceFiles...))
					if err != nil {
						return err
					}
					proof, ok := evidence[id]
					if ok && proof.Result == "passed" && proof.Level == "offline" && proof.ContractRevision == row["contract_revision"] && proof.ImplementationRevision == implHash && proof.EvidenceRevision == evidenceHash {
						row["verification"] = map[string]any{"level": "offline", "contract_revision": proof.ContractRevision, "implementation_revision": implHash, "evidence_ref": proof.EvidenceRef}
					}
				}
				rows = append(rows, row)
			}
		}
	}
	sort.Slice(rows, func(i, j int) bool { return rows[i]["operation_id"].(string) < rows[j]["operation_id"].(string) })
	if err := writeJSON(filepath.Join(dir, "manifest.json"), rows); err != nil {
		return err
	}
	raw, err := os.ReadFile(filepath.Join(dir, "service-registry.json"))
	if err != nil {
		return err
	}
	var registry []map[string]any
	if err := json.Unmarshal(raw, &registry); err != nil {
		return err
	}
	for _, r := range registry {
		r["inventory_complete"] = complete[r["id"].(string)]
	}
	return writeJSON(filepath.Join(dir, "service-registry.json"), registry)
}

type binding struct {
	GoMethod         string   `json:"go_method"`
	InputType        string   `json:"input_type"`
	OutputType       string   `json:"output_type"`
	ContractRef      string   `json:"contract_ref"`
	Status           string   `json:"status"`
	Files            []string `json:"files"`
	EvidenceFiles    []string `json:"evidence_files"`
	EvidenceRef      string   `json:"evidence_ref"`
	Verification     string   `json:"verification"`
	CoveredSchemaRef string   `json:"covered_schema_ref"`
	ExcludedVariants []string `json:"excluded_variants"`
	WaitGoals        []string `json:"wait_goals"`
	Recovery         bool     `json:"recovery"`
}
type verification struct {
	Level                  string `json:"level"`
	Result                 string `json:"result"`
	ContractRevision       string `json:"contract_revision"`
	ImplementationRevision string `json:"implementation_revision"`
	EvidenceRevision       string `json:"evidence_revision"`
	EvidenceRef            string `json:"evidence_ref"`
}

func (b binding) ContractRefPath(dir string) []string {
	return []string{filepath.Join(filepath.Base(dir), b.ContractRef)}
}
func fileHash(root string, files []string) (string, error) {
	sort.Strings(files)
	h := sha256.New()
	for _, file := range files {
		raw, err := os.ReadFile(filepath.Join(root, file))
		if err != nil {
			return "", err
		}
		_, _ = h.Write([]byte(file))
		_, _ = h.Write([]byte{0})
		_, _ = h.Write(raw)
		_, _ = h.Write([]byte{0})
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}
func hash(b []byte) string { h := sha256.Sum256(b); return hex.EncodeToString(h[:]) }
func writeJSON(path string, v any) error {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(b, '\n'), 0644)
}
func slug(path string) string {
	parts := []string{}
	for _, p := range strings.Split(path, "/") {
		if p == "" || strings.HasPrefix(p, "{") || regexp.MustCompile(`^v[0-9]+$`).MatchString(p) {
			continue
		}
		parts = append(parts, p)
	}
	s := strings.ToLower(strings.Join(parts, "_"))
	s = regexp.MustCompile(`[^a-z0-9_]+`).ReplaceAllString(s, "_")
	if s == "" {
		return "secrets"
	}
	return strings.Trim(s, "_")
}
