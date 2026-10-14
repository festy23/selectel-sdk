// Package endpoints contains documented address recipes, never hostname guesses.
package endpoints

import "github.com/festy23/selectel-sdk/types"

// Region IDs follow the Selectel API URL table's location tabs.
// Source: https://docs.selectel.ru/api/urls/
var pools = map[string]string{"ru-1": "spb", "ru-3": "spb", "ru-9": "spb", "gis-2": "spb", "ru-2": "msk", "ru-6": "msk", "ru-7": "msk", "gis-1": "msk", "ru-8": "nsk", "uz-1": "tas", "uz-2": "tas", "kz-1": "alm", "ke-1": "nrb"}

func init() {
	for _, pool := range []string{"SPB-2", "SPB-3", "SPB-4", "SPB-5"} {
		pools[pool] = "spb"
	}
	for _, pool := range []string{"MSK-1", "MSK-2", "MSK-3", "MSK-7"} {
		pools[pool] = "msk"
	}
	pools["NSK-1"] = "nsk"
	pools["TAS-1"] = "tas"
	pools["TAS-2"] = "tas"
	pools["ALM-1"] = "alm"
	pools["NBO-1"] = "nrb"
}

func Location(region, pool string) (string, error) {
	known, ok := pools[pool]
	if region == "" {
		if !ok {
			return "", types.NewError(types.ScopeMismatch)
		}
		return known, nil
	}
	if ok && region != known {
		return "", types.NewError(types.ScopeMismatch)
	}
	return region, nil
}

var addresses = map[string]map[string]string{
	"logs.cloudwatch":    {"ru-1": "https://ru-1.logs.selcloud.ru", "ru-3": "https://ru-3.logs.selcloud.ru", "ru-9": "https://ru-9.logs.selcloud.ru", "ru-2": "https://ru-2.logs.selcloud.ru", "ru-6": "https://ru-6.logs.selcloud.ru", "ru-7": "https://ru-7.logs.selcloud.ru", "ru-8": "https://ru-8.logs.selcloud.ru", "uz-1": "https://uz-1.logs.servercore.com", "uz-2": "https://uz-2.logs.servercore.com", "kz-1": "https://kz-1.logs.servercore.com", "ke-1": "https://ke-1.logs.servercore.com"},
	"metrics.prometheus": {"ru-1": "https://ru-1.metrics.selcloud.ru", "ru-3": "https://ru-3.metrics.selcloud.ru", "ru-9": "https://ru-9.metrics.selcloud.ru", "ru-2": "https://ru-2.metrics.selcloud.ru", "ru-6": "https://ru-6.metrics.selcloud.ru", "ru-7": "https://ru-7.metrics.selcloud.ru", "ru-8": "https://ru-8.metrics.selcloud.ru", "gis-1": "https://gis-1.metrics.selcloud.ru", "gis-2": "https://gis-2.metrics.selcloud.ru", "uz-1": "https://uz-1.metrics.servercore.com", "uz-2": "https://uz-2.metrics.servercore.com", "kz-1": "https://kz-1.metrics.servercore.com", "ke-1": "https://ke-1.metrics.servercore.com", "SPB-2": "https://ru-1.metrics.selcloud.ru", "SPB-3": "https://ru-1.metrics.selcloud.ru", "SPB-4": "https://ru-1.metrics.selcloud.ru", "SPB-5": "https://ru-1.metrics.selcloud.ru", "MSK-1": "https://ru-7.metrics.selcloud.ru", "MSK-2": "https://ru-7.metrics.selcloud.ru", "MSK-3": "https://ru-7.metrics.selcloud.ru", "MSK-7": "https://ru-7.metrics.selcloud.ru", "NSK-1": "https://ru-8.metrics.selcloud.ru", "TAS-1": "https://uz-1.metrics.servercore.com", "TAS-2": "https://uz-1.metrics.servercore.com", "ALM-1": "https://kz-1.metrics.servercore.com", "NBO-1": "https://ke-1.metrics.servercore.com"},
	"mks.v2":             {"ru-1": "https://ru-1.mks.selcloud.ru/v2", "ru-3": "https://ru-3.mks.selcloud.ru/v2", "ru-9": "https://ru-8.mks.selcloud.ru/v2", "ru-2": "https://ru-2.mks.selcloud.ru/v2", "ru-7": "https://ru-7.mks.selcloud.ru/v2", "ru-8": "https://ru-8.mks.selcloud.ru/v2", "uz-1": "https://uz-1.mks.api.servercore.com/v2", "ke-1": "https://ke-1.mks.api.servercore.com/v2"},
	"databases.v1":       {"ru-1": "https://ru-1.dbaas.selcloud.ru/v1", "ru-3": "https://ru-3.dbaas.selcloud.ru/v1", "ru-9": "https://ru-9.dbaas.selcloud.ru/v1", "ru-2": "https://ru-2.dbaas.selcloud.ru/v1", "ru-6": "https://ru-6.dbaas.selcloud.ru/v1", "ru-7": "https://ru-7.dbaas.selcloud.ru/v1", "ru-8": "https://ru-8.dbaas.selcloud.ru/v1", "uz-1": "https://uz-1.dbaas.api.servercore.com/v1", "uz-2": "https://uz-2.dbaas.api.servercore.com/v1", "kz-1": "https://kz-1.dbaas.api.servercore.com/v1", "ke-1": "https://ke-1.dbaas.api.servercore.com/v1"},
}

func Resolve(service, generation string, scope types.Scope) (types.EndpointBinding, error) {
	if service == "iam" && generation == "v1" {
		if scope.ProjectID != "" || scope.Region != "" || scope.Pool != "" {
			return types.EndpointBinding{}, types.NewError(types.ScopeMismatch)
		}
		return types.EndpointBinding{ID: "selectel-iam", URL: "https://api.selectel.ru", Scope: scope, APIGeneration: generation, Audience: "selectel"}, nil
	}
	key := service + "." + generation
	u := addresses[key][scope.Pool]
	if u == "" {
		return types.EndpointBinding{}, types.NewError(types.EndpointUnavailable)
	}
	return types.EndpointBinding{ID: "selectel-" + key + ":" + scope.Pool, URL: u, Scope: scope, APIGeneration: generation, Audience: "selectel"}, nil
}

func Registry(scope types.Scope, generation string) (types.EndpointBinding, error) {
	if generation != "v1" && generation != "v2" {
		return types.EndpointBinding{}, types.NewError(types.CapabilityUnavailable)
	}
	url := "https://cr.selcloud.ru/api"
	id := "selectel-registry"
	if scope.Region == "tas" || scope.Region == "alm" || scope.Region == "nrb" {
		url = "https://cr.servercore.com/api"
		id = "selectel-registry-servercore"
	}
	return types.EndpointBinding{ID: id, URL: url, Scope: scope, APIGeneration: generation, Audience: "selectel"}, nil
}
