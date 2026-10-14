#!/usr/bin/env python3
"""Pinned Octavia 2023.1 API 2.26 models. Offline generation only."""
import ast,json,re,subprocess,hashlib
from pathlib import Path
R=Path(__file__).resolve().parents[2]; A=Path(__file__).parent/'assets'
REV='4486d2416f600df15df1b5c98a676f898e2b1d42'
classes={}
for p in A.glob('octavia-*.py.txt'):
 for c in ast.parse(p.read_text()).body:
  if isinstance(c,ast.ClassDef):classes[c.name]=c
names={'LoadBalancerPOST':'LoadBalancerCreate','LoadBalancerResponse':'LoadBalancer','ListenerPOST':'ListenerCreate','ListenerResponse':'Listener','PoolPOST':'PoolCreate','PoolResponse':'Pool','MemberPOST':'MemberCreate','MemberResponse':'Member','HealthMonitorPOST':'MonitorCreate','HealthMonitorResponse':'Monitor','LoadBalancerPUT':'LoadBalancerUpdate','ListenerPUT':'ListenerUpdate','PoolPUT':'PoolUpdate','MemberPUT':'MemberUpdate','HealthMonitorPUT':'MonitorUpdate','IdOnlyType':'IDOnly'}
# TLS and L7 request variants belong to #41; no unsupported-API claim.
def excluded(k):return 'tls' in k or k in ['sni_container_refs','l7policies','client_authentication','alpn_protocols','crl_container_ref']
def name(k):return ''.join({'id':'ID','ip':'IP','vip':'VIP','url':'URL','http':'HTTP','tcp':'TCP','qos':'QoS','cidrs':'CIDRs','ca':'CA','crl':'CRL','tls':'TLS','sni':'SNI','alpn':'ALPN'}.get(w,w.title()) for w in k.split('_'))
pending=['LoadBalancerPOST','LoadBalancerResponse','LoadBalancerPUT','ListenerPOST','ListenerResponse','ListenerPUT','PoolPOST','PoolResponse','PoolPUT','HealthMonitorPOST','HealthMonitorResponse','HealthMonitorPUT','MemberPOST','MemberResponse','MemberPUT','StatusResponse','LoadBalancerStatisticsResponse']; emitted={}
def typ(n):
 if isinstance(n,ast.List):return '[]'+typ(n.elts[0])
 s=ast.unparse(n)
 if isinstance(n,ast.Call):
  f=ast.unparse(n.func)
  if f.endswith('Enum'):return 'types.Number' if ast.unparse(n.args[0])=='float' else 'string'
  if f.endswith('ArrayType'):return '[]'+typ(n.args[0])
  if f.endswith('DictType'):return 'map[string]string'
  if 'Integer' in f:return 'types.Integer'
  if 'String' in f or 'Uuid' in f or 'IPAddress' in f or 'URLPath' in f or 'CidrType' in f or 'AlpnProtocol' in f:return 'string'
 if s=='bool':return 'bool'
 if s=='float':return 'types.Number'
 if s in ['wtypes.text','str','wtypes.datetime.datetime']:return 'string'
 if s=='types.IdOnlyType':return 'IDOnly'
 if isinstance(n,ast.Dict):return 'map[string]string'
 t=s.split('.')[-1]
 if t not in classes:raise ValueError(s)
 pending.append(t);return names.get(t,t)
