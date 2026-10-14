#!/usr/bin/env python3
"""Generate current DNS DTOs and bindings from a pinned public OpenAPI snapshot."""
import json,hashlib,subprocess
from pathlib import Path
R=Path(__file__).resolve().parents[2];A=Path(__file__).parent/'assets';raw=(A/'dns-actual.json').read_bytes();rev=hashlib.sha256(raw).hexdigest();j=json.loads(raw);defs=j['components']['schemas']
names={'CreateZoneForm':'ZoneCreate','ZoneResponse':'Zone','RRSetCreateForm':'RRSetCreate','RRSetSchemaResponse':'RRSet','RRSetUpdateForm':'RRSetUpdate','ZonesSortingOptions':'ZonesSortingOptions','RRSetSortingOptions':'RRSetSortingOptions'}
def name(k):return ''.join({'id':'ID','ttl':'TTL','rrset':'RRSet'}.get(w,w.title()) for w in k.split('_'))
def nullable(s):return 'anyOf' in s and any(x.get('type')=='null' for x in s['anyOf'])
def resolve(s):return next((x for x in s.get('anyOf',[]) if x.get('type')!='null'),s)
pending=list(names);emitted={}
def typ(s):
 s=resolve(s)
 if '$ref' in s:
  old=s['$ref'].split('/')[-1];pending.append(old);return names.get(old,old)
 return {'string':'string','integer':'types.Integer','boolean':'bool','array':'[]'+typ(s['items']) if 'items' in s else ''}[s['type']]
while pending:
 old=pending.pop(0)
 if old in emitted:continue
 schema=defs[old];go=names.get(old,old)
 if schema['type']=='string':
  emitted[old]='type '+go+' string\nconst(\n'+'\n'.join(go+(v if v.isupper() else name(v.replace('.','_')))+' '+go+'='+json.dumps(v) for v in schema.get('enum',[]))+'\n)';continue
 req=old.endswith('Form');fs=[]
 for key,s in schema['properties'].items():
  t=typ(s)
  if old=='Records' and key=='content':t='types.ProtectedValue'
  required=key in schema.get('required',[])
  if req and nullable(s):t='*types.Nullable['+t+']'
  elif nullable(s) or not required:t='*'+t
  fs.append(name(key)+' '+t+' `json:"'+key+(',omitempty' if not required else '')+'"`')
 if old in ['ZoneResponse','RRSetSchemaResponse']:fs.append('Ref *types.ResourceRef `json:"ref,omitempty"`')
 emitted[old]='type '+go+' struct{\n'+'\n'.join(fs)+'\n}'
