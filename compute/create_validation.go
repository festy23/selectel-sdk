package compute

import (
	"github.com/festy23/selectel-sdk/types"
	"regexp"
)

var novaUUID = regexp.MustCompile(`^[a-fA-F0-9]{8}-[a-fA-F0-9]{4}-[a-fA-F0-9]{4}-[a-fA-F0-9]{4}-[a-fA-F0-9]{12}$`)
var novaHostname = regexp.MustCompile(`^[a-zA-Z0-9]+[a-zA-Z0-9-]*[a-zA-Z0-9]+$`)

// These constraints are pinned in Nova Zed parameter_types.py, not inferred
// from the latest upstream API (which accepts FQDN hostnames only from 2.94).
func validateCreateFields(r CreateRequest) error {
	if name := r.Options.Hostname; name != "" && (len(name) < 2 || len(name) > 63 || !novaHostname.MatchString(name)) {
		return types.NewError(types.InvalidArgument)
	}
	if r.TrustedImageCertificates != nil {
		ids, valid := r.TrustedImageCertificates.Value()
		if valid {
			if len(ids) < 1 || len(ids) > 50 {
				return types.NewError(types.InvalidArgument)
			}
			seen := map[string]bool{}
			for _, id := range ids {
				if id == "" || seen[id] {
					return types.NewError(types.InvalidArgument)
				}
				seen[id] = true
			}
		}
	}
	return nil
}
