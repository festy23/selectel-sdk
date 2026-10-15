#!/usr/bin/env python3
"""Generate source-bound Private DNS models and compiled contracts offline."""
import json,hashlib,subprocess
from pathlib import Path
R=Path(__file__).resolve().parents[2];A=Path(__file__).parent/'assets';raw=(A/'private-dns.json').read_bytes();rev=hashlib.sha256(raw).hexdigest();j=json.loads(raw);D=j['components']['schemas']
N={'serviceItem':'Service','serviceDetailedItem':'ServiceDetails','addressItem':'Address','serviceCreateBody':'ServiceCreate','serviceUpdateBody':'ServiceUpdate','zoneItem':'Zone','zoneDetailedItem':'ZoneDetails','zoneBindingItem':'ZoneBinding','zoneCreateBody':'ZoneCreate','zoneUpdateBody':'ZoneUpdate','recordSetItem':'RecordSet','recordSetAddBody':'RecordSetAdd','recordSetDeleteBody':'RecordSetDelete','recordSetHardUpdateBody':'RecordSetReplace','recordSetSoftUpdateBody':'RecordSetPatch'}
def nm(k):return ''.join({'id':'ID','ttl':'TTL','cidr':'CIDR'}.get(w,w.title()) for w in k.split('_'))
def resolve(s):
 while '$ref' in s:s=D[s['$ref'].split('/')[-1]]
 return s
def props(s):
 if 'allOf' in s:
  p={}
  for x in s['allOf']:p.update(props(resolve(x)))
  return p
 return s['properties']
def gt(s,key):
 if '$ref' in s and s['$ref'].split('/')[-1] in N:return N[s['$ref'].split('/')[-1]]
 s=resolve(s)
 if s['type']=='array':return '[]types.ProtectedValue' if key in ['values','add','delete'] and resolve(s['items'])['type']=='string' else '[]'+gt(s['items'],'')
 if s['type']=='object':return 'RecordPatchItem'
 return {'string':'string','integer':'types.Integer','boolean':'bool'}[s['type']]
D['recordPatchItem']=D['recordSetSoftUpdateBody']['properties']['records']['items'];N['recordPatchItem']='RecordPatchItem'
out=['// Code generated from Private DNS OpenAPI SHA256 '+rev+'; DO NOT EDIT.','package privatedns','import("encoding/json";"github.com/festy23/selectel-sdk/types")']
for old,new in N.items():
 p=props(D[old]);req=old.endswith('Body') or old=='recordPatchItem';fields=[]
 for k,s in p.items():
  typ=gt(s,k);required=k in D[old].get('required',[])
  if not required and (req or not typ.startswith('[]')):typ='*'+typ
  fields.append(nm(k)+' '+typ+' `json:"'+k+(',omitempty' if not required else '')+'"`')
 if old in ['serviceItem','zoneItem','serviceDetailedItem','zoneDetailedItem']:fields.append('Ref *types.ResourceRef `json:"ref,omitempty"`')
 out.append('type '+new+' struct{'+ ';'.join(fields)+'}')
 if not req and 'values' in p:
  out.append('func(v *'+new+')UnmarshalJSON(raw []byte)error{type plain '+new+'; var x struct{Type *string `json:"type"`;Domain *string `json:"domain"`;TTL *types.Integer `json:"ttl"`;Generated *bool `json:"generated"`;Values []string `json:"values"`};if json.Unmarshal(raw,&x)!=nil{return types.NewError(types.ProtocolError)};v.Type=x.Type;v.Domain=x.Domain;v.TTL=x.TTL;v.Generated=x.Generated;v.Values=nil;for _,s:=range x.Values{v.Values=append(v.Values,types.Protect([]byte(s)))};return nil}')
 if req:
  lines=['m:=map[string]any{}']
  for k,s in p.items():
   t=gt(s,k);required=k in D[old].get('required',[]);expr='v.'+nm(k);value=expr if required else '*'+expr
   if t=='[]types.ProtectedValue':value='wireValues('+value+')'
   elif t.startswith('[]') and t[2:] in N.values():value='wireSlice('+value+')'
   lines.append(('if '+expr+'!=nil{' if not required else '')+'m['+json.dumps(k)+']='+value+('}' if not required else ''))
  out.append('func(v '+new+')wire()map[string]any{'+ ';'.join(lines)+';return m}')
