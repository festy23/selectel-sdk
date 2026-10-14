#!/usr/bin/env python3
"""Build the source-bound Neutron core subset from local pinned SDK snapshots."""
import hashlib
import json
import re
from pathlib import Path

ROOT = Path(__file__).resolve().parents[2]
ASSETS = Path(__file__).resolve().parent / "assets"
REVISION = "460f09b941de260160c68d1c9cac3ae792096bf5"
definitions = {}
roots = {}

for resource in ("network", "subnet", "port"):
    source = (ASSETS / f"vpc-{resource}.go.txt").read_text()
    structs = dict(re.findall(r"type (\w+) struct \{\n(.*?)^\}", source, re.M | re.S))
    included = {name: body for name, body in structs.items() if name[0].isupper() and 'json:"' in body}
    for name, body in included.items():
        reference = resource + "." + name
        go_name = resource.title() + ("CreateSpec" if name == "CreateRequest" else "UpdateSpec" if name == "UpdateRequest" else "" if name == resource.title() else name)
        roots[reference] = go_name
        properties = {}
        required = []
        for field, kind, wire, flags in re.findall(r'^\s*(\w+)\s+([\w.*\[\]]+)\s+`json:"([^",]+)([^"`]*)"`', body, re.M):
            pointer = kind.startswith("*")
            kind = kind.lstrip("*")
            nullable = kind == "vpc.Optional[string]" or (pointer and "omitempty" not in flags)
            if kind == "vpc.Optional[string]":
                kind = "string"
            array = kind.startswith("[]")
            kind = kind.removeprefix("[]")
            if kind in ("string", "bool", "int"):
                value = {"type": {"string": "string", "bool": "boolean", "int": "integer"}[kind]}
            else:
                value = {"$ref": "#/definitions/" + resource + "." + kind}
            if array:
                value = {"type": "array", "items": value}
            if nullable:
                value["x-nullable"] = True
            value["x-go-field-name"] = field
            properties[wire] = value
            if "omitempty" not in flags and name != resource.title():
                required.append(wire)
        if name in ("CreateRequest", "UpdateRequest"):
            properties.pop("network_id", None)  # supplied by the explicit network parent ref
            required = [field for field in required if field != "network_id"]
        definitions[reference] = {"type": "object", "properties": properties, "required": required}

# Standard core/alias fields represented by the pinned Gophercloud adapter.
for resource in ("network", "subnet", "port"):
    definitions[resource + "." + resource.title()]["properties"]["tenant_id"] = {"type": "string", "x-go-field-name": "TenantID"}
    definitions[resource + ".CreateRequest"]["properties"]["tenant_id"] = {"type": "string", "x-go-field-name": "TenantID"}
for variant in ("CreateRequest", "UpdateRequest"):
    definitions["network." + variant]["properties"]["shared"] = {"type": "boolean", "x-go-field-name": "Shared"}

definitions["port.UpdateRequest"]["properties"]["mac_address"] = {"type": "string", "x-go-field-name": "MACAddress"}

for variant in ("CreateRequest", "UpdateRequest"):
    definitions["network." + variant]["properties"]["mtu"] = {"type": "integer", "x-go-field-name": "MTU"}
    definitions["network." + variant]["properties"]["router:external"] = {"type": "boolean", "x-go-field-name": "RouterExternal"}

for resource in ("network", "port"):
    for variant in (resource.title(), "CreateRequest", "UpdateRequest"):
        definitions[resource + "." + variant]["properties"]["qos_policy_id"] = {"type": "string", "x-nullable": True, "x-go-field-name": "QoSPolicyID"}
for variant in ("Network", "CreateRequest", "UpdateRequest"):
    definitions["network." + variant]["properties"]["vlan_transparent"] = {"type": "boolean", "x-go-field-name": "VLANTransparent"}
definitions["port.Port"]["properties"]["qos_network_policy_id"] = {"type": "string", "x-nullable": True, "x-go-field-name": "QoSNetworkPolicyID"}