(R/'dns/types_generated.go').write_text('// Code generated from current DNS OpenAPI SHA256 '+rev+'; DO NOT EDIT.\npackage dns\nimport "github.com/festy23/selectel-sdk/types"\n'+('import "encoding/json"\n' if 'Records' in emitted else '')+''+'\n'.join(emitted.values())+('''
func(v *Records)UnmarshalJSON(raw []byte)error{var w struct{Content *string `json:"content"`;Disabled *bool `json:"disabled"`};if json.Unmarshal(raw,&w)!=nil||w.Content==nil{return types.NewError(types.ProtocolError)};v.Content=types.Protect([]byte(*w.Content));v.Disabled=w.Disabled;return nil}
''' if 'Records' in emitted else ''))
subprocess.run(['gofmt','-w',str(R/'dns/types_generated.go')],check=True)
ops=[('POST','/zones','zones.create','Zones.Create','ZoneCreate','Zone'),('POST','/zones/{zone_id}/rrset','rrsets.create','RRSets.Create','RRSetCreate','RRSet')]
ops.extend([('GET','/zones/{zone_id}','zones.get','Zones.Get','types.ResourceRef','Zone'),('DELETE','/zones/{zone_id}','zones.delete','Zones.Delete','types.ResourceRef','Empty'),('PATCH','/zones/{zone_id}','zones.comment','Zones.UpdateComment','string','Empty'),('PATCH','/zones/{zone_id}/state','zones.state','Zones.SetDisabled','bool','Empty'),('PATCH','/zones/{zone_id}/protection','zones.protection','Zones.SetProtected','bool','Empty'),('PUT','/zones/{zone_id}/owner','zones.owner','Zones.TransferOwner','string','Empty')])
ops.extend([('GET','/zones/{zone_id}/rrset/{rrset_id}','rrsets.get','RRSets.Get','types.ResourceRef','RRSet'),('PATCH','/zones/{zone_id}/rrset/{rrset_id}','rrsets.update','RRSets.Update','RRSetUpdate','Empty'),('DELETE','/zones/{zone_id}/rrset/{rrset_id}','rrsets.delete','RRSets.Delete','types.ResourceRef','Empty')])
ops.append(('GET','/zones','zones.list','Zones.ListPage','ZoneListOptions','Zone'))
ops.append(('GET','/zones/{zone_id}/rrset','rrsets.list','RRSets.ListPage','RRSetListOptions','RRSet'))
b={k:v for k,v in json.loads((R/'catalog/bindings.json').read_text()).items() if not k.startswith('dns.v2.')};paths={};overrides=[]
files=['client.go','generate.go','dns/client.go','dns/zones.go','dns/list.go','dns/wait.go','dns/validation.go','dns/constraints_generated.go','dns/rrsets.go','dns/types_generated.go','internal/dnscontractgen/generate.py']
files=sorted(set(files+['go.mod','go.sum','config.go']+[str(p.relative_to(R)) for d in ['internal/core','internal/endpoints','types','credentials','operations','pagination'] for p in (R/d).glob('*.go') if not p.name.endswith('_test.go')]))
for method,path,suffix,go,inp,out in ops:
 id='dns.v2.'+suffix;o=j['paths'][path][method.lower()];paths.setdefault(path,{})[method.lower()]=o
 overrides.append(json.dumps('dns-actual '+method+' '+path)+':'+json.dumps(id)+',')
 c={'operation_id':id,'source_revision':rev,'endpoint':'global https://api.selectel.ru/domains/v2 or exact explicit override; no location','auth':'iam_project X-Auth-Token, no account/static-token fallback','request':{'method':method,'path':path,'schema':o.get('requestBody'),'native_management_variant':{'methods':['RRSets.CreateNative','RRSets.UpdateNative'],'body':'managed_by string, omit empty, no X-Managed-By fallback','source':'pinned published domains-go v2 wire serialization, current server acceptance is unknown'} if suffix in ['rrsets.create','rrsets.update'] else None,'parameters':o.get('parameters',[]),'scope':'caller auth view, provider project_id owner metadata, owner transfer never adopts new scope'},'response':{'source':o['responses'],'sensitive_fields':['records[].content'] if suffix.startswith('rrsets.') else [],'mapping':'source generated DTO, exact Integer TTL/count, optional booleans and raw timestamp strings, validated identity retained independently from malformed unrelated fields'},'errors':{'400':'invalid_argument','401':'unauthenticated','403':'permission_denied','404':'not_found in saved scope','409':'conflict','422':'invalid_argument','500':'provider_error for reads, unknown for sent mutations','provider_text':'discarded; no text-based classification'},'retry':'bounded safe reads; mutations no replay; source-documented4xx rejected only','shared_provenance':'reviewed379458e receipt-only recovery/revision-before-validation; reviewed8351f2f service exact endpoint pinning and identity-first decoding pattern, no unmerged service feature code','recovery':'exact refs/pinned endpoint only; known receipts conclusive without I/O; unknown never inferred by name/adopted','pagination':'source next_offset 0/current or null/pinnedSDK terminates; positive provider offsets strictly advance; count retained as reported_count, no invented total; bounded cursor semantics include endpoint URL and private query digest','wait':'existence/absence observation only, no DNS propagation claim'}
 (R/'catalog/contracts'/f'{id}.json').write_text(json.dumps(c,indent=2)+'\n')
 input_type=inp if inp in ['bool','string'] or inp.startswith('types.') else 'dns.'+inp
 if suffix.startswith('rrsets.') and not suffix.endswith('list'):input_type='types.ResourceRef, '+input_type
 if suffix=='rrsets.update':input_type+=', dns.RRSetOptions'
 if suffix=='rrsets.delete':input_type='types.ResourceRef, dns.RRSetOptions'
 if suffix in ['zones.comment','zones.state','zones.owner','zones.protection']:input_type='types.ResourceRef, '+input_type
 if suffix.endswith('list'):input_type+=', pagination.Options'
 if suffix=='rrsets.list':input_type='types.ResourceRef, '+input_type
 output_type=('pagination.Page[dns.'+out+']') if suffix.endswith('list') else 'dns.'+out
 b[id]={'go_method':'dns.'+go,'input_type':input_type,'output_type':output_type,'contract_ref':f'contracts/{id}.json','status':'implemented','files':files,'evidence_files':['dns/public_test.go','dns/catalog_test.go'],'evidence_ref':'dns/public_test.go;dns/catalog_test.go','covered_schema_ref':f'contracts/{id}.json#/request','excluded_variants':[],'recovery':method!='GET','wait_goals':[(suffix.split('.')[0][:-1] if suffix.startswith('zones.') else 'rrset')+('.present' if suffix.endswith('create') else '.absent')] if suffix.endswith(('create','delete')) else []}
source=dict(j);source['paths']=paths;source['x-source-uri']='https://docs.selectel.ru/api/dns-actual.json';source['x-source-revision']=rev;source['x-local-snapshots']={p.name:hashlib.sha256(p.read_bytes()).hexdigest() for p in sorted(A.iterdir()) if p.is_file()};source['x-sdk-source']='https://github.com/selectel/domains-go/tree/6b87a8d9f7b4089a30a4a1dc361cf73117f1af61';(R/'catalog/sources/dns-actual.json').write_text(json.dumps(source,indent=2)+'\n');(R/'catalog/bindings.json').write_text(json.dumps(b,indent=2)+'\n')
(R/'internal/cataloggen/dns_generated.go').write_text('// Code generated by dnscontractgen; DO NOT EDIT.\npackage main\nfunc init(){for k,v:=range map[string]string{'+''.join(overrides)+'}{overrides[k]=v}}\n');subprocess.run(['gofmt','-w',str(R/'internal/cataloggen/dns_generated.go')],check=True)

form=defs['RRSetCreateForm']['properties'];comment=resolve(form['comment'])
text='// Code generated from current request schema; DO NOT EDIT.\npackage dns\nconst ('+f'minTTL={form["ttl"]["minimum"]};maxTTL={form["ttl"]["maximum"]};minRecords={form["records"]["minItems"]};maxRecords={form["records"]["maxItems"]};maxCommentLength={comment["maxLength"]}'+')\nvar requestRecordTypes=map[AllowedRecordTypes]bool{'+''.join(json.dumps(v)+':true,' for v in defs['AllowedRecordTypes']['enum'])+'}\n'
(R/'dns/constraints_generated.go').write_text(text);subprocess.run(['gofmt','-w',str(R/'dns/constraints_generated.go')],check=True)
