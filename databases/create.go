package databases

import (
	"encoding/json"
	"math/big"
	"regexp"
	"unicode/utf8"

	"github.com/festy23/selectel-sdk/types"
)

var datastoreName = regexp.MustCompile(`^[A-Za-z0-9_-]{1,255}$`)

func integerRange(value *types.Integer, minimum int64, maximum *big.Int) bool {
	if value == nil {
		return false
	}
	n, ok := new(big.Int).SetString(value.String(), 10)
	return ok && n.Cmp(big.NewInt(minimum)) >= 0 && (maximum == nil || n.Cmp(maximum) <= 0)
}
func validString(value *string) bool { return value == nil || utf8.ValidString(*value) }
func nonempty(value *string) bool    { return value != nil && *value != "" && utf8.ValidString(*value) }

func validateCreate(r CreateRequest) error {
	invalid := types.NewError(types.InvalidArgument)
	if r.Validate() != nil || !datastoreName.MatchString(*r.Name) || !integerRange(r.NodeCount, 1, nil) || !nonempty(r.TypeID) || !nonempty(r.SubnetID) {
		return invalid
	}
	if r.BackupRetentionDays != nil && !integerRange(r.BackupRetentionDays, 1, nil) {
		return invalid
	}
	if r.FlavorID != nil && !nonempty(r.FlavorID) {
		return invalid
	}
	if r.Disk != nil && (!integerRange(r.Disk.Size, 1, nil) || *r.Disk.Type != "network-ultra") {
		return invalid
	}
	if r.Flavor != nil {
		f := r.Flavor
		if !integerRange(f.Disk, 1, nil) || !integerRange(f.RAM, 1, nil) || !integerRange(f.VCPUs, 1, nil) || (f.DiskType != nil && *f.DiskType != "local" && *f.DiskType != "network-ultra") {
			return invalid
		}
	}
	if r.FloatingIPs != nil {
		max, _ := new(big.Int).SetString(r.NodeCount.String(), 10)
		max.Sub(max, big.NewInt(1))
		if !integerRange(r.FloatingIPs.Master, 0, big.NewInt(1)) || !integerRange(r.FloatingIPs.Replica, 0, max) {
			return invalid
		}
	}
	if r.Pooler != nil {
		p := r.Pooler
		if p.Size != nil && !integerRange(p.Size, 1, big.NewInt(500)) {
			return invalid
		}
		if p.Mode != nil && *p.Mode != "session" && *p.Mode != "transaction" && *p.Mode != "statement" {
			return invalid
		}
	}
	if r.Restore != nil && (!nonempty(r.Restore.DatastoreID) || !validString(r.Restore.TargetTime)) {
		return invalid
	}
	if r.LogPlatform != nil && !validString(r.LogPlatform.LogGroup) {
		return invalid
	}
	if r.SecurityGroups != nil {
		if len(r.SecurityGroups) == 0 {
			return invalid
		}
		seen := map[string]bool{}
		for _, id := range r.SecurityGroups {
			if id == "" || !utf8.ValidString(id) || seen[id] {
				return invalid
			}
			seen[id] = true
		}
	}
	if r.RedisPassword != nil {
		value := r.RedisPassword.Reveal()
		if !utf8.Valid(value) || utf8.RuneCount(value) < 1 || utf8.RuneCount(value) > 255 {
			return invalid
		}
	}
	if r.Config != nil {
		raw := r.Config.Reveal()
		var object map[string]json.RawMessage
		if !utf8.Valid(raw) || json.Unmarshal(raw, &object) != nil || object == nil {
			return invalid
		}
	}
	return nil
}

// createBody reveals protected fields only in the outgoing request. Public DTO
// JSON and formatting remain redacted; no default engine settings are invented.
func createBody(request CreateRequest) ([]byte, error) {
	if err := validateCreate(request); err != nil {
		return nil, err
	}
	raw, err := json.Marshal(request)
	if err != nil {
		return nil, types.NewError(types.InvalidArgument)
	}
	var fields map[string]json.RawMessage
	if json.Unmarshal(raw, &fields) != nil {
		return nil, types.NewError(types.InvalidArgument)
	}
	if request.Config != nil {
		fields["config"] = request.Config.Reveal()
	}
	if request.RedisPassword != nil {
		fields["redis_password"], err = json.Marshal(string(request.RedisPassword.Reveal()))
		if err != nil {
			return nil, types.NewError(types.InvalidArgument)
		}
	}
	body, err := json.Marshal(map[string]any{"datastore": fields})
	if err != nil {
		return nil, types.NewError(types.InvalidArgument)
	}
	return body, nil
}
