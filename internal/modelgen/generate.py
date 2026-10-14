#!/usr/bin/env python3
"""Build scoped service DTOs from pinned public schema artifacts.

No credential lookup, network access or cloud state is used by generation.
Schemaless response fields are protected; they require explicit Reveal.
"""
import json
import re
import subprocess
from pathlib import Path

ROOT = Path(__file__).resolve().parents[2]
CASES = {
    "billing_transactions": ("billing-transactions", {"TransactionsResponse":"TransactionsResponse"}, None),
    "billing_report": ("billing-reports", {"ProjectReportResponse":"DetailedReportResponse", "ProjectReportData":"ReportData", "Project":"ReportProject", "Product":"ReportProduct", "ProductObject":"ReportObject", "Resource":"ReportResource", "BalancePayment":"ReportBalancePayment"}, None),
    "billing_statistics": ("balance-statistics", {"@/v1/cloud_billing/statistic/projects": "ProjectsResponse", "@/v1/cloud_billing/statistic/objects": "ObjectsResponse"}, None),
    "billing_prediction": ("balance", {"@/v2/billing/prediction": "PredictionResponse"}, None),
    "projects": ("cloud-projects-and-resources", {
        "_ListProjectsResponse": "Project",
        "_CreateProjectResponse": "CreatedProject",
        "_GetProjectResponse": "DetailedProject",
        "_UpdateProjectResponse": "UpdatedProject",
    }, None),
    "mks": ("managed-kubernetes", {
        "Cluster": "Cluster", "ClusterDetailed": "DetailedCluster",
        "Nodegroup": "Nodegroup", "NodegroupListItem": "NodegroupListItem",
        "NodegroupDetailed": "DetailedNodegroup", "Node": "Node", "Task": "Task",
        "RegistriesIntegration": "RegistriesIntegration",
        "AdmissionControllersList": "AdmissionControllersList",
        "KubeVersionsList": "KubeVersionsList", "FeatureGatesList": "FeatureGatesList",
        "ClusterCreateStruct": "ClusterCreateSpec", "ClusterUpdateStruct": "ClusterUpdateSpec",
        "NodegroupCreateStruct": "NodegroupCreateSpec", "NodegroupUpdateStruct": "NodegroupUpdateSpec",
        "NodegroupResizeStruct": "NodegroupResizeSpec",
        "RegistriesIntegrationCreateStruct": "RegistryIntegrationCreateSpec",
        "RegistriesIntegrationUpdateStruct": "RegistryIntegrationUpdateSpec",
    }, None),
    "databases": ("managed-databases", "datastore", "Datastore"),
    "registry": ("craas", "Registry", "Registry"),
    "registry_v1": ("craas", "registry.Registry", "V1Registry"),
    "audit": ("audit-logs", "LogResponse", "Event"),
}
ACRONYMS = {"id": "ID", "ip": "IP", "url": "URL", "oidc": "OIDC", "cni": "CNI", "cpu": "CPU", "vcpus": "VCPUs", "ram": "RAM", "api": "API", "x509": "X509"}


def name(text):
    words = re.sub(r"([a-z])([A-Z])", r"\1_\2", text).split("_")
    return "".join(ACRONYMS.get(w.lower(), w[:1].upper() + w[1:]) for w in words if w)


