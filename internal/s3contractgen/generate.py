#!/usr/bin/env python3
"""Generate SDK-owned S3 protocol contracts from pinned Selectel/AWS sources."""
from pathlib import Path
import json,hashlib,subprocess,re
R=Path(__file__).resolve().parents[2];A=Path(__file__).parent/'assets';rev=hashlib.sha256((A/'selectel-s3.html').read_bytes()).hexdigest()
ops=[('CreateBucket','PUT','/{bucket}','buckets.create','Buckets.Create','CreateBucketOptions','BucketInfo')]
ops.append(('PutObject','PUT','/{bucket}/{key}','objects.put','Objects.Upload','UploadOptions','Transfer'))
ops.append(('GetObject','GET','/{bucket}/{key}','objects.get','Objects.Download','DownloadOptions','Transfer'))
ops.extend([('HeadBucket','HEAD','/{bucket}','buckets.head','Buckets.Head','types.CallOptions','BucketInfo'),('GetBucketLocation','GET','/{bucket}?location','buckets.location','Buckets.Location','types.CallOptions','BucketInfo'),('DeleteBucket','DELETE','/{bucket}','buckets.delete','Buckets.Delete','types.CallOptions','Empty')])
ops.extend([('HeadObject','HEAD','/{bucket}/{key}','objects.head','Objects.Head','HeadObjectOptions','ObjectInfo'),('DeleteObject','DELETE','/{bucket}/{key}','objects.delete','Objects.Delete','DeleteObjectOptions','ObjectInfo'),('CopyObject','PUT','/{bucket}/{key} copy-source','objects.copy','Objects.Copy','CopyOptions','ObjectInfo')])
ops.append(('DeleteObjects','POST','/{bucket}?delete','objects.delete_many','Objects.DeleteMany','DeleteManyRequest','DeleteManyResponse'))
ops.extend([('ListBuckets','GET','/','buckets.list','Buckets.ListPage','BucketListOptions','BucketSummary'),('ListObjects','GET','/{bucket}','objects.list_v1','Objects.ListPageV1','ObjectListOptions','ListEntry'),('ListObjectsV2','GET','/{bucket}?list-type=2','objects.list_v2','Objects.ListPage','ObjectListOptions','ListEntry')])
b={k:v for k,v in json.loads((R/'catalog/bindings.json').read_text()).items() if not k.startswith('s3.v2006_03_01.')};paths={};over=[]
files=['client.go','generate.go','s3/client.go','s3/wait.go','s3/presign.go','s3/lists.go','s3/bulk.go','s3/objects.go','s3/buckets.go','s3/transfers.go','s3/types_generated.go','internal/s3contractgen/generate.py']+sorted(str(p.relative_to(R)) for d in ['internal/core','internal/endpoints','types','credentials','operations','pagination'] for p in (R/d).glob('*.go') if not p.name.endswith('_test.go'))
covered={
'CreateBucket':{'Bucket','CreateBucketConfiguration','ObjectLockEnabledForBucket'},'DeleteBucket':{'Bucket'},'HeadBucket':{'Bucket'},'GetBucketLocation':{'Bucket'},
'ListBuckets':{'ContinuationToken','MaxBuckets','Prefix','BucketRegion'},
'PutObject':{'Bucket','Key','Body','ContentLength','ContentType','ContentMD5','Metadata','IfMatch','IfNoneMatch'},
'GetObject':{'Bucket','Key','Range','IfMatch','IfNoneMatch','VersionId'},'HeadObject':{'Bucket','Key','Range','IfMatch','IfNoneMatch','VersionId'},'DeleteObject':{'Bucket','Key','VersionId','IfMatch'},'DeleteObjects':{'Bucket','Delete'},
'CopyObject':{'Bucket','Key','CopySource','Metadata','MetadataDirective','ContentType','IfMatch','IfNoneMatch','CopySourceIfMatch','CopySourceIfNoneMatch','CopySourceIfModifiedSince','CopySourceIfUnmodifiedSince'},
'ListObjects':{'Bucket','Delimiter','EncodingType','Marker','MaxKeys','Prefix'},'ListObjectsV2':{'Bucket','ContinuationToken','Delimiter','EncodingType','FetchOwner','MaxKeys','Prefix','StartAfter'}}
def native_fields(native,kind):
 text=(A/('api_op_'+native+'.go.txt')).read_text();match=re.search(r'type '+native+kind+r' struct \{(.*?)\n\}',text,re.S);assert match,native
 return sorted(set(re.findall(r'^\t([A-Z]\w*)\s+',match.group(1),re.M)))
