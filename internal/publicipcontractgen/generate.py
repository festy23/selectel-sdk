#!/usr/bin/env python3
"""Generate public IP models from local pinned public API/SDK snapshots only."""
import copy
import hashlib
import json
from pathlib import Path
ROOT=Path(__file__).resolve().parents[2]
ASSETS=Path(__file__).resolve().parent/'assets'
source=json.loads((ASSETS/'public-network-openapi.json').read_text())
source['x-source-uri']='https://docs.selectel.ru/api/cloud-public-network.json'
source['x-source-revision']='sha256:'+hashlib.sha256((ASSETS/'public-network-openapi.json').read_bytes()).hexdigest()
(ROOT/'catalog/sources/public-network.json').write_text(json.dumps(source,indent=2)+'\n')
roots={'PublicPort':'DirectPort','PortCreateRequest':'DirectPortCreateSpec','PortUpdateRequest':'DirectPortUpdateSpec','QuotaItem':'DirectQuotaItem','ProjectQuotasResponse':'DirectQuotas'}
definitions=copy.deepcopy(source['components']['schemas'])
fields={'id':'ID','project_id':'ProjectID','network_id':'NetworkID','ip_address':'IPAddress','security_group_ids':'SecurityGroupIDs','network_direct_public_ips':'NetworkDirectPublicIPs','loadbalancer':'LoadBalancer'}
for definition in definitions.values():
 for wire,prop in definition.get('properties',{}).items():
  prop['x-go-field-name']=fields.get(wire,''.join(word.title() for word in wire.split('_')))
  if prop.get('nullable'):prop['x-nullable']=True
definitions['DirectPortNativeCreateSpec']={'type':'object','required':['project_id'],'properties':{
    'project_id':{'type':'string','x-go-field-name':'ProjectID'},
    'description':{'type':'string','x-go-field-name':'Description'},
    'security_group_ids':{'type':'array','items':{'type':'string'},'x-go-field-name':'SecurityGroupIDs','x-wire-always':True},
    'admin_state_up':{'type':'boolean','x-go-field-name':'AdminStateUp'},
}}
roots['DirectPortNativeCreateSpec']='DirectPortNativeCreateSpec'
definitions['AllocationNativeSpec']={'type':'object','required':['region'],'properties':{'region':{'type':'string','x-go-field-name':'Region'}}}
roots['AllocationNativeSpec']='AllocationNativeSpec'
definitions['DirectPortNativeUpdateSpec']=copy.deepcopy(definitions['DirectPortNativeCreateSpec'])
definitions['DirectPortNativeUpdateSpec']['properties'].pop('project_id')
definitions['DirectPortNativeUpdateSpec']['required']=[]
roots['DirectPortNativeUpdateSpec']='DirectPortNativeUpdateSpec'
resell=json.loads((ROOT/'catalog/sources/cloud-projects-and-resources.json').read_text())
resell_roots={'_GetFloatingIPResponse':'Allocation','_CreateFloatingIPRequest':'AllocationSpec','LoadBalancer':'AllocationLoadBalancer','Server':'AllocationServer'}
for old,new in resell_roots.items():
    definition=copy.deepcopy(resell['definitions'][old])
    for wire,prop in definition.get('properties',{}).items():
        prop['x-go-field-name']=fields.get(wire, {'floating_ip_address':'FloatingIPAddress','fixed_ip_address':'FixedIPAddress','port_id':'PortID','ips':'IPs'}.get(wire,''.join(word.title() for word in wire.split('_'))))
        if prop.get('x-nullable'):prop['x-nullable']=True
    definitions[new]=definition
    roots[new]=new
for key in resell_roots.values():
    encoded=json.dumps(definitions[key])
    for old,new in resell_roots.items():encoded=encoded.replace('#/definitions/'+old, '#/definitions/'+new)
    definitions[key]=json.loads(encoded)
def strings(names):
    return {wire:{'type':'string','x-go-field-name':field} for wire,field in names.items()}
