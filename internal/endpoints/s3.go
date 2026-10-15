package endpoints

import (
	"github.com/festy23/selectel-sdk/types"
	"net"
	"net/url"
	"strings"
)

// S3VirtualHosted derives only an explicitly selected bucket host from a bound
// S3 endpoint. It never discovers a region or follows a redirect.
func S3VirtualHosted(ep types.EndpointBinding, bucket string) (types.EndpointBinding, error) {
	if types.ValidateEndpoint(ep.URL) != nil || ep.APIGeneration != "v2006_03_01" || ep.Audience != "selectel" || ep.ID == "" {
		return ep, types.NewError(types.EndpointUnavailable)
	}
	if len(bucket) < 3 || len(bucket) > 63 || strings.Contains(bucket, "..") || !alphaNumeric(bucket[0]) || !alphaNumeric(bucket[len(bucket)-1]) {
		return ep, types.NewError(types.InvalidArgument)
	}
	for _, c := range bucket {
		if !(c >= 'a' && c <= 'z' || c >= '0' && c <= '9' || c == '-' || c == '.') {
			return ep, types.NewError(types.InvalidArgument)
		}
	}
	u, _ := url.Parse(ep.URL)
	if net.ParseIP(u.Hostname()) != nil {
		return ep, types.NewError(types.EndpointUnavailable)
	}
	u.Host = bucket + "." + u.Host
	ep.URL = u.String()
	ep.ID += ":bucket=" + bucket
	return ep, nil
}
func alphaNumeric(c byte) bool { return c >= 'a' && c <= 'z' || c >= '0' && c <= '9' }