successes={'DeleteBucket':[204],'DeleteObject':[204],'GetObject':[200,206,304],'HeadObject':[200,206,304]}
for native,m,path,suffix,go,inp,out in ops:
 id='s3.v2006_03_01.'+suffix
 o={'operationId':native,'x-operation-locator':m+' '+path,'responses':{str(code):{'description':'AWS S3 typed XML/header response; operation-specific source mapping in SDK contract'} for code in successes.get(native,[200])},'x-native-source':'api_op_'+native+'.go.txt'}
 key='/'+native;paths[key]={m.lower():o};over.append(json.dumps('s3-protocol '+m+' '+key)+':'+json.dumps(id)+',')
 c={'operation_id':id,'source_revision':rev,'endpoint':'explicit Selectel regional S3 URL; default path-style, immutable WithVirtualHosted selects bucket-host derivation; both saved as exact refs with no redirect/fallback/AWS defaults','auth':'s3_sigv4 scope account/project/region/pool; signing region=pool/service=s3','request':{'method':m,'path':path.split('?')[0].split(' copy-source')[0],'query_selector':path.split('?')[1] if '?' in path else None,'header_selector':'x-amz-copy-source' if native=='CopyObject' else None,'native_model':'AWS Go SDK service/s3 v1.114.3 '+native,'mapping':'typed SDK subset; common CallOptions retained; unsupported or unknown AWS variants never forwarded'},'response':{'mapping':'typed XML/header mapping, protected metadata; addressed names/keys are explicit requested addresses, never creation proof','sensitive_fields':['metadata','version_id','signed_url']},'errors':{'mapping':'S3 XML machine code and HTTP status only; provider message discarded; mutation no replay and receipt retained'},'retry':'bounded reads before response streaming; mutations never replay; redirects protocol_error','recovery':'exact caller-addressed refs only; unknown observation never creation proof','wait':'existence/absence only; no object contents/write ownership proof','pagination':'bounded pages and source continuation only; protected opaque cursor','shared_provenance':'reviewed Swift DoResponse 8a07c83 plus reviewed receipt support379458e'}
 native_input=native_fields(native,'Input');native_output=native_fields(native,'Output');assert covered[native].issubset(set(native_input)),native
 missing=sorted(set(native_input)-covered[native])
 exclusions=[('AWS input '+field+': excluded from Selectel subset by published ACL/tagging/request-payment/object-response compatibility; not forwarded' if field=='ACL' or field.startswith('Grant') or field in ['Tagging','TaggingDirective','RequestPayer'] or field.startswith('Response') else 'AWS input '+field+': Selectel support/semantics unconfirmed or delegated advanced variant; not forwarded; evidence owner #74, P1 implementation / acceptance #75') for field in missing]
 if native=='CreateBucket':exclusions.append('CreateBucketConfiguration: directory-bucket location/bucket/tags nested variants unconfirmed; LocationConstraint only')
 if native=='ListBuckets':exclusions.append('NativePagination max-buckets/prefix/bucket-region/continuation request variant has AWS SDK wire evidence only; current Selectel availability unknown')
 c['request']['covered_native_fields']=sorted(covered[native]&set(native_input));c['request']['native_fields']=native_input;c['request']['excluded_variants']=exclusions
 c['response']['native_fields']=native_output
 c['response']['limitation']='Only source-confirmed basic Selectel header/XML subset is typed; advanced AWS output fields are not claimed complete. Provider timestamps retain raw source strings; SDK observation times are UTC time.Time.'
 c['errors']={'400':'invalid_argument','401':'unauthenticated','403':'permission_denied except known authentication machine codes','404':'not_found','409':'conflict','412':'conflict','416':'invalid_argument','429':'rate_limited','503 SlowDown':'rate_limited; bounded safe-read retry','copy HTTP200 Error':'provider_error with Accepted acknowledgement retained; no success claim','delete-many HTTP200 mixed':'provider_error/partial_success, exact per-item results preserved, Accepted acknowledgement retained','provider_message':'discarded; optional XML diagnostics cannot bypass normal status/retry/cancellation'}
 c['local_presign']='PresignGet/PresignPut: explicit CredentialSource only, no HTTP or mutation receipt; protected URL and signed headers; 1..900 whole seconds, source-provided credential expiry enforced' if suffix in ['objects.get','objects.put'] else None
 c['recovery']='Exact named bucket/key address only; never creation or contents proof. Known outcomes conclusive without I/O; unknown bulk or version-selected delete RecoveryUnavailable; version arguments are not serialized into generic key refs.'
 input_type=inp if inp.startswith('types.') else 's3.'+inp
 if native!='ListBuckets':input_type='types.ResourceRef, '+input_type
 if native=='CopyObject':input_type='types.ResourceRef, '+input_type
 if native=='PutObject':input_type='types.ResourceRef, io.Reader, int64, s3.UploadOptions'
 if native=='GetObject':input_type='types.ResourceRef, io.Writer, s3.DownloadOptions'
 output_type='s3.'+out
 if native.startswith('List'):input_type+=', pagination.Options';output_type='pagination.Page['+output_type+']'
 (R/'catalog/contracts'/f'{id}.json').write_text(json.dumps(c,indent=2)+'\n')
 b[id]={'go_method':'s3.'+go,'input_type':input_type,'output_type':output_type,'contract_ref':f'contracts/{id}.json','status':'partial','files':files,'evidence_files':['s3/public_test.go','s3/catalog_test.go'],'evidence_ref':'s3/public_test.go;s3/catalog_test.go','covered_schema_ref':f'contracts/{id}.json#/request','excluded_variants':exclusions+['Advanced AWS response fields not source-confirmed for Selectel; see response native_fields and limitation'],'recovery':m not in ['GET','HEAD'],'wait_goals':(['bucket.present'] if suffix=='buckets.create' else ['bucket.absent'] if suffix=='buckets.delete' else ['object.present'] if suffix=='objects.put' else ['object.absent'] if suffix=='objects.delete' else [])}
