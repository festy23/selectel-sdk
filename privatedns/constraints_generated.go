// Code generated from request schemas; DO NOT EDIT.
package privatedns

var rejectedStatuses = map[string][]int{"services.create": {400, 401, 403}, "services.get": {401, 403, 404}, "services.update": {400, 401, 403, 404}, "services.delete": {401, 403, 404}, "zones.create": {400, 401, 403}, "zones.get": {401, 403, 404}, "zones.update": {400, 401, 403, 404}, "zones.delete": {401, 403, 404}, "records.replace": {400, 401, 403, 404}, "records.patch": {400, 401, 403, 404}, "services.list": {401, 403}, "zones.list": {401, 403}, "records.list": {401, 403, 404}}

const (
	maxZoneName      = 64
	maxZoneDomain    = 255
	maxRecordDomain  = 255
	maxCreateRecords = 100
	maxReplace       = 1000
	maxDelete        = 1000
	maxValues        = 100
)
