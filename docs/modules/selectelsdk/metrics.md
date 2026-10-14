# Metrics: confirmed read subset

The [pinned source reconciliation](../../../catalog/evidence/metrics-read-sources.json)
covers current Selectel Prometheus, Grafana, VMAgent, Telegraf, get-metrics and
metric-directory pages. These explicitly confirm the namespaces compute,
dedicated and inference, project/pool access using Basic user ID/password, the
scrape URL and the Grafana integration base. Dedicated pool aliases use the
corresponding documented cloud-pool metrics address. Requests keep exact
account/project/region/pool, generation and endpoint; redirects are rejected.

Scrape reads GET `/projects/{project}/namespaces/{namespace}/prometheus/metrics`
using legacy Prometheus text 0.0.4. Query and QueryRange retain their existing
compatible GET `/api/v1/query` and `/api/v1/query_range` bindings under the
integration base. Grafana integration wording does not establish that all
upstream parameters, methods and formats are Selectel-supported. All three
bindings remain partial; the service inventory is incomplete and
coverage_ratio is null. No additional confirmed remote operation was found in
the bounded source review. This is not proof no other operation exists.

POST form queries, timeout/limit/lookback_delta/stats options, OpenMetrics
exemplars/UTF8 identifiers, arbitrary extra namespaces and namespace CRUD remain
unconfirmed and unexposed. The documented remote_write configurations send
scraped samples to caller-owned external storage; they do not prove Selectel
ingestion support. No retention or completeness guarantee is inferred from the
Grafana history window. Different tools reading the same scrape endpoint do
not create additional remote operations.

Query accepts a nonempty PromQL expression and optional explicit evaluation
time. QueryRange additionally requires Start < End and a positive Step; UTC
RFC3339Nano and exact decimal seconds avoid rounding durations. Response
sample timestamps preserve decimal literals; numeric values retain original
strings including NaN/Inf. Vector, matrix, scalar/string and native histogram
shapes are compatible typed decoder variants, not availability claims for
particular metric families. The namespace directory supplies allowed names,
not dynamic namespace discovery or a readiness proof.

Arbitrary string results use generated StringSample with a ProtectedValue
Value: caller-directed `Value.Reveal()` is required. JSON and diagnostic
formatting redact it. A null sample string is malformed rather than an empty
value. Warnings, infos and scrape HELP text are protected as before. Numeric
samples and typed labels are caller data; no example or diagnostic prints raw
telemetry or credentials.

CallTimeout and MaxJSONBytes bound reads. MaxMetricSamples returns a typed
bounded prefix with response_limit and Complete=false; warnings/infos,
namespace and requested range/evaluation metadata remain available. A byte
limit returns no decoded sample data. Complete describes the decoded bounded
response, not a complete inventory of available metrics or namespaces.
Provider errors are sanitized; response bodies/messages are not logged.

The [read helper](../../../examples/metrics-read/main.go) requires an explicit
scoped client and prints nothing. Public Client tests use synthetic credentials
and only HTTP transport substitution. Namespace/interface matrix tests,
dedicated aliases, precise values, protected text and partial metadata are
offline evidence, not live verification.