out.append('func wireSlice[T interface{wire()map[string]any}](v []T)[]map[string]any{o:=make([]map[string]any,len(v));for i:=range v{o[i]=v[i].wire()};return o}\nfunc wireValues(v []types.ProtectedValue)[]string{o:=make([]string,len(v));for i:=range v{o[i]=string(v[i].Reveal())};return o}')
out.append('var recordTypes=map[string]bool{'+''.join(json.dumps(v)+':true,' for v in D['recordTypeField']['enum'])+'}')
(R/'privatedns/types_generated.go').write_text('\n'.join(out)+'\n');subprocess.run(['gofmt','-w',str(R/'privatedns/types_generated.go')],check=True)
ops=[('post','/services','services.create','Services.Create','ServiceCreate','ServiceDetails'),('get','/services/{service_id}','services.get','Services.Get','types.ResourceRef','ServiceDetails'),('put','/services/{service_id}','services.update','Services.Update','ServiceUpdate','Empty'),('delete','/services/{service_id}','services.delete','Services.Delete','types.ResourceRef','Empty'),('post','/zones','zones.create','Zones.Create','ZoneCreate','ZoneDetails')]
ops += [('get','/zones/{zone_id}','zones.get','Zones.Get','types.ResourceRef','ZoneDetails'),('put','/zones/{zone_id}','zones.update','Zones.Update','ZoneUpdate','ZoneDetails'),('delete','/zones/{zone_id}','zones.delete','Zones.Delete','types.ResourceRef','Empty'),('put','/zones/{zone_id}/recordset','records.replace','Records.Replace','RecordSetReplace','[]RecordSet'),('patch','/zones/{zone_id}/recordset','records.patch','Records.PatchValues','RecordSetPatch','[]RecordSet')]
ops += [('get','/services','services.list','Services.ListPage','ServiceListOptions','Service'),('get','/zones','zones.list','Zones.ListPage','ZoneListOptions','Zone'),('get','/zones/{zone_id}/recordset','records.list','Records.ListPage','RecordListOptions','RecordSet')]
b={k:v for k,v in json.loads((R/'catalog/bindings.json').read_text()).items() if not k.startswith('private_dns.v1.')};over=[]
files=['client.go','generate.go','privatedns/client.go','privatedns/wait.go','privatedns/list.go','privatedns/constraints_generated.go','privatedns/validation.go','privatedns/types_generated.go','internal/privatednscontractgen/generate.py']
files+=sorted(str(p.relative_to(R)) for d in ['internal/core','internal/endpoints','types','credentials','operations','pagination'] for p in (R/d).glob('*.go') if not p.name.endswith('_test.go'))
for m,p,suffix,method,inp,outp in ops:
 id='private_dns.v1.'+suffix;o=j['paths'][p][m];over.append(json.dumps('private-dns '+m.upper()+' '+p)+':'+json.dumps(id)+',')
 c={'operation_id':id,'source_revision':rev,'endpoint':'regional URL table private-dns/v1; exact explicit override','auth':'iam_project X-Auth-Token only','request':{'method':m.upper(),'path':p,'schema':o.get('requestBody'),'parameters':o.get('parameters',[]),'scope':'caller project/location authorization view, response project owner metadata'},'response':{'source':o['responses'],'mapping':'source generated DTO; exact integers; protected record values; identity decoded before unrelated fields'},'errors':{'400':'invalid_argument','401':'unauthenticated','403':'permission_denied','404':'not_found','provider_text':'discarded; no text classification'},'retry':'bounded safe reads; mutations no replay','recovery':'exact resource observation only; never name lookup/adoption; known outcomes conclusive','wait':'existence/absence only; no DNS propagation claim','pagination':'source complete single response, SDK bounded fixed-page collection'}
 (R/'catalog/contracts'/f'{id}.json').write_text(json.dumps(c,indent=2)+'\n')
 input_type=inp if inp.startswith('types.') else 'privatedns.'+inp
 if m in ['put','patch'] or suffix.startswith('records.'):input_type='types.ResourceRef, '+input_type
 if suffix.endswith('list'):input_type+=', pagination.Options'
 output_type='[]privatedns.'+outp[2:] if outp.startswith('[]') else 'privatedns.'+outp
 if suffix.endswith('list'):output_type='pagination.Page['+output_type+']'
 c['response']['sensitive_fields']=['records[].values[]'] if suffix.startswith(('records.','zones.')) else []
 c['request']['native_read_filter_variant']='Records list type/domain query confirmed in pinned SDK; current OpenAPI has no query parameters' if suffix=='records.list' else None
 c['shared_provenance']='reviewed379458e receipt-only recovery and revision-before-validation; reviewed095faea diagnostic numericcode decoder retains status refresh/retry semantics; no imported service feature code'
 (R/'catalog/contracts'/f'{id}.json').write_text(json.dumps(c,indent=2)+'\n')
 b[id]={'go_method':'privatedns.'+method,'input_type':input_type,'output_type':output_type,'contract_ref':f'contracts/{id}.json','status':'implemented','files':files,'evidence_files':['privatedns/public_test.go'],'evidence_ref':'privatedns/public_test.go','covered_schema_ref':f'contracts/{id}.json#/request','excluded_variants':[],'recovery':m!='get','wait_goals':[suffix.split('.')[0][:-1]+('.present' if suffix.endswith('create') else '.absent')] if suffix.endswith(('create','delete')) else []}