while pending:
 old=pending.pop(0)
 if old in emitted:continue
 c=classes[old];req=old.endswith(('POST','PUT','SingleCreate')) or old=='AdditionalVipsType';fs=[]
 for x in c.body:
  if not isinstance(x,ast.Assign) or not isinstance(x.value,ast.Call) or not ast.unparse(x.value.func).endswith('wsattr'):continue
  k=x.targets[0].id
  if req and (excluded(k) or k in ['loadbalancer_id','pool_id','listener_id']):continue
  t=typ(x.value.args[0])
  if old=='LoadBalancerResponse' and k in ['listeners','pools']:t='[]'+('Listener' if k=='listeners' else 'Pool')
  if old=='PoolResponse' and k=='members':t='[]Member'
  required=any(q.arg=='mandatory' and ast.literal_eval(q.value) for q in x.value.keywords if q.arg=='mandatory')
  if req and (k in ['vip_qos_policy_id','session_persistence'] or old in ['MemberPOST','MemberPUT','MemberSingleCreate'] and k in ['monitor_port','monitor_address','subnet_id']):
   t='*types.Nullable['+t+']'
  elif req and not required:t='*'+t
  elif not req and (t=='types.Integer' or t=='types.Number' or (t not in ['string','bool','IDOnly'] and not t.startswith(('[]','map[')))):t='*'+t
  fs.append(f'{name(k)} {t} `json:"{k}'+(',omitempty' if req and not required else '')+'"`')
 if old=='PoolResponse':fs.append('Healthmonitor *Monitor `json:"healthmonitor,omitempty"`')
 if not req and old.endswith('Response'):fs.append('Ref *types.ResourceRef `json:"ref,omitempty"`')
 emitted[old]='type '+names.get(old,old)+' struct {\n'+'\n'.join(fs)+'\n}'
(R/'loadbalancers/types_generated.go').write_text('// Code generated from Octavia '+REV+'; DO NOT EDIT.\npackage loadbalancers\nimport "github.com/festy23/selectel-sdk/types"\nimport "encoding/json"\ntype IDOnly struct {ID string `json:"id"`}\n'+'\n'.join(emitted.values())+'\n'+'\n'.join('func(v *'+names.get(k,k)+')UnmarshalJSON(raw []byte)error{type wire '+names.get(k,k)+';var w wire;if e:=json.Unmarshal(raw,&w);e!=nil{return e};w.Ref=nil;*v='+names.get(k,k)+'(w);return nil}' for k in emitted if k.endswith('Response')))
subprocess.run(['gofmt','-w',str(R/'loadbalancers/types_generated.go')],check=True)

ops=[('loadbalancers','create','POST','/v2/lbaas/loadbalancers','Client.Create','LoadBalancerCreate','LoadBalancer',[201]),('loadbalancers','get','GET','/v2/lbaas/loadbalancers/{loadbalancer_id}','Client.Get','types.ResourceRef','LoadBalancer',[200]),('loadbalancers','update','PUT','/v2/lbaas/loadbalancers/{loadbalancer_id}','Client.Update','LoadBalancerUpdate','LoadBalancer',[200,202]),('loadbalancers','delete','DELETE','/v2/lbaas/loadbalancers/{loadbalancer_id}','Client.Delete','types.ResourceRef,cascade','Empty',[204])]
for act,method,status in [('create','POST',[201]),('get','GET',[200]),('update','PUT',[200,202]),('delete','DELETE',[204])]:
 ops.append(('listeners',act,method,'/v2/lbaas/listeners'+('' if act=='create' else '/{listener_id}'),'Listeners.'+act.title(),{'create':'ListenerCreate','update':'ListenerUpdate'}.get(act,'types.ResourceRef'),'Empty' if act=='delete' else 'Listener',status))
for act,method,status in [('create','POST',[201]),('get','GET',[200]),('update','PUT',[200,202]),('delete','DELETE',[204])]:
 ops.append(('pools',act,method,'/v2/lbaas/pools'+('' if act=='create' else '/{pool_id}'),'Pools.'+act.title(),{'create':'PoolCreate','update':'PoolUpdate'}.get(act,'types.ResourceRef'),'Empty' if act=='delete' else 'Pool',status))
for act,method,status in [('create','POST',[201]),('get','GET',[200]),('update','PUT',[200,202]),('delete','DELETE',[204])]:
 ops.append(('healthmonitors',act,method,'/v2/lbaas/healthmonitors'+('' if act=='create' else '/{healthmonitor_id}'),'Monitors.'+act.title(),{'create':'MonitorCreate','update':'MonitorUpdate'}.get(act,'types.ResourceRef'),'Empty' if act=='delete' else 'Monitor',status))