floating_fields={'id':'ID','description':'Description','floating_network_id':'FloatingNetworkID','floating_ip_address':'FloatingIPAddress','port_id':'PortID','fixed_ip_address':'FixedIPAddress','tenant_id':'TenantID','project_id':'ProjectID','status':'Status','router_id':'RouterID','created_at':'CreatedAt','updated_at':'UpdatedAt'}
definitions['FloatingIP']={'type':'object','properties':strings(floating_fields)}
for wire in ('port_id','fixed_ip_address','router_id'):
    definitions['FloatingIP']['properties'][wire]['x-nullable']=True
definitions['FloatingIP']['properties']['tags']={'type':'array','items':{'type':'string'},'x-go-field-name':'Tags'}
definitions['FloatingIP']['properties']['revision_number']={'type':'integer','x-go-field-name':'RevisionNumber'}
definitions['FloatingIPCreateSpec']={'type':'object','properties':strings({'description':'Description','floating_ip_address':'FloatingIPAddress','fixed_ip_address':'FixedIPAddress','subnet_id':'SubnetID','tenant_id':'TenantID','project_id':'ProjectID'})}
definitions['FloatingIPUpdateSpec']={'type':'object','properties':strings({'description':'Description','fixed_ip_address':'FixedIPAddress'})}
definitions['FloatingIPUpdateSpec']['properties']['fixed_ip_address']['x-nullable']=True
for key in ('FloatingIP','FloatingIPCreateSpec','FloatingIPUpdateSpec'):roots[key]=key
neutron={'x-source-uri':'https://github.com/selectel/vpc-go/tree/460f09b941de260160c68d1c9cac3ae792096bf5/pkg/v2/floatingip', 'x-source-revision':'460f09b941de260160c68d1c9cac3ae792096bf5; gophercloud v2.13.0; neutron-lib 89dc650220244628ea78f58ea2771a2ca0c63a73','paths':{}}
for path,methods in {'/v2.0/floatingips':('get','post'),'/v2.0/floatingips/{floatingip_id}':('get','put','delete')}.items():
    neutron['paths'][path]={m:{'operationId':m,'responses':{str(201 if m=='post' else 204 if m=='delete' else 200):{'description':'source-defined success'}}} for m in methods}
(ROOT/'catalog/sources/neutron-floating-ips.json').write_text(json.dumps(neutron,indent=2)+'\n')
models={'x-source-uri':source['x-source-uri'],'x-source-revision':source['x-source-revision'],'x-generator-roots':roots,'x-resource-roots':['PublicPort','Allocation','FloatingIP'],'definitions':definitions}
models['x-secondary-sources']={'Resell API':'sources/cloud-projects-and-resources.json','Resell SDK':'https://github.com/selectel/go-selvpcclient/tree/79b0d0ff1d46ebfe7c50056f8c829e0d32349605','Direct SDK':'https://github.com/selectel/public-net-api-go/tree/75bee6c707a155bd1ff0514a35fe58c7cbbca7f6','endpoint/auth recipe':'https://github.com/selectel/terraform-provider-selectel/tree/cf33c549d0c910c137f4a51411f6d709eb2be7d4/selectel','Neutron compatible API':'https://github.com/openstack/neutron-lib/blob/89dc650220244628ea78f58ea2771a2ca0c63a73/api-ref/source/v2/floatingips.inc'}
models['x-snapshot-sha256']={p.name:hashlib.sha256(p.read_bytes()).hexdigest() for p in sorted(ASSETS.iterdir()) if p.is_file()}
models['x-upstream-assets']={p.name:p.read_text() for p in sorted(ASSETS.glob('*.txt'))}
(ROOT/'catalog/sources/public-ip-models.json').write_text(json.dumps(models,indent=2)+'\n')

