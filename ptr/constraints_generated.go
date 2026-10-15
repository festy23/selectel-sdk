// Code generated from PTR request schema; DO NOT EDIT.
package ptr

const (
	minContent = 2
	maxContent = 65534
	minTTL     = 60
	maxTTL     = 604800
)

var rejectedStatuses = map[string][]int{"create": {400, 401, 409, 422}, "get": {400, 401, 404, 422}, "update": {400, 401, 404, 422}, "delete": {400, 401, 404, 422}, "list": {400, 401, 422}}