# dns_assignment is read-only: Selectel port reference, pinned Neutron DNS
# integration contract, and pinned Gophercloud DNS response fixtures.
definitions["port.DNSAssignment"] = {"type": "object", "properties": {
    "ip_address": {"type": "string", "x-go-field-name": "IPAddress"},
    "hostname": {"type": "string", "x-go-field-name": "Hostname"},
    "fqdn": {"type": "string", "x-go-field-name": "FQDN"},
}}
roots["port.DNSAssignment"] = "PortDNSAssignment"
definitions["port.Port"]["properties"]["dns_assignment"] = {"type": "array", "items": {"$ref": "#/definitions/port.DNSAssignment"}, "x-go-field-name": "DNSAssignment"}

paths = {}
for resource in ("network", "subnet", "port"):
    root = "/v2.0/" + resource + "s"
    paths[root] = {
        "get": {"operationId": "List", "resource": resource, "native_request_type": "vpc.ListOptions", "native_response_type": roots[resource + "." + resource.title()] + "[]"},
        "post": {"operationId": "Create", "resource": resource, "native_request_type": roots[resource + ".CreateRequest"], "native_response_type": roots[resource + "." + resource.title()]},
    }
    paths[root + "/{" + resource + "_id}"] = {method: {"operationId": action, "resource": resource} for method, action in (("get", "Get"), ("put", "Update"), ("delete", "Delete"))}

doc = {
    "x-source-uri": "https://github.com/selectel/vpc-go/tree/" + REVISION,
    "x-inventory-kind": "SDK-owned networks/subnets/ports CRUD/list subset, not complete Neutron inventory",
    "x-source-revision": REVISION,
    "x-api-reference": "https://docs.openstack.org/api-ref/network/v2/",
    "x-generator-roots": roots,
    "x-compatible-api-revision": "openstack/neutron-lib 3.0.0 89dc650220244628ea78f58ea2771a2ca0c63a73 (Zed API reference)",
    "x-snapshot-sha256": {path.name: hashlib.sha256(path.read_bytes()).hexdigest() for path in sorted(ASSETS.glob("*.txt"))},
    "x-upstream-asset-uris": {
        "gopher-ext-dns-results.go.txt": "https://github.com/gophercloud/gophercloud/blob/v2.13.0/openstack/networking/v2/extensions/dns/results.go",
        "gopher-ext-dns-fixtures_test.go.txt": "https://github.com/gophercloud/gophercloud/blob/v2.13.0/openstack/networking/v2/extensions/dns/testing/fixtures_test.go",
        **{f"neutron-lib-{name}.inc.txt": f"https://github.com/openstack/neutron-lib/blob/89dc650220244628ea78f58ea2771a2ca0c63a73/api-ref/source/v2/{name}.inc" for name in ("networks", "ports", "subnets")},
        **{f"gopher-ext-{extension}-{name}.txt": f"https://github.com/gophercloud/gophercloud/blob/v2.13.0/openstack/networking/v2/extensions/{extension}/{name}" for extension in ("mtu", "external", "vlantransparent") for name in ("requests.go", "results.go")},
        **{f"gopher-ext-qos-policies-{name}.txt": f"https://github.com/gophercloud/gophercloud/blob/v2.13.0/openstack/networking/v2/extensions/qos/policies/{name}" for name in ("requests.go", "results.go")},
        "selectel-private-networks.md.txt": "https://docs.selectel.ru/cloud-servers/cloud-networks/private-networks-and-subnets.md",
        "selectel-about-networks.md.txt": "https://docs.selectel.ru/cloud-servers/cloud-networks/about-networks.md",
        "selectel-ports.md.txt": "https://docs.selectel.ru/cloud-servers/cloud-networks/ports.md",
        "selectel-network-reference.md.txt": "https://docs.selectel.ru/terraform/openstack-provider-reference/networking-neutron/resources/openstack_networking_network_v2.md",
        "selectel-port-reference.md.txt": "https://docs.selectel.ru/terraform/openstack-provider-reference/networking-neutron/resources/openstack_networking_port_v2.md",
        **{f"vpc-{resource}.go.txt": f"https://github.com/selectel/vpc-go/blob/{REVISION}/pkg/v2/{resource}/{resource}.go" for resource in ("network", "subnet", "port")},
        **{f"gopher-{plural}-{name}.txt": f"https://github.com/gophercloud/gophercloud/blob/v2.13.0/openstack/networking/v2/{plural}/{name}" for plural in ("networks", "subnets", "ports") for name in ("requests.go", "results.go", "urls.go")},
        **{f"vpc-internal-api-{name}.go.txt": f"https://github.com/selectel/vpc-go/blob/{REVISION}/internal/api/{name}.go" for name in ("client", "errors", "pagination")},
        "vpc-pkg-v2-query.go.txt": f"https://github.com/selectel/vpc-go/blob/{REVISION}/pkg/v2/query.go",
    },
    "x-upstream-assets": {path.name: path.read_text() for path in sorted(ASSETS.glob("*.txt"))},
    "paths": paths,
    "definitions": definitions,
}
(ROOT / "catalog/sources/network-core.json").write_text(json.dumps(doc, indent=2) + "\n")

