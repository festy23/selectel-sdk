#!/usr/bin/env python3
"""Build SDK-owned Nova catalog schemas from local pinned official source assets."""
import json
from pathlib import Path
ROOT=Path(__file__).resolve().parents[2]
def prop(kind):return {'type':kind}
def object_(properties,required=()):return {'type':'object','properties':properties,'required':list(required)}
flavor={k:prop(t) for k,t in {'id':'string','name':'string','disk':'integer','ram':'integer','vcpus':'integer','swap':'integer','rxtx_factor':'number','os-flavor-access:is_public':'boolean','OS-FLV-EXT-DATA:ephemeral':'integer','OS-FLV-DISABLED:disabled':'boolean','description':'string'}.items()}
flavor['description']={'type':['string','null']}
flavor['extra_specs']={'type':'object','additionalProperties':prop('string')}
flavor['links']={'type':'array','items':object_({'href':prop('string'),'rel':prop('string')})}
doc={'x-source-uri':'https://github.com/gophercloud/gophercloud/tree/v2.13.0/openstack/compute/v2','x-inventory-kind':'SDK-owned documented flavors/keypairs subset; not a complete Nova API inventory','x-upstream-assets':{p.name:p.read_text() for p in sorted((ROOT/'internal/computecontractgen/assets').glob('*.go.txt'))},'paths':{'/flavors/detail':{'get':{'operationId':'ListDetail','native_request_type':'flavors.ListOpts','native_response_type':'flavors.Flavor[]','responses':{'200':{'schema':{'type':'array','items':{'$ref':'#/definitions/Flavor'}}}}}},'/flavors/{flavor_id}':{'get':{'operationId':'Get','responses':{'200':{'schema':{'$ref':'#/definitions/Flavor'}}}}},'/flavors/{flavor_id}/os-extra_specs':{'get':{'operationId':'ListExtraSpecs','responses':{'200':{'schema':{'$ref':'#/definitions/FlavorExtraSpecs'}}}}}},'definitions':{'Flavor':object_(flavor,['id']),'FlavorExtraSpecs':object_({'extra_specs':{'type':'object','additionalProperties':prop('string')}})}}
doc['definitions']['PersonalityFile']=object_({'path':prop('string'),'contents':prop('string')},['path','contents'])
legacy={k:prop(t) for k,t in {'virtual_name':'string','volume_id':'string','snapshot_id':'string','volume_size':'integer','device_name':'string','delete_on_termination':'boolean','no_device':'object','connection_info':'string'}.items()}
doc['definitions']['LegacyBlockDeviceMapping']=object_(legacy)
v2=dict(legacy);v2.update({k:prop(t) for k,t in {'source_type':'string','uuid':'string','image_id':'string','destination_type':'string','guest_format':'string','device_type':'string','disk_bus':'string','boot_index':'integer','tag':'string','volume_type':'string'}.items()})
doc['definitions']['BlockDeviceMapping']=object_(v2)
doc['definitions']['NetworkAttachment']=object_({k:prop('string') for k in ['port','uuid','fixed_ip','tag']})
keypair={k:prop(t) for k,t in {'name':'string','fingerprint':'string','public_key':'string','private_key':'string','user_id':'string','type':'string','id':'integer','deleted':'boolean','created_at':'string','deleted_at':'string','updated_at':'string'}.items()}
doc['definitions']['KeyPair']=object_(keypair,['name'])
for path,methods in {'/os-keypairs':['get','post'],'/os-keypairs/{keypair_name}':['get','delete']}.items():
 doc['paths'][path]={m:{'operationId':{'get':'List' if path=='/os-keypairs' else 'Get','post':'Create','delete':'Delete'}[m],'native_request_type':'keypairs options','native_response_type':'keypairs.KeyPair'} for m in methods}
doc['x-create-schema']={'uri':'https://github.com/openstack/nova/blob/3d1a705fc85a0f0a46e7cbc322e98fff892e21b8/nova/api/openstack/compute/schemas/servers.py','revision':'3d1a705fc85a0f0a46e7cbc322e98fff892e21b8','asset':'nova-servers-schema.py.txt'}
doc['x-selectel-microversions']={'source_uri':'https://docs.selectel.ru/cloud-servers/tools/openstack-cli/about-openstack-cli/','release':'Zed','maximum':'2.93','checked_at':'2026-10-14'}
for asset in sorted((ROOT/'internal/computecontractgen/assets').glob('*.py.txt')):doc['x-upstream-assets'][asset.name]=asset.read_text()
doc['x-upstream-asset-sources']={}
for service in ['flavors','keypairs']:
 for filename in ['requests.go','results.go','urls.go']:
  doc['x-upstream-asset-sources'][service+'-'+filename+'.txt']='https://github.com/gophercloud/gophercloud/blob/v2.13.0/openstack/compute/v2/'+service+'/'+filename
nova_assets={'nova-servers-schema.py.txt':'nova/api/openstack/compute/schemas/servers.py','nova-keypairs-schema.py.txt':'nova/api/openstack/compute/schemas/keypairs.py','nova-parameter-types.py.txt':'nova/api/validation/parameter_types.py','nova-keypairs-controller.py.txt':'nova/api/openstack/compute/keypairs.py'}
for asset,path in nova_assets.items():doc['x-upstream-asset-sources'][asset]='https://github.com/openstack/nova/blob/3d1a705fc85a0f0a46e7cbc322e98fff892e21b8/'+path
(ROOT/'catalog/sources/compute-catalog.json').write_text(json.dumps(doc,indent=2)+'\n')

nova_path=ROOT/'catalog/sources/compute-nova.json'
nova=json.loads(nova_path.read_text())
nova['x-selectel-microversions']=doc['x-selectel-microversions']
nova.setdefault('x-upstream-asset-sources',{}).update({asset:doc['x-upstream-asset-sources'][asset] for asset in nova_assets})
for asset in sorted((ROOT/'internal/computecontractgen/assets').glob('*.py.txt')):nova['x-upstream-assets'][asset.name]=asset.read_text()
nova['paths']['/servers']['post'].update({'native_request_type':'compute.CreateRequest (protected/native options + source-bound extension DTOs)','native_response_type':'compute.CreateResult (server or reservation)','source_schema_revision':'3d1a705fc85a0f0a46e7cbc322e98fff892e21b8','supported_microversions':['2.1','2.93'],'request_fields':['name','imageRef','flavorRef','adminPass','metadata','networks','OS-DCF:diskConfig','accessIPv4','accessIPv6','personality','availability_zone','block_device_mapping','block_device_mapping_v2','config_drive','key_name','min_count','max_count','return_reservation_id','security_groups','user_data','description','tags','trusted_image_certificates','host','hypervisor_hostname','hostname','os:scheduler_hints','OS-SCH-HNT:scheduler_hints']})
nova_path.write_text(json.dumps(nova,indent=2)+'\n')