import subprocess
selectors=['// Code generated by internal/publicipcontractgen; DO NOT EDIT.','package publicips','']
for kind in ('Field','Filter'):
    values={wire:prop['x-go-field-name'] for wire,prop in definitions['FloatingIP']['properties'].items()}
    if kind=='Filter':values.update({'tags-any':'TagsAny','not-tags':'NotTags','not-tags-any':'NotTagsAny'})
    typename='FloatingIP'+kind
    selectors += ['type '+typename+' string','const (']+[typename+name+' '+typename+' = '+json.dumps(wire) for wire,name in sorted(values.items())]+[')']
    selectors+=['func validFloatingIP'+kind+'(v '+typename+')bool{switch v{','case '+', '.join(typename+name for wire,name in sorted(values.items()))+':return true','};return false}']
(ROOT/'publicips/selectors_generated.go').write_text('\n'.join(selectors)+'\n')
subprocess.run(['gofmt','-w',str(ROOT/'publicips/selectors_generated.go')],check=True)

files=['client.go','generate.go','internal/core/request.go','internal/modelgen/generate.py','internal/publicipcontractgen/generate.py']+[str(p.relative_to(ROOT)) for p in sorted((ROOT/'publicips').glob('*.go')) if not p.name.endswith('_test.go')]
evidence=['publicips/public_test.go','publicips/allocation_test.go','publicips/floating_test.go','publicips/catalog_test.go','internal/core/request_test.go','pagination/pagination_test.go']
bindings_path=ROOT/'catalog/bindings.json';bindings=json.loads(bindings_path.read_text())
operations=[]
for kind,service,generation,prefix,typename in [('allocation','projects','v2','/v2/floatingips','Allocation'),('floating','network','v2','/v2.0/floatingips','FloatingIP'),('direct','public_network','v1','/v1/public_ports','DirectPort')]:
    for action,method in [('list','GET'),('get','GET'),('create','POST'),('update','PATCH' if kind=='direct' else 'PUT'),('delete','DELETE')]:
        if kind=='allocation' and action=='update':continue
        op=service+'.'+generation+'.'+('direct_ports' if kind=='direct' else 'floatingips')+'.'+action
        if kind=='allocation' and action=='create':op='projects.v2.floatingips_projects.create'
        path=prefix+('/projects/{project_id}' if kind=='allocation' and action=='create' else '/{resource_id}' if action in ('get','update','delete') else '')
        go_class='Allocations' if kind=='allocation' else 'FloatingIPs' if kind=='floating' else 'DirectPorts'
        schema=('sources/public-network.json#/components/schemas/'+('PortCreateRequest' if action=='create' else 'PortUpdateRequest' if action=='update' else 'PublicPort') if kind=='direct' else 'sources/public-ip-models.json#/definitions/'+(typename+'Spec' if kind=='allocation' and action=='create' else typename+'CreateSpec' if action=='create' else typename+'UpdateSpec' if action=='update' else typename))
        contract={
            'operation_id':op,'effect':'read' if method=='GET' else 'mutation',
            'source':{'primary':'https://docs.selectel.ru/api/cloud-public-network.json' if kind=='direct' else 'sources/cloud-projects-and-resources.json' if kind=='allocation' else 'pinned Gophercloud v2.13.0 layer3/floatingips + vpc-go460f09b + neutron-lib89dc650', 'snapshots':'sources/public-ip-models.json#x-upstream-assets'},
            'endpoint':{'generation':generation,'source':'fixed https://api.selectel.ru/vpc/resell or exact override' if kind=='allocation' else 'exact project IAM catalog public '+('network' if kind=='floating' else 'public-net-api')+' endpoint for explicit region/pool, or exact override','redirects':'reject'},
            'auth':{'scope':'account view; explicit project view allowed for Get/Create/Delete only' if kind=='allocation' else 'explicit project/location IAM view','hidden_fallback':False},
            'request':{'method':method,'path':path,'schema':schema,'DTO':'publicips/types_generated.go','optional':'nil omitted, false/empty preserved; nullable wrappers distinguish null/value/absence','identity':'exact scoped service/generation/kind/endpoint refs; no implicit cross-API conversion or name lookup','constraints':'quantity positive int32, explicit target project; project auth cannot target another project' if kind=='allocation' else 'typed external network/optional port refs; association port must be same auth view and Neutron kind; owner aliases agree; revision condition optional on update' if kind=='floating' else 'current API body omits legacy project_id; description nullable <=5000 chars; security groups <=20 and PATCH explicit list requires >=1; no implicit legacy force parameter'},
            'response':{'success':[204 if action=='delete' else 201 if action=='create' and kind!='allocation' else 200],'mapping':'complete source-bound typed model; provider ref field discarded; identity extracted before other-field validation','owner':'source project is metadata in account and Neutron reads; Neutron Ref.Scope is caller auth view; direct read owner remains metadata; Create checks known target owner; native explicit project filter is checked separately','timestamps':'direct RFC3339 normalized UTC; Neutron and Resell provider strings kept raw without invented offset','allocation_create_union':'primary floatingip object or published SDK floatingips array; reject both/missing/duplicate IDs; cardinality shortfall is Incomplete with Accepted receipt and every trustworthy ID' if kind=='allocation' and action=='create' else None},
            'errors':{'400':'provider_error','401':'unauthenticated','403':'permission_denied','404':'not_found','409':'conflict','429':'rate_limited','numeric_code':'public-network allowlisted integer code retained exactly on read/rejected errors; UnknownOutcome retains shared CauseCode semantics; msg discarded' if kind=='direct' else 'no undocumented provider-text classification','mutation_outcome':'documented rejection statuses retain Rejected receipt; ambiguous transmission/5xx are UnknownOutcome; never replay','malformed_success':'Accepted receipt and trustworthy IDs retained; protocol_error/scope_mismatch/incomplete as appropriate'},
            'retry':'bounded safe reads; mutations single attempt',
            'pagination':{'shape':'marker/limit, repeated typed filters and projection fields, sort pairs and page_reverse; end only no next/previous link' if kind=='floating' else 'documented complete provider array; local bounded replay/skip only, no invented remote pagination','cursor':'exact scope/endpoint/generation/filters/contract revision','limits':'MaxItems, MaxPages, PageSize, CollectionTimeout and MaxJSONBytes','consistency':'best_effort'} if action=='list' else None,
            'wait':{'availability':'unavailable; acknowledgement is not dataplane readiness'},
            'recovery':{'availability':'root Resource/Resume unavailable; request parent/association refs are context, not proof of created identity; no replay, name lookup or adoption of newly appeared resources'},
            'excluded_variants':['arbitrary foreign plugin/raw fields','Neutron tag/QoS/port-forwarding mutation extensions','implicit allocation-to-Neutron/direct-to-Neutron ref conversion'],
            'source_inventory_complete':False,
        }
        if kind=='direct':contract['source_drift']={'legacy_project_id':'SDK1.0.1 emits it; current schema uses project token; target scope remains explicit','legacy_force':'SDK1.0.1 supports force=true but current schema omits it and documents attached-port409; Current server support not live-verified; explicit DeleteNative implements published SDK serialization without fallback.'}
        if kind=='allocation' and action=='create':
            contract['native_request_variant']={'public_method':'publicips.Allocations.CreateNative','input_type':'publicips.AllocationNativeCreateRequest','source':'https://github.com/selectel/go-selvpcclient/blob/79b0d0ff1d46ebfe7c50056f8c829e0d32349605/selvpcclient/resell/v2/floatingips/requests.go','wire':'one floatingips entry with region only, no inferred quantity; account/project explicit auth view retained','response':'return every trustworthy ID; provider-default cardinality is not inferred, empty/duplicate/malformed results are protocol_error','availability':'published SDK wire contract; current server support not live verified','retry':'single attempt, no fallback or replay','receipt':'same canonical operation; preserved on errors'}
        if kind=='direct' and action in ('create','update','delete','list'):
            contract['native_request_variant']={'public_method':'publicips.DirectPorts.'+('CreateNative' if action=='create' else 'UpdateNative' if action=='update' else 'ListPageNative' if action=='list' else 'DeleteNative'),'input_type':'publicips.'+('DirectPortNativeCreateSpec' if action=='create' else 'DirectPortNativeUpdateRequest' if action=='update' else 'DirectPortNativeListOptions' if action=='list' else 'DirectPortNativeDeleteRequest'),'source':'https://github.com/selectel/public-net-api-go/blob/75bee6c707a155bd1ff0514a35fe58c7cbbca7f6/pkg/v1/port.go','wire':'same endpoint/auth/statuses, explicit project_id body and always-present security_group_ids (nil=>null)' if action=='create' else 'same endpoint/auth/statuses; native security_group_ids always present (nil=>null), description nil omitted' if action=='update' else 'same endpoint/auth/statuses; nonempty ProjectID/NetworkID/Description query filters, empty omitted exactly as SDK; no auth switching' if action=='list' else 'same endpoint/auth/statuses; Force true => query force=true, false => omission','availability':'published SDK request contract only; current server support not live-verified','retry':'single attempt; no fallback to/from current API variant','receipt':'same canonical operation/kind, retained on errors; unknown must not be replayed'}
        (ROOT/'catalog/contracts'/(op+'.json')).write_text(json.dumps(contract,indent=2)+'\n')
        method_name='ListPage' if action=='list' else action.title()
        status='implemented'
        bindings[op]={'go_method':'publicips.'+go_class+'.'+method_name,'input_type':'publicips.'+typename+'ListOptions, pagination.Options' if action=='list' else 'types.ResourceRef, publicips.FloatingIPGetOptions, types.CallOptions' if kind=='floating' and action=='get' else 'types.ResourceRef, types.CallOptions' if action in ('get','delete') else 'publicips.'+(typename+'CreateSpec' if kind=='direct' and action=='create' else typename+'CreateRequest' if action=='create' else typename+'UpdateRequest')+', types.CallOptions','output_type':'pagination.Page[publicips.'+typename+']' if action=='list' else 'types.Result[[]publicips.Allocation]' if kind=='allocation' and action=='create' else 'types.Result[struct{}]' if action=='delete' else 'types.Result[publicips.'+typename+']','contract_ref':'contracts/'+op+'.json','status':status,'files':files,'evidence_files':evidence,'evidence_ref':'publicips/public_test.go','verification':'offline'}
        operations.append(op)
