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
 "dnslegacy": ("dns-legacy", {"TagSchema": "Tag", "TagBody": "TagUpdateSpec", "DomainOut": "Domain", "ZoneState": "ZoneState", "ZoneStateForm": "ZoneStateSpec", "DomainUpdateForm": "DomainUpdateSpec", "RecordResponseForm": "Record", "RecordCreateForm": "RecordCreateSpec", "RecordUpdateForm": "RecordUpdateSpec", "ZoneSetResponse": "ZoneSetResponse", "RecordBatchForm": "RecordBatchSpec", "RecordBatchUpdateForm": "RecordBatchUpdateSpec", "RecordDeleteForm": "RecordDeleteSpec", "DomainCreationForm": "DomainCreateSpec", "DomainBZOut": "DomainBindResult", "ZoneCopy": "MigrationSpec"}, None),
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
    package = "registry" if case == "registry_v1" else case
    doc = json.loads((ROOT / "catalog/sources" / (family + ".json")).read_text())
    definitions = doc.get("definitions", doc.get("components", {}).get("schemas", {}))
    roots = initial if isinstance(initial, dict) else {initial: top_name}
    resource_roots = set(roots)
    if package == "dnslegacy":
        resource_roots = {"TagSchema", "DomainOut"}
    if package == "mks":
        resource_roots = {"Cluster", "ClusterDetailed", "Nodegroup", "NodegroupDetailed", "NodegroupListItem", "Node", "Task"}
    pending = dict(roots)
    emitted = {}
    imports = {"types": "github.com/festy23/selectel-sdk/types", "json": "encoding/json"}

    def resolve(schema):
        if "allOf" in schema and package == "dnslegacy":
            # The pinned DNS schema uses singleton primitive enum refs with defaults.
            parts = schema["allOf"]
            if len(parts) == 1 and "$ref" in parts[0]:
                base = definitions[parts[0]["$ref"].split("/")[-1]]
                if not base.get("properties"):
                    return dict(base, **{k: v for k, v in schema.items() if k != "allOf"})
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
        if "$ref" in schema:
            reference = schema["$ref"].split("/")[-1]
            if reference not in pending:
                pending[reference] = name(reference)
            referenced = resolve(definitions[reference])
            if referenced.get("properties") or (package == "dnslegacy" and referenced.get("anyOf")):
                return pending[reference], False
            return field_type(referenced, pending[reference], field)
        schema = resolve(schema)
        kind = schema.get("type")
        if package == "dnslegacy" and kind == "string" and field != "type":
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
        if package == "dnslegacy" and schema.get("anyOf") and schema.get("discriminator"):
            mapping = schema["discriminator"]["mapping"]
            variants = []
            for key, uri in sorted(mapping.items()):
                child = uri.split("/")[-1]
                pending.setdefault(child, name(child))
                variants.append((key, pending[child]))
            body = "type " + type_name + " struct {\n"
            for key, child in variants:
                body += f'\t{key} *{child} `json:"-"`\n'
            if reference == "RecordResponseForm":
                body += '\tRef *types.ResourceRef `json:"ref,omitempty"`\n'
            body += "}\n"
            body += f"func (v {type_name}) Validate() error {{\n\tcount := 0\n"
            for key, _ in variants:
                body += f"\tif v.{key} != nil {{ count++; if err := v.{key}.Validate(); err != nil {{ return err }} }}\n"
            body += "\tif count != 1 { return types.NewError(types.ProtocolError) }; return nil\n}\n"
            body += f"func (v {type_name}) wireVariant() (any, error) {{\n\tif err := v.Validate(); err != nil {{ return nil, err }}\n"
            for key, _ in variants:
                body += f"\tif v.{key} != nil {{ return v.{key}, nil }}\n"
            body += "\treturn nil, types.NewError(types.ProtocolError)\n}\n"
            if all("id" in resolve(definitions[uri.split("/")[-1]]).get("properties", {}) for uri in mapping.values()):
                body += f"func (v {type_name}) RecordID() *types.Integer {{\n\tif v.Validate() != nil {{ return nil }}\n"
                for key, _ in variants:
                    body += f"\tif v.{key} != nil && v.{key}.ID != nil {{ value := *v.{key}.ID; return &value }}\n"
                body += "\treturn nil\n}\n"
            body += f"func (v {type_name}) MarshalJSON() ([]byte, error) {{\n\tvalue, err := v.wireVariant(); if err != nil {{ return nil, err }}; raw, err := json.Marshal(value); if err != nil {{ return nil, err }}\n"
            if reference == "RecordResponseForm":
                body += '\tif v.Ref != nil { var fields map[string]json.RawMessage; if err := json.Unmarshal(raw, &fields); err != nil { return nil, err }; fields["ref"], err = json.Marshal(v.Ref); if err != nil { return nil, err }; return json.Marshal(fields) }\n'
            body += "\treturn raw, nil\n}\n"
            body += f"func (v *{type_name}) normalize() {{\n"
            for key, _ in variants:
                body += f"\tif v.{key} != nil {{ v.{key}.normalize() }}\n"
            body += "}\n"
            body += f"func (v *{type_name}) UnmarshalJSON(raw []byte) error {{\n\tvar selector struct {{ Type *string `json:\"type\"` }}; if json.Unmarshal(raw, &selector) != nil || selector.Type == nil {{ return types.NewError(types.ProtocolError) }}\n\tswitch *selector.Type {{\n"
            for key, child in variants:
                body += f"\tcase {json.dumps(key)}: var row {child}; if json.Unmarshal(raw, &row) != nil || row.Validate() != nil {{ return types.NewError(types.ProtocolError) }}; *v = {type_name}{{{key}: &row}}\n"
            body += "\tdefault: return types.NewError(types.ProtocolError)\n\t}; v.normalize(); return nil\n}\n"
            emitted[reference] = body
            continue
        if not schema.get("properties"):
            emitted[reference] = ""
            continue
        properties = schema["properties"]
        fields = []
        required = []
        for wire, value in sorted(properties.items()):
            field_name = name(wire)
            go_type, collection = field_type(value, type_name + field_name, wire)
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
            if package == "dnslegacy" and nested.get("format") in {"ipv4", "ipv6"}:
                imports["netip"] = "net/netip"
                family_check = "Is4" if nested["format"] == "ipv4" else "Is6"
                code += f"\tif v.{field_name} != nil {{ if ip, err := netip.ParseAddr(string(v.{field_name}.Reveal())); err != nil || !ip.{family_check}() {{ return types.NewError(types.ProtocolError) }} }}\n"
            if package == "dnslegacy" and nested.get("type") == "string" and wire != "type":
                imports["utf8"] = "unicode/utf8"
                code += f"\tif v.{field_name} != nil && !utf8.Valid(v.{field_name}.Reveal()) {{ return types.NewError(types.ProtocolError) }}\n"
                if "minLength" in nested:
                    code += f"\tif v.{field_name} != nil && utf8.RuneCount(v.{field_name}.Reveal()) < {nested['minLength']} {{ return types.NewError(types.ProtocolError) }}\n"
                if "maxLength" in nested:
                    code += f"\tif v.{field_name} != nil && utf8.RuneCount(v.{field_name}.Reveal()) > {nested['maxLength']} {{ return types.NewError(types.ProtocolError) }}\n"
            if package == "dnslegacy" and nested.get("type") == "array":
                if "maxItems" in nested:
                    code += f"\tif len(v.{field_name}) > {nested['maxItems']} {{ return types.NewError(types.ProtocolError) }}\n"
                item = nested.get("items", {})
                if item.get("type") == "object" and not item.get("properties"):
                    code += f"\tfor _, item := range v.{field_name} {{ var object map[string]json.RawMessage; if json.Unmarshal(item.Reveal(), &object) != nil || object == nil {{ return types.NewError(types.ProtocolError) }} }}\n"
                if item.get("type") == "integer":
                    code += f"\tfor _, item := range v.{field_name} {{ if _, err := types.ParseInteger(item.String()); err != nil {{ return types.NewError(types.ProtocolError) }} }}\n"
            if package == "dnslegacy" and nested.get("type") == "integer" and any(k in nested for k in ["minimum", "maximum"]):
                imports["big"] = "math/big"
                for key, comparison in [("minimum", "<"), ("maximum", ">")]:
                    if key in nested:
                        code += f"\tif v.{field_name} != nil {{ number, ok := new(big.Int).SetString(v.{field_name}.String(), 10); if !ok || number.Cmp(big.NewInt({nested[key]})) {comparison} 0 {{ return types.NewError(types.ProtocolError) }} }}\n"
            if package == "dnslegacy" and nested.get("type") == "integer":
                code += f"\tif v.{field_name} != nil {{ if _, err := types.ParseInteger(v.{field_name}.String()); err != nil {{ return types.NewError(types.ProtocolError) }} }}\n"
            if package == "dnslegacy" and nested.get("enum"):
                if nested.get("type") == "integer":
                    expression = f"v.{field_name}.String()"
                    values = [str(item) for item in nested["enum"]]
                else:
                    expression = f"*v.{field_name}" if wire == "type" else f"string(v.{field_name}.Reveal())"
                    values = nested["enum"]
                checks = " && ".join(f"{expression} != {json.dumps(item)}" for item in values)
                code += f"\tif v.{field_name} != nil && ({checks}) {{ return types.NewError(types.ProtocolError) }}\n"

            if resolve(nested).get("properties") or (package == "dnslegacy" and nested.get("anyOf")):
                code += f"\tif v.{field_name} != nil {{ if err := v.{field_name}.Validate(); err != nil {{ return err }} }}\n"
            elif nested.get("type") == "array":
                item = nested["items"]
                item = definitions.get(item.get("$ref", "").split("/")[-1], item)
                if resolve(item).get("properties") or (package == "dnslegacy" and item.get("anyOf")):
                    code += f"\tfor _, item := range v.{field_name} {{ if err := item.Validate(); err != nil {{ return err }} }}\n"
        code += "\treturn nil\n}\n"
        code += f"func (v *{type_name}) normalize() {{\n"
        for wire, value in sorted(properties.items()):
            field_name = name(wire)
            value = resolve(value)
            reference_name = value.get("$ref", "").split("/")[-1]
            nested = resolve(definitions.get(reference_name, value))
            if nested.get("format") == "date-time":
                code += f"\tif v.{field_name} != nil {{ instant := v.{field_name}.UTC(); v.{field_name} = &instant }}\n"
            elif nested.get("properties"):
                code += f"\tif v.{field_name} != nil {{ v.{field_name}.normalize() }}\n"
            elif nested.get("type") == "array":
                item = nested["items"]
                item = definitions.get(item.get("$ref", "").split("/")[-1], item)
                if resolve(item).get("properties") or (package == "dnslegacy" and item.get("anyOf")):
                    code += f"\tfor i := range v.{field_name} {{ v.{field_name}[i].normalize() }}\n"
        code += "}\n"
        if reference in roots or package == "dnslegacy":
            code += f"func (v *{type_name}) UnmarshalJSON(raw []byte) error {{\n"
            code += "\tvar fields map[string]json.RawMessage\n\tif json.Unmarshal(raw, &fields) != nil || fields == nil { return types.NewError(types.ProtocolError) }\n"
            if package == "dnslegacy":
                imports["bytes"] = "bytes"
                imports["utf8"] = "unicode/utf8"
                code += '\tif !utf8.Valid(raw) { return types.NewError(types.ProtocolError) }\n'
                for wire in sorted(properties):
                    code += f'\tif value, ok := fields[{json.dumps(wire)}]; ok && bytes.Equal(bytes.TrimSpace(value), []byte("null")) {{ return types.NewError(types.ProtocolError) }}\n'
            code += '\tdelete(fields, "ref")\n\tclean, err := json.Marshal(fields); if err != nil { return types.NewError(types.ProtocolError) }\n'
            code += f"\ttype wire {type_name}; var value wire\n\tif json.Unmarshal(clean, &value) != nil {{ return types.NewError(types.ProtocolError) }}\n\t*v = {type_name}(value); v.normalize(); return nil\n}}\n"
        emitted[reference] = code
    filename = "v1_types_generated.go" if case == "registry_v1" else "types_generated.go"
    target = ROOT / package / filename
    target.parent.mkdir(exist_ok=True)
    header = f"// Code generated by internal/modelgen/generate.py; DO NOT EDIT.\n// Source: catalog/sources/{family}.json.\npackage {package}\n\n"
    header += "import (\n" + "\n".join('"' + value + '"' for value in sorted(imports.values())) + "\n)\n\n"
    target.write_text(header + "\n".join(emitted[key] for key in sorted(emitted)))
    subprocess.run(["gofmt", "-w", str(target)], check=True)