fields = ['// Code generated by internal/networkcontractgen; DO NOT EDIT.', 'package network', '']
for resource in ('network', 'subnet', 'port'):
    title = resource.title()
    fields += ['type ' + title + 'Field string', 'const (']
    for wire, prop in sorted(definitions[resource + '.' + title]['properties'].items()):
        fields.append(title + 'Field' + prop['x-go-field-name'] + ' ' + title + 'Field = ' + json.dumps(wire))
    fields += [')', 'func valid' + title + 'Field(v ' + title + 'Field) bool { switch v {']
    constants = [title + 'Field' + prop['x-go-field-name'] for wire, prop in sorted(definitions[resource + '.' + title]['properties'].items())]
    fields += ['case ' + ', '.join(constants) + ': return true', '}; return false }', '']
    query_source = (ASSETS / f"gopher-{resource}s-requests.go.txt").read_text()
    list_opts = re.search(r"type ListOpts struct \{\n(.*?)^\}", query_source, re.M | re.S).group(1)
    # Preserve the existing source-model attribute filters; add explicit query
    # selectors separately. Do not import excluded subnet-pool/segment plugins.
    filters = {wire: prop['x-go-field-name'] for wire, prop in definitions[resource + '.' + title]['properties'].items() if wire != 'dns_assignment'}
    tag_filters = {"tags", "tags-any", "not-tags", "not-tags-any"}
    filters.update({wire: field for field, wire in re.findall(r'^\s*(\w+).*`q:"([^",]+)', list_opts, re.M) if wire in tag_filters})
    fields += ['type ' + title + 'Filter string', 'const (']
    fields += [title + 'Filter' + field + ' ' + title + 'Filter = ' + json.dumps(wire) for wire, field in sorted(filters.items())]
    fields += [')', 'func valid' + title + 'Filter(v ' + title + 'Filter) bool { switch v {']
    fields += ['case ' + ', '.join(title + 'Filter' + field for wire, field in sorted(filters.items())) + ': return true', '}; return false }', '']
(ROOT / 'network/fields_generated.go').write_text('\n'.join(fields))

# Bindings/contracts are SDK-owned artifacts, regenerated with the source DTOs.
import subprocess
subprocess.run(['gofmt', '-w', str(ROOT / 'network/fields_generated.go')], check=True)
files = ['client.go', 'generate.go', 'internal/cataloggen/main.go', 'internal/modelgen/generate.py',
         'internal/networkcontractgen/generate.py'] + [str(path.relative_to(ROOT)) for path in sorted((ROOT / 'network').glob('*.go')) if not path.name.endswith('_test.go')]