quota='public_network.v1.quotas.get'
contract={'operation_id':quota,'source':'sources/public-network.json#/paths/~1v1~1projects~1{project_id}~1quotas/get','endpoint':'exact project public-net-api catalog endpoint or override','auth':'IAM project with explicit location','request':{'method':'GET','path':'/v1/projects/{project_id}/quotas','project_id':'immutable auth view project only'},'response':{'status':200,'DTO':'publicips.DirectQuotas','numeric':'exact arbitrary-precision Integer, optional source counters remain absent'},'errors':{'400':'provider_error','401':'unauthenticated','403':'permission_denied','404':'not_found','code':'allowlisted exact numeric provider code, msg discarded'},'retry':'bounded safe read','pagination':None,'wait':None,'recovery':None}
(ROOT/'catalog/contracts'/(quota+'.json')).write_text(json.dumps(contract,indent=2)+'\n')
bindings[quota]={'go_method':'publicips.DirectPorts.Quotas','input_type':'types.CallOptions','output_type':'types.Result[publicips.DirectQuotas]','contract_ref':'contracts/'+quota+'.json','status':'implemented','files':files,'evidence_files':evidence,'evidence_ref':'publicips/public_test.go','verification':'offline'}
bindings_path.write_text(json.dumps(bindings,indent=2)+'\n')