rejected={suffix:[int(v) for v in j['paths'][p][m]['responses'] if v.isdigit() and 400<=int(v)<500] for m,p,suffix,*_ in ops}
cs='// Code generated from request schemas; DO NOT EDIT.\npackage privatedns\nvar rejectedStatuses=map[string][]int{'+''.join(json.dumps(k)+':{'+','.join(map(str,v))+'},' for k,v in rejected.items())+'}\nconst('+ ';'.join(k+'='+str(v) for k,v in {'maxZoneName':D['zoneNameField']['maxLength'],'maxZoneDomain':D['zoneDomainField']['maxLength'],'maxRecordDomain':D['recordDomainField']['maxLength'],'maxCreateRecords':D['zoneCreateBody']['properties']['records']['maxItems'],'maxReplace':D['recordSetHardUpdateBody']['properties']['set']['maxItems'],'maxDelete':D['recordSetHardUpdateBody']['properties']['delete']['maxItems'],'maxValues':D['recordValuesField']['maxItems']}.items())+')\n'
(R/'privatedns/constraints_generated.go').write_text(cs);subprocess.run(['gofmt','-w',str(R/'privatedns/constraints_generated.go')],check=True)
source=dict(j);source['x-source-uri']='https://docs.selectel.ru/api/private-dns.json';source['x-source-revision']=rev;source['x-local-snapshots']={p.name:hashlib.sha256(p.read_bytes()).hexdigest() for p in sorted(A.iterdir()) if p.is_file()};(R/'catalog/sources/private-dns.json').write_text(json.dumps(source,indent=2)+'\n');(R/'catalog/bindings.json').write_text(json.dumps(b,indent=2)+'\n')
(R/'internal/cataloggen/privatedns_generated.go').write_text('// Code generated by privatednscontractgen; DO NOT EDIT.\npackage main\nfunc init(){families["private-dns"]=family{"private_dns","v1","iam_project",false};for k,v:=range map[string]string{'+''.join(over)+'}{overrides[k]=v}}\n');subprocess.run(['gofmt','-w',str(R/'internal/cataloggen/privatedns_generated.go')],check=True)
# PTR is a separate account/static-token API, never a Private DNS endpoint.
praw=(A/'ip-addresses.json').read_bytes();pr=hashlib.sha256(praw).hexdigest();pj=json.loads(praw)
assert set(pj['components']['schemas']['RecordPTRCreate']['properties'])=={'ip','content','ttl'}
assert set(pj['components']['schemas']['RecordPTRUpdate']['properties'])=={'ip','content'}
assert set(pj['components']['schemas']['RecordPTRRead']['required'])=={'id','content','ip','user_id','ttl'}
(R/'ptr/types_generated.go').write_text('''// Code generated from PTR OpenAPI SHA256 '''+pr+'''; DO NOT EDIT.
package ptr
import("encoding/json";"github.com/festy23/selectel-sdk/types")
type Record struct{ID types.Integer `json:"id"`;Content types.ProtectedValue `json:"content"`;IP string `json:"ip"`;UserID types.Integer `json:"user_id"`;TTL types.Integer `json:"ttl"`;Ref *types.ResourceRef `json:"ref,omitempty"`}
type CreateRequest struct{IP string `json:"ip"`;Content *types.ProtectedValue `json:"content,omitempty"`;TTL *types.Integer `json:"ttl,omitempty"`}
type UpdateRequest struct{IP *string `json:"ip,omitempty"`;Content *types.ProtectedValue `json:"content,omitempty"`}
func(v *Record)UnmarshalJSON(raw []byte)error{var x struct{ID *types.Integer `json:"id"`;Content *string `json:"content"`;IP *string `json:"ip"`;UserID *types.Integer `json:"user_id"`;TTL *types.Integer `json:"ttl"`};if json.Unmarshal(raw,&x)!=nil||x.ID==nil||x.Content==nil||x.IP==nil||x.UserID==nil||x.TTL==nil{return types.NewError(types.ProtocolError)};v.ID=*x.ID;v.Content=types.Protect([]byte(*x.Content));v.IP=*x.IP;v.UserID=*x.UserID;v.TTL=*x.TTL;return nil}
''');subprocess.run(['gofmt','-w',str(R/'ptr/types_generated.go')],check=True)
pops=[('post','/','create','Create','CreateRequest','Record'),('get','/{ptr_id}','get','Get','types.ResourceRef','Record'),('put','/{ptr_id}','update','Update','UpdateRequest','Record'),('delete','/{ptr_id}','delete','Delete','types.ResourceRef','Empty')]
pops.append(('get','/','list','ListPage','ListOptions','Record'))
b={k:v for k,v in json.loads((R/'catalog/bindings.json').read_text()).items() if not k.startswith('ip_addresses.v1.')};over=[]
for m,p,act,method,inp,outp in pops:
 id='ip_addresses.v1.ptrs.'+act;o=pj['paths'][p][m];over.append(json.dumps('ip-addresses '+m.upper()+' '+p)+':'+json.dumps(id)+',')
 c={'operation_id':id,'source_revision':pr,'endpoint':'https://api.selectel.ru/ipam/v1 from URL table direct link to ip-addresses; schema-relative paths / and /{ptr_id}','auth':'account static_token X-Token only; no IAM fallback','request':{'method':m.upper(),'path':p,'schema':o.get('requestBody'),'parameters':o.get('parameters',[])},'response':{'source':o['responses'],'mapping':'exact integer IDs/user_id/TTL; protected content; identity-first accepted receipt'},'errors':{'400':'invalid_argument','401':'unauthenticated','404':'not_found','409':'conflict','422':'invalid_argument','500':'provider_error read/unknown sent mutation','provider_text':'discarded'},'retry':'safe bounded reads only; no mutation replay','recovery':'exact numeric provider ID only; no IP/name search or adoption','pagination':'offset advances by observed item count until empty provider page; bounded prefixes; count-only response unknown','wait':'existence/absence only, no reverse-DNS propagation claim'}
 (R/'catalog/contracts'/f'{id}.json').write_text(json.dumps(c,indent=2)+'\n')
 input_type=inp if inp.startswith('types.') else 'ptr.'+inp
 if act=='update':input_type='types.ResourceRef, '+input_type
 if act=='list':input_type+=', pagination.Options'
 output_type='pagination.Page[ptr.Record]' if act=='list' else 'ptr.'+outp
 c['response']['sensitive_fields']=['content']
 (R/'catalog/contracts'/f'{id}.json').write_text(json.dumps(c,indent=2)+'\n')
 b[id]={'go_method':'ptr.Client.'+method,'input_type':input_type,'output_type':output_type,'contract_ref':f'contracts/{id}.json','status':'partial' if act=='list' else 'implemented','files':sorted(set(files+['ptr/client.go','ptr/list.go','ptr/wait.go','ptr/constraints_generated.go','ptr/types_generated.go'])),'evidence_files':['ptr/public_test.go'],'evidence_ref':'ptr/public_test.go','covered_schema_ref':f'contracts/{id}.json#/request','excluded_variants':['only-count=true: description says total count but schema declares RecordPTRRead array; count response shape unknown'] if act=='list' else [],'recovery':m!='get','wait_goals':['ptr.present' if act=='create' else 'ptr.absent'] if act in ['create','delete'] else []}