for act,method,status in [('create','POST',[201]),('get','GET',[200]),('update','PUT',[200,201,202]),('delete','DELETE',[204])]:
 ops.append(('members',act,method,'/v2/lbaas/pools/{pool_id}/members'+('' if act=='create' else '/{member_id}'),'Members.'+act.title(),{'create':'MemberCreate','update':'MemberUpdate'}.get(act,'types.ResourceRef'),'Empty' if act=='delete' else 'Member',status))
ops.append(('loadbalancers','list','GET','/v2/lbaas/loadbalancers','Client.ListPage','ListOptions','LoadBalancer',[200]))
for resource,go,out in [('listeners','Listeners','Listener'),('pools','Pools','Pool'),('healthmonitors','Monitors','Monitor'),('members','Members','Member')]:
 path='/v2/lbaas/'+resource if resource!='members' else '/v2/lbaas/pools/{pool_id}/members'
 ops.append((resource,'list','GET',path,go+'.ListPage','ListOptions',out,[200]))
ops.extend([('loadbalancers','statistics','GET','/v2/lbaas/loadbalancers/{loadbalancer_id}/stats','Client.Statistics','types.ResourceRef','LoadBalancerStatisticsResponse',[200]),('loadbalancers','statuses','GET','/v2/lbaas/loadbalancers/{loadbalancer_id}/status','Client.StatusTree','types.ResourceRef','StatusResponse',[200]),('listeners','statistics','GET','/v2/lbaas/listeners/{listener_id}/stats','Listeners.Statistics','types.ResourceRef','LoadBalancerStatisticsResponse',[200])])
ops.extend([('loadbalancers','failover','PUT','/v2/lbaas/loadbalancers/{loadbalancer_id}/failover','Client.Failover','types.ResourceRef','Empty',[202]),('members','batch_update','PUT','/v2/lbaas/pools/{pool_id}/members','Members.BatchUpdate','[]MemberCreate,additiveOnly','Empty',[202])])
b={k:v for k,v in json.loads((R/'catalog/bindings.json').read_text()).items() if not k.startswith('loadbalancers.v2.')};paths={};overrides=[]
files=['client.go','generate.go','loadbalancers/client.go','loadbalancers/children.go','loadbalancers/list.go','loadbalancers/diagnostics.go','loadbalancers/validation.go','loadbalancers/lists_generated.go','loadbalancers/fields_generated.go','loadbalancers/types_generated.go','loadbalancers/wait.go','internal/lbcontractgen/generate.py']
files=sorted(set(files+['config.go','go.mod','go.sum']+[str(p.relative_to(R)) for d in ['internal/core','internal/endpoints','types','credentials','operations','pagination'] for p in (R/d).glob('*.go') if not p.name.endswith('_test.go')]))
for resource,action,method,path,go,inp,out,statuses in ops:
 id='loadbalancers.v2.'+resource+'.'+action
 paths.setdefault(path,{})[method.lower()]={'operationId':id,'responses':{str(n):{'description':out} for n in statuses}}
 overrides.append(json.dumps('octavia-lifecycle '+method+' '+path)+':'+json.dumps(id)+',')
 partial=resource in ['loadbalancers','listeners','pools'] and action=='create' or resource in ['listeners','pools'] and action=='update'
 contract={'operation_id':id,'source_revision':REV,'endpoint':'Exact IAM public load-balancer endpoint selected by pool including /v2; no hostname inference','auth':'iam_project','request':{'method':method,'path':path,'DTO':inp,'scope':'caller authorization view; provider project_id remains owner metadata; every exact resource/parent request pins original endpoint ID+URL+scope+generation before target I/O','variants':'TLS/L7 request variants excluded to #41, never unsupported; core optional fields preserve presence; parent IDs injected from exact refs; member batch additive_only explicit, Replace convenience forces false'},'response':{'statuses':statuses,'DTO':out,'mapping':'identity and required parent relationships validated before remaining DTO; trustworthy refs retained on acknowledged malformed payload, invalid/foreign identity never adopted; source-bound generated core/full-inline model; provisioning and operating states independent; precise Integer/Number, nullable member monitoring fields, raw provider timestamp strings; ref from wire ignored'},'errors':{'400':'provider_error','401':'unauthenticated','403':'permission_denied','404':'not_found in caller view','409':'conflict','429':'rate_limited','malformed_success':'protocol_error with accepted receipt retained'},'retry':'bounded safe reads; mutation no replay; documented400/401/403/404/409 rejected; other unexpected statuses including429 remain unknown with sanitized cause','wait':'*.provisioned ACTIVE configuration (not operating ONLINE); *.absent exact404 only; no causal proof of failover or batch members','recovery':'known receipt conclusive without I/O, including no-ID; unknown exact ref only, indeterminate; no replay, name search or adoption','pagination':'limit/marker/page_reverse and strict same-endpoint next links, bounded prefix/cursor and filter/projection selectors; projected missing identity yields no ResourceRef', 'shared_provenance':'receipt-only recovery/revision-before-validation from reviewed379458e; compiled endpoint prefix from reviewedafaa6e9 extended with statically bound /v2.0 alias from pinned Octavia root_controller'}
 (R/'catalog/contracts'/f'{id}.json').write_text(json.dumps(contract,indent=2)+'\n')
 b[id]={'go_method':'loadbalancers.'+go,'input_type':inp,'output_type':'loadbalancers.'+out,'contract_ref':f'contracts/{id}.json','status':'partial' if partial else 'implemented','files':files,'evidence_files':['loadbalancers/public_test.go','loadbalancers/catalog_test.go'],'evidence_ref':'loadbalancers/public_test.go;loadbalancers/catalog_test.go','covered_schema_ref':f'contracts/{id}.json#/request','excluded_variants':['#41 TLS/L7 inline request variants'] if partial else [],'wait_goals':[] if action in ['failover','batch_update','statistics','statuses'] else [(dict(loadbalancers='loadbalancer',listeners='listener',pools='pool',members='member',healthmonitors='healthmonitor')[resource])+('.absent' if action=='delete' else '.provisioned')],'recovery':method!='GET'}
