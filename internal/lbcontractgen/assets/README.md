# Pinned public evidence

Octavia source and API reference: release 12.0.0 (2023.1), commit
4486d2416f600df15df1b5c98a676f898e2b1d42:
https://github.com/openstack/octavia/tree/4486d2416f600df15df1b5c98a676f898e2b1d42

- octavia-{load-balancer,listener,pool,member,monitor}.py.txt correspond to
  octavia/api/v2/types/{load_balancer,listener,pool,member,health_monitor}.py.
- root_controller.py.txt is octavia/api/root_controller.py (v2/v2.0 routing,
  version capabilities through 2.26; no negotiated microversion header).
- load_balancer.py.txt is octavia/api/v2/controllers/load_balancer.py.
- common-constants.py.txt is octavia/common/constants.py (integer bounds).
- *.inc.txt come from api-ref/source/v2/*.inc; parameters.yaml.txt comes from
  api-ref/source/parameters.yaml. Full graph, filters/fields/pagination,
  parent relations, response/status variants, batch additive_only and errors
  are covered by this reference.

Gophercloud request/result/URL files: public Go module v2.13.0, already pinned
by this repository's go.mod/go.sum; tag resolves to commit
0e61a3a3fc1f72a396a4e86785dd8b633d4c55fb. Source names preserve package + file:
https://github.com/gophercloud/gophercloud/tree/v2.13.0/openstack/loadbalancer/v2

Selectel compatibility and service-specific confirmation (checked 2026-10-15):
https://docs.selectel.ru/en/cloud-servers/tools/openstack-cli/about-openstack-cli/
https://docs.selectel.ru/en/terraform/openstack-provider-reference/lbaas-octavia/resources/openstack_lb_loadbalancer_v2/
https://docs.selectel.ru/en/terraform/openstack-provider-reference/lbaas-octavia/resources/openstack_lb_listener_v2/
https://docs.selectel.ru/en/terraform/openstack-provider-reference/lbaas-octavia/resources/openstack_lb_pool_v2/
https://docs.selectel.ru/en/cloud-servers/load-balancers/about-load-balancers/
https://docs.selectel.ru/en/cloud-servers/load-balancers/manage/failover/

Selectel log offloading and current provider security-group options are known
additional source inventory work; absence from pinned 2.26 is not proof of
unsupported Selectel availability. TLS/L7 inline writes remain partial to #41.
All snapshots are public source only; none contains cloud credentials or live
resource data. Local SHA256 digests are emitted into the source manifest.