prc=pj['components']['schemas']['RecordPTRCreate']['properties']
rej={act:[int(v) for v in pj['paths'][p][m]['responses'] if v.isdigit() and 400<=int(v)<500] for m,p,act,*_ in pops}
text='// Code generated from PTR request schema; DO NOT EDIT.\npackage ptr\nconst('+ ';'.join(k+'='+str(v) for k,v in {'minContent':prc['content']['minLength'],'maxContent':prc['content']['maxLength'],'minTTL':prc['ttl']['minimum'],'maxTTL':prc['ttl']['maximum']}.items())+')\nvar rejectedStatuses=map[string][]int{'+''.join(json.dumps(k)+':{'+','.join(map(str,v))+'},' for k,v in rej.items())+'}\n'
(R/'ptr/constraints_generated.go').write_text(text);subprocess.run(['gofmt','-w',str(R/'ptr/constraints_generated.go')],check=True)
ps=dict(pj);ps['x-source-uri']='https://docs.selectel.ru/api/ip-addresses.json';ps['x-source-revision']=pr;(R/'catalog/sources/ip-addresses.json').write_text(json.dumps(ps,indent=2)+'\n');(R/'catalog/bindings.json').write_text(json.dumps(b,indent=2)+'\n')
p=R/'internal/cataloggen/ptr_generated.go';p.write_text('// Code generated by privatednscontractgen; DO NOT EDIT.\npackage main\nfunc init(){families["ip-addresses"]=family{"ip_addresses","v1","static_token",false};for k,v:=range map[string]string{'+''.join(over)+'}{overrides[k]=v}}\n');subprocess.run(['gofmt','-w',str(p)],check=True)
