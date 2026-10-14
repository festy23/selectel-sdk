package compute

import (
	"github.com/festy23/selectel-sdk/internal/core"
	"github.com/festy23/selectel-sdk/types"
	"net/http"
	"strconv"
	"strings"
)

// microversion is capped by Selectel's published Nova Zed matrix (maximum 2.93).
func microversion(v string) (int, string, error) {
	if v == "" {
		v = "2.1"
	}
	if !strings.HasPrefix(v, "2.") {
		return 0, "", types.NewError(types.InvalidArgument)
	}
	minor, e := strconv.Atoi(strings.TrimPrefix(v, "2."))
	if e != nil || minor < 1 || minor > 93 || v != "2."+strconv.Itoa(minor) {
		return 0, "", types.NewError(types.InvalidArgument)
	}
	return minor, v, nil
}
func createOperation(v string) (core.Operation, error) {
	op := operation("create", "POST")
	_, selected, e := microversion(v)
	op.Headers = http.Header{"OpenStack-API-Version": {"compute " + selected}, "X-OpenStack-Nova-API-Version": {selected}}
	return op, e
}