s={'openapi':'3.0.0','info':{'title':'Selectel Octavia lifecycle confirmed subset','version':'2.26'},'x-source-uri':'https://github.com/openstack/octavia/tree/'+REV,'x-source-revision':REV,'x-sdk-revision':'0e61a3a3fc1f72a396a4e86785dd8b633d4c55fb','x-secondary-sources':['https://docs.selectel.ru/en/cloud-servers/tools/openstack-cli/about-openstack-cli/','https://docs.selectel.ru/en/terraform/openstack-provider-reference/lbaas-octavia/resources/openstack_lb_loadbalancer_v2/','https://docs.selectel.ru/en/terraform/openstack-provider-reference/lbaas-octavia/resources/openstack_lb_listener_v2/','https://docs.selectel.ru/en/terraform/openstack-provider-reference/lbaas-octavia/resources/openstack_lb_pool_v2/','https://docs.selectel.ru/en/cloud-servers/load-balancers/about-load-balancers/','https://docs.selectel.ru/en/cloud-servers/load-balancers/manage/failover/','https://github.com/gophercloud/gophercloud/tree/v2.13.0/openstack/loadbalancer/v2'],'x-local-snapshots':{p.name:hashlib.sha256(p.read_bytes()).hexdigest() for p in sorted(A.iterdir())},'paths':paths}
(R/'catalog/sources/octavia-lifecycle.json').write_text(json.dumps(s,indent=2)+'\n');(R/'catalog/bindings.json').write_text(json.dumps(b,indent=2)+'\n')
(R/'internal/cataloggen/octavia_generated.go').write_text('// Code generated by lbcontractgen; DO NOT EDIT.\npackage main\nfunc init(){for k,v:=range map[string]string{'+''.join(overrides)+'}{overrides[k]=v}}\n')
subprocess.run(['gofmt','-w',str(R/'internal/cataloggen/octavia_generated.go')],check=True)