s={'openapi':'3.0.3','info':{'title':'SDK-owned S3 protocol extraction, not a provider OpenAPI','version':'2006-03-01'},'paths':paths,'x-source-uri':'https://docs.selectel.ru/api/object-storage-s3/','x-source-revision':rev,'x-sdk-source':'github.com/aws/aws-sdk-go-v2/service/s3 v1.114.3 c8f4a4457aa933bd6c8ce60bc2ca2a0599b77478','x-local-snapshots':{p.name:hashlib.sha256(p.read_bytes()).hexdigest() for p in sorted(A.iterdir()) if p.is_file()}}
(R/'catalog/sources/s3-protocol.json').write_text(json.dumps(s,indent=2)+'\n');(R/'catalog/bindings.json').write_text(json.dumps(b,indent=2)+'\n');p=R/'internal/cataloggen/s3_generated.go';p.write_text('// Code generated by s3contractgen; DO NOT EDIT.\npackage main\nfunc init(){families["s3-protocol"]=family{"s3","v2006_03_01","s3_sigv4",false};for k,v:=range map[string]string{'+''.join(over)+'}{overrides[k]=v}}\n');subprocess.run(['gofmt','-w',str(p)],check=True)
(R/'s3/types_generated.go').write_text('''// Code generated by s3contractgen from pinned AWS S3 operation models; DO NOT EDIT.
package s3
import("github.com/festy23/selectel-sdk/types";"time")
type CreateBucketOptions struct{LocationConstraint *string;ObjectLockEnabled *bool}
type BucketInfo struct{CreationLocation *types.ProtectedValue `json:"creation_location,omitempty"`;Region *string `json:"region,omitempty"`;Ref *types.ResourceRef `json:"ref,omitempty"`;Location *string `json:"location,omitempty"`}
type Empty struct{}
type PresignedURL struct{URL types.ProtectedValue `json:"url"`;Headers map[string]types.ProtectedValue `json:"headers"`;ExpiresAt time.Time `json:"expires_at"`}
type Owner struct{ID *string `xml:"ID" json:"id,omitempty"`;DisplayName *string `xml:"DisplayName" json:"display_name,omitempty"`}
type BucketSummary struct{Name string `json:"name"`;CreationDate,Region,ARN *string;Ref *types.ResourceRef `json:"ref,omitempty"`}
type ObjectSummary struct{Key string `json:"key"`;Size *types.Integer `json:"size,omitempty"`;ETag,LastModified,StorageClass *string;Owner *Owner;Ref *types.ResourceRef `json:"ref,omitempty"`}
type ListEntry struct{Object *ObjectSummary `json:"object,omitempty"`;CommonPrefix *types.ProtectedValue `json:"common_prefix,omitempty"`}
type DeleteEntry struct{Key string;VersionID *types.ProtectedValue}
type DeleteManyRequest struct{Objects []DeleteEntry;Quiet *bool}
type DeleteResult struct{Key string `json:"key"`;VersionID *types.ProtectedValue `json:"version_id,omitempty"`;DeleteMarker *bool `json:"delete_marker,omitempty"`;DeleteMarkerVersionID *types.ProtectedValue `json:"delete_marker_version_id,omitempty"`}
type DeleteFailure struct{Key string `json:"key"`;VersionID *types.ProtectedValue `json:"version_id,omitempty"`;Code string `json:"code"`}
type DeleteManyResponse struct{Deleted []DeleteResult `json:"deleted"`;Errors []DeleteFailure `json:"errors"`;Complete bool `json:"complete"`}
type UploadOptions struct{ContentType *types.ProtectedValue;Metadata map[string]types.ProtectedValue;ContentMD5,IfMatch,IfNoneMatch *string}
type HeadObjectOptions struct{VersionID *types.ProtectedValue;IfMatch,IfNoneMatch,Range *string}
type DeleteObjectOptions struct{VersionID *types.ProtectedValue;IfMatch *string}
type CopyOptions struct{SourceIfModifiedSince,SourceIfUnmodifiedSince *time.Time;SourceVersionID *types.ProtectedValue;MetadataDirective,SourceIfMatch,SourceIfNoneMatch,IfMatch,IfNoneMatch *string;Metadata map[string]types.ProtectedValue;ContentType *types.ProtectedValue}
type ObjectInfo struct{ContentLength *types.Integer `json:"content_length,omitempty"`;CacheControl,ContentDisposition,ContentEncoding,ContentLanguage,Expires *types.ProtectedValue;ContentRange,AcceptRanges,StorageClass *string;DeleteMarker *bool `json:"delete_marker,omitempty"`;SourceVersionID *types.ProtectedValue `json:"source_version_id,omitempty"`;NotModified bool `json:"not_modified,omitempty"`;LastModified *string `json:"last_modified,omitempty"`;Ref *types.ResourceRef `json:"ref,omitempty"`;ETag *string `json:"etag,omitempty"`;VersionID *types.ProtectedValue `json:"version_id,omitempty"`;ContentType *types.ProtectedValue `json:"content_type,omitempty"`;Metadata map[string]types.ProtectedValue `json:"metadata,omitempty"`}
type DownloadOptions struct{MaxBytes int64;Range,IfMatch,IfNoneMatch *string;VersionID *types.ProtectedValue}
type Transfer struct{NotModified bool `json:"not_modified,omitempty"`;Object ObjectInfo `json:"object"`;Bytes int64 `json:"bytes"`;Complete bool `json:"complete"`}
''');subprocess.run(['gofmt','-w',str(R/'s3/types_generated.go')],check=True)