evidence = ['network/core_test.go', 'network/catalog_test.go', 'internal/core/request_test.go', 'pagination/pagination_test.go']
bindings_path = ROOT / 'catalog/bindings.json'
bindings = json.loads(bindings_path.read_text())
for resource in ('network', 'subnet', 'port'):
    title = resource.title()
    for action, method in (('create', 'POST'), ('list', 'GET'), ('get', 'GET'), ('update', 'PUT'), ('delete', 'DELETE')):
        operation = 'network.v2.' + resource + 's.' + action
        path = '/v2.0/' + resource + 's' + ('/{' + resource + '_id}' if action in ('get', 'update', 'delete') else '')
        contract = {
            'operation_id': operation,
            'effect': 'read' if method == 'GET' else 'mutation',
            'endpoint': {'generation': 'v2', 'source': 'IAM project catalog network public endpoint for exact region/pool, or exact explicit override', 'path_prefix': '/v2.0', 'redirects': 'reject'},
            'request': {'method': method, 'path': path, 'schema': 'sources/network-core.json#/definitions/' + resource + '.' + ('CreateRequest' if action == 'create' else 'UpdateRequest' if action == 'update' else title),
                        'scope': 'explicit IAM project/location auth view, never inferred from resource project_id',
                        'owner': 'project_id/tenant_id select explicit target owner on create, aliases must agree; omission uses token project. Shared/accessible reads preserve provider owner as metadata and retain caller scope in refs.',
                        'headers': 'optional If-Match revision_number=<exact nonnegative Integer> on update/delete',
                        'query': 'generated source-model attribute filters plus pinned Gophercloud tag query selectors, including tags/tags-any/not-tags/not-tags-any; separate generated response projection/sort selectors; repeated values preserved; pagination owns limit/marker',
                        'excluded_variants': ['arbitrary plugin extension bodies/filters', 'provider:* physical network fields', 'binding:* host/VIF fields', 'subnet pools, segments and routed provider networks', 'port binding/propagate_uplink_status/value_specs', 'tag mutations (separate capability)', 'bulk create/update/delete', 'provider-segments conditional physical plugin variants', 'port_security_enabled writes: Selectel explicitly disallows toggling traffic filtering']},
            'response': {'successful_statuses': [201 if action == 'create' else 204 if action == 'delete' else 200], 'dto_source': 'network/types_generated.go',
                         'fields': 'all pinned Selectel vpc-go resource response fields plus source-bound tenant_id/QoS/VLAN and read-only port dns_assignment (ip_address, hostname, fqdn); no synthetic ownership or timestamp offset',
                         'resource_ref': 'ID must be nonempty for full responses; subnet/port network_id binds exact parent in same auth view. Projection may omit identity/parent and then list Ref is nil; exact Get retains request identity. Provider ref field discarded.',
                         'source_timestamps': 'raw strings, preserving fractional digits and absent timezone', 'sensitive_fields': []},
            'errors': {'400': 'provider_error', '401': 'unauthenticated', '403': 'permission_denied', '404': 'not_found', '409': 'conflict', '412': 'provider_error; revision precondition failure',
                       'messages': 'fixed SDK diagnostics; raw Neutron message/detail discarded; no undocumented machine-code classification',
                       'malformed_success': 'protocol_error or scope_mismatch; Accepted mutation receipt retains trustworthy identity when extracted',
                       'ambiguous_mutation': 'unknown_outcome with retained receipt and no replay'},
            'retry': 'bounded safe reads; single-attempt mutations',
            'pagination': {'end': 'no next link (previous when page_reverse=true); never infer completion from short page', 'link': 'same endpoint/path only; consume marker, rebuild original selection', 'limits': 'PageSize, MaxItems, MaxPages, CollectionTimeout; cursor binds exact scope, endpoint, generation, selection and contract revision', 'consistency': 'best_effort, no snapshot guarantee'} if action == 'list' else None,
            'wait': {'availability': 'unavailable; CRUD success is logical API acknowledgement, not dataplane health'},
            'recovery': {'availability': 'root Resource/Resume unavailable; exact Get observes identity but does not prove authorship of an unknown mutation'},
            'source_inventory_complete': False,
        }
        (ROOT / 'catalog/contracts' / (operation + '.json')).write_text(json.dumps(contract, indent=2) + '\n')
        input_type = (title + 'CreateSpec' if resource == 'network' else title + 'CreateRequest') if action == 'create' else title + 'UpdateRequest' if action == 'update' else 'DeleteRequest' if action == 'delete' else title + 'Filters, pagination.Options' if action == 'list' else 'types.ResourceRef, network.' + title + 'GetOptions, types.CallOptions'
        bindings[operation] = {'go_method': 'network.' + title + 's.' + ('ListPage' if action == 'list' else action.title()), 'input_type': 'network.' + input_type if action != 'get' else input_type,
                               'output_type': 'pagination.Page[network.' + title + ']' if action == 'list' else 'types.Result[struct{}]' if action == 'delete' else 'types.Result[network.' + title + ']',
                               'contract_ref': 'contracts/' + operation + '.json', 'status': 'implemented', 'files': files, 'evidence_files': evidence, 'evidence_ref': 'network/core_test.go', 'verification': 'offline'}
bindings_path.write_text(json.dumps(bindings, indent=2) + '\n')