for case, (family, initial, top_name) in CASES.items():
    package = "billing" if case.startswith("billing_") else "registry" if case == "registry_v1" else case
    doc = json.loads((ROOT / "catalog/sources" / (family + ".json")).read_text())
    definitions = doc.get("definitions", doc.get("components", {}).get("schemas", {}))
    if case == "billing_transactions":
        # Published 200 example omits server_meta.ru despite required schema.
        definitions["ServerMeta"]["required"] = [field for field in definitions["ServerMeta"]["required"] if field != "ru"]
    roots = initial if isinstance(initial, dict) else {initial: top_name}
    for key in list(roots):
        if key.startswith("@"):
            definitions[key] = doc["paths"][key[1:]]["get"]["responses"]["200"]["content"]["application/json"]["schema"]
    resource_roots = set() if package == "billing" else set(roots)
    if package == "mks":
        resource_roots = {"Cluster", "ClusterDetailed", "Nodegroup", "NodegroupDetailed", "NodegroupListItem", "Node", "Task"}
    pending = dict(roots)
    emitted = {}
    imports = {"types": "github.com/festy23/selectel-sdk/types", "json": "encoding/json"}

    def resolve(schema):
        if "allOf" in schema:
            merged = {"properties": dict(schema.get("properties", {})), "required": list(schema.get("required", []))}
            for part in schema["allOf"]:
                if "$ref" in part:
                    part = definitions[part["$ref"].split("/")[-1]]
                merged["properties"].update(part.get("properties", {}))
                merged["required"].extend(part.get("required", []))
            return merged
        return schema

    def field_type(schema, type_name, field):
        if package == "billing" and "oneOf" in schema:
            return "Identifier", False
        if "$ref" in schema:
            reference = schema["$ref"].split("/")[-1]
            if package == "billing" and reference == "LocalizedText":
                return "map[string]types.ProtectedValue", True
            if reference not in pending:
                pending[reference] = name(reference)
            referenced = resolve(definitions[reference])
            if referenced.get("properties"):
                return pending[reference], False
            return field_type(referenced, pending[reference], field)
        schema = resolve(schema)
        kind = schema.get("type")
        if package == "billing" and kind == "integer" and field in {"value", "price"}:
            return "Amount", False
        if package == "billing" and schema.get("format") == "date-time":
            return "ProviderTimestamp", False
        if package == "billing" and field in {"name", "service_name", "service_name_en", "full_name", "operation", "service", "reason_for_debt", "jurbrand_key"} and kind == "string":
            return "types.ProtectedValue", False
        if package == "mks" and ((field == "user_data" and kind == "string") or (type_name.startswith("TaskErrorDetails") and field in {"name", "details"})):
            return "types.ProtectedValue", False
        if field == "parameters" and package == "audit":
            return "types.ProtectedValue", False
        if kind == "array":
            inner, _ = field_type(schema["items"], type_name + "Item", "")
            return "[]" + inner, True
        if schema.get("properties"):
            reference = "inline_" + type_name
            definitions[reference] = schema
            pending[reference] = type_name
            return type_name, False
        if kind == "integer":
            return "types.Integer", False
        if kind == "number":
            return "types.Number", False
        if kind == "boolean":
            return "bool", False
        if kind == "string":
            if schema.get("format") == "date-time":
                imports["time"] = "time"
                return "time.Time", False
            return "string", False
        if isinstance(schema.get("additionalProperties"), dict) and schema["additionalProperties"]:
            inner, _ = field_type(schema["additionalProperties"], type_name + "Value", "")
            return "map[string]" + inner, True
        if kind == "object" or kind is None:
            return "types.ProtectedJSON", False
        raise ValueError(f"Unbound schema variant {package}.{type_name}: {kind!r}")

    while True:
        todo = [key for key in pending if key not in emitted]
        if not todo:
            break
        reference = todo[0]
        type_name = pending[reference]
        schema = resolve(definitions[reference])
        if not schema.get("properties"):
            emitted[reference] = ""
            continue
        properties = schema["properties"]
        fields = []
        required = []
        nullable = []
        for wire, value in sorted(properties.items()):
            field_name = name(wire)
            go_type, collection = field_type(value, type_name + field_name, wire)
            if package == "billing" and value.get("nullable"):
                nullable_type = "OptionalText" if go_type == "types.ProtectedValue" else "types.Nullable[" + go_type + "]"
                nullable.append((field_name, wire, nullable_type))
                go_type = nullable_type
            tag = "omitzero" if collection else "omitempty"
            if not collection:
                go_type = "*" + go_type
            fields.append(f'\t{field_name} {go_type} `json:"{wire},{tag}"`')
            if wire in schema.get("required", []):
                required.append(f"v.{field_name} == nil")
        if reference in resource_roots:
            fields.append('\tRef *types.ResourceRef `json:"ref,omitempty"`')
        code = "type " + type_name + " struct {\n" + "\n".join(fields) + "\n}\n"
        code += "func (v " + type_name + ") Validate() error {\n"
        if required:
            code += "\tif " + " || ".join(required) + " { return types.NewError(types.ProtocolError) }\n"
        for wire, value in sorted(properties.items()):
            field_name = name(wire)
            value = resolve(value)
            ref = value.get("$ref", "").split("/")[-1]
            nested = definitions.get(ref, value)
            if package == "billing" and ref == "LocalizedText":
                continue
            if resolve(nested).get("properties"):
                code += f"\tif v.{field_name} != nil {{ if err := v.{field_name}.Validate(); err != nil {{ return err }} }}\n"
            elif nested.get("type") == "array":
                item = nested["items"]
                item = definitions.get(item.get("$ref", "").split("/")[-1], item)
                if resolve(item).get("properties"):
                    code += f"\tfor _, item := range v.{field_name} {{ if err := item.Validate(); err != nil {{ return err }} }}\n"
        code += "\treturn nil\n}\n"
        code += f"func (v *{type_name}) normalize() {{\n"
        for wire, value in sorted(properties.items()):
            field_name = name(wire)
            value = resolve(value)
            reference_name = value.get("$ref", "").split("/")[-1]
            nested = resolve(definitions.get(reference_name, value))
            if package == "billing" and reference_name == "LocalizedText":
                continue
            if nested.get("format") == "date-time" and package != "billing":
                code += f"\tif v.{field_name} != nil {{ instant := v.{field_name}.UTC(); v.{field_name} = &instant }}\n"
            elif nested.get("properties"):
                code += f"\tif v.{field_name} != nil {{ v.{field_name}.normalize() }}\n"
            elif nested.get("type") == "array":
                item = nested["items"]
                item = definitions.get(item.get("$ref", "").split("/")[-1], item)
                if resolve(item).get("properties"):
                    code += f"\tfor i := range v.{field_name} {{ v.{field_name}[i].normalize() }}\n"
        code += "}\n"
        if reference in roots or package == "billing":
            code += f"func (v *{type_name}) UnmarshalJSON(raw []byte) error {{\n"
            code += "\tvar fields map[string]json.RawMessage\n\tif json.Unmarshal(raw, &fields) != nil || fields == nil { return types.NewError(types.ProtocolError) }\n"
            code += '\tdelete(fields, "ref")\n\tclean, err := json.Marshal(fields); if err != nil { return types.NewError(types.ProtocolError) }\n'
            code += f"\ttype wire {type_name}; var value wire\n\tif json.Unmarshal(clean, &value) != nil {{ return types.NewError(types.ProtocolError) }}\n"
            for field_name, wire, go_type in nullable:
                code += f'\tif raw, present := fields["{wire}"]; present {{ n := new({go_type}); if json.Unmarshal(raw,n) != nil {{ return types.NewError(types.ProtocolError) }}; value.{field_name} = n }}\n'
            code += f"\t*v = {type_name}(value); v.normalize(); return nil\n}}\n"

        emitted[reference] = code
    filename = case.removeprefix("billing_") + "_types_generated.go" if package == "billing" else "v1_types_generated.go" if case == "registry_v1" else "types_generated.go"
    target = ROOT / package / filename
    target.parent.mkdir(exist_ok=True)
    header = f"// Code generated by internal/modelgen/generate.py; DO NOT EDIT.\n// Source: catalog/sources/{family}.json.\npackage {package}\n\n"
    header += "import (\n" + "\n".join('"' + value + '"' for value in sorted(imports.values())) + "\n)\n\n"
    target.write_text(header + "\n".join(emitted[key] for key in sorted(emitted)))
    subprocess.run(["gofmt", "-w", str(target)], check=True)