selectors={}
for old,key in [('LoadBalancerResponse','loadbalancers'),('ListenerResponse','listeners'),('PoolResponse','pools'),('MemberResponse','members'),('HealthMonitorResponse','healthmonitors')]:
 fields=[];filters=[]
 for x in classes[old].body:
  if isinstance(x,ast.Assign) and isinstance(x.targets[0],ast.Name) and isinstance(x.value,ast.Call) and ast.unparse(x.value.func).endswith('wsattr'):
   k=x.targets[0].id;fields.append(k)
   filters.append(k)
 sdk={'loadbalancers':'loadbalancers','listeners':'listeners','pools':'pools','members':'pools','healthmonitors':'monitors'}[key]
 body=(A/(sdk+'_requests.go.txt')).read_text()
 block=re.search(r'type '+('ListMembersOpts' if key=='members' else 'ListOpts')+r' struct \{(.*?)\n\}',body,re.S)
 aliases=re.findall(r'q:"([^"]+)"',block[1]) if block else []
 selectors[key]=(fields,filters+['tags','tags-any','not-tags','not-tags-any']+[k for k in aliases if k not in ['limit','marker','sort_key','sort_dir']])
text='// Code generated from pinned Octavia response fields and API-ref query parameters; DO NOT EDIT.\npackage loadbalancers\nvar projections=map[string]map[string]bool{'
for key,(fields,filters) in selectors.items():text+=json.dumps(key)+':{'+''.join(json.dumps(k)+':true,' for k in fields)+'},'
text+='}\nvar filters=map[string]map[string]bool{'
for key,(fields,fs) in selectors.items():text+=json.dumps(key)+':{'+''.join(json.dumps(k)+':true,' for k in dict.fromkeys(fs))+'},'
text+='}\n';(R/'loadbalancers/fields_generated.go').write_text(text);subprocess.run(['gofmt','-w',str(R/'loadbalancers/fields_generated.go')],check=True)

text='// Code generated by lbcontractgen; DO NOT EDIT.\npackage loadbalancers\nimport("context";"github.com/festy23/selectel-sdk/pagination";"github.com/festy23/selectel-sdk/types")\n'
for go,resource,kind,out in [('Listeners','listeners','listener','Listener'),('Pools','pools','pool','Pool'),('Monitors','healthmonitors','healthmonitor','Monitor'),('Members','members','member','Member')]:
 arg='p types.ResourceRef,' if go=='Members' else '';p='&p' if go=='Members' else 'nil';passed='p,' if go=='Members' else ''
 text+=f'func(c *{go})Iterate({arg}f ListOptions,o pagination.Options)(*pagination.Iterator[{out}],error){{return iterate[{out}](c.service,"{resource}","{kind}",{p},f,o)}}\n'
 text+=f'func(c *{go})ListPage(ctx context.Context,{arg}f ListOptions,o pagination.Options)(pagination.Page[{out}],error){{it,e:=c.Iterate({passed}f,o);if e!=nil{{return pagination.Page[{out}]{{}},e}};return it.Next(ctx)}}\n'
 text+=f'func(c *{go})Collect(ctx context.Context,{arg}f ListOptions,o pagination.Options)(pagination.Collection[{out}],error){{it,e:=c.Iterate({passed}f,o);if e!=nil{{return pagination.Collection[{out}]{{Complete:false}},e}};return it.Collect(ctx)}}\n'
(R/'loadbalancers/lists_generated.go').write_text(text);subprocess.run(['gofmt','-w',str(R/'loadbalancers/lists_generated.go')],check=True)
