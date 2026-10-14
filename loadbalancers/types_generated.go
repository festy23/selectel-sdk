// Code generated from Octavia 4486d2416f600df15df1b5c98a676f898e2b1d42; DO NOT EDIT.
package loadbalancers

import "github.com/festy23/selectel-sdk/types"
import "encoding/json"

type IDOnly struct {
	ID string `json:"id"`
}
type LoadBalancerCreate struct {
	Name             *string                 `json:"name,omitempty"`
	Description      *string                 `json:"description,omitempty"`
	AdminStateUp     *bool                   `json:"admin_state_up,omitempty"`
	VIPAddress       *string                 `json:"vip_address,omitempty"`
	VIPPortID        *string                 `json:"vip_port_id,omitempty"`
	VIPSubnetID      *string                 `json:"vip_subnet_id,omitempty"`
	VIPNetworkID     *string                 `json:"vip_network_id,omitempty"`
	VIPQoSPolicyID   *types.Nullable[string] `json:"vip_qos_policy_id,omitempty"`
	AdditionalVips   *[]AdditionalVipsType   `json:"additional_vips,omitempty"`
	ProjectID        *string                 `json:"project_id,omitempty"`
	Listeners        *[]ListenerSingleCreate `json:"listeners,omitempty"`
	Pools            *[]PoolSingleCreate     `json:"pools,omitempty"`
	Provider         *string                 `json:"provider,omitempty"`
	Tags             *[]string               `json:"tags,omitempty"`
	FlavorID         *string                 `json:"flavor_id,omitempty"`
	AvailabilityZone *string                 `json:"availability_zone,omitempty"`
}
type LoadBalancer struct {
	ID                 string               `json:"id"`
	Name               string               `json:"name"`
	Description        string               `json:"description"`
	ProvisioningStatus string               `json:"provisioning_status"`
	OperatingStatus    string               `json:"operating_status"`
	AdminStateUp       bool                 `json:"admin_state_up"`
	ProjectID          string               `json:"project_id"`
	CreatedAt          string               `json:"created_at"`
	UpdatedAt          string               `json:"updated_at"`
	VIPAddress         string               `json:"vip_address"`
	VIPPortID          string               `json:"vip_port_id"`
	VIPSubnetID        string               `json:"vip_subnet_id"`
	VIPNetworkID       string               `json:"vip_network_id"`
	AdditionalVips     []AdditionalVipsType `json:"additional_vips"`
	Listeners          []Listener           `json:"listeners"`
	Pools              []Pool               `json:"pools"`
	Provider           string               `json:"provider"`
	FlavorID           string               `json:"flavor_id"`
	VIPQoSPolicyID     string               `json:"vip_qos_policy_id"`
	Tags               []string             `json:"tags"`
	AvailabilityZone   string               `json:"availability_zone"`
	Ref                *types.ResourceRef   `json:"ref,omitempty"`
}
type LoadBalancerUpdate struct {
	Name           *string                 `json:"name,omitempty"`
	Description    *string                 `json:"description,omitempty"`
	VIPQoSPolicyID *types.Nullable[string] `json:"vip_qos_policy_id,omitempty"`
	AdminStateUp   *bool                   `json:"admin_state_up,omitempty"`
	Tags           *[]string               `json:"tags,omitempty"`
}
type ListenerCreate struct {
	Name                 *string            `json:"name,omitempty"`
	Description          *string            `json:"description,omitempty"`
	AdminStateUp         *bool              `json:"admin_state_up,omitempty"`
	Protocol             string             `json:"protocol"`
	ProtocolPort         types.Integer      `json:"protocol_port"`
	ConnectionLimit      *types.Integer     `json:"connection_limit,omitempty"`
	ProjectID            *string            `json:"project_id,omitempty"`
	DefaultPoolID        *string            `json:"default_pool_id,omitempty"`
	DefaultPool          *PoolSingleCreate  `json:"default_pool,omitempty"`
	InsertHeaders        *map[string]string `json:"insert_headers,omitempty"`
	TimeoutClientData    *types.Integer     `json:"timeout_client_data,omitempty"`
	TimeoutMemberConnect *types.Integer     `json:"timeout_member_connect,omitempty"`
	TimeoutMemberData    *types.Integer     `json:"timeout_member_data,omitempty"`
	TimeoutTCPInspect    *types.Integer     `json:"timeout_tcp_inspect,omitempty"`
	Tags                 *[]string          `json:"tags,omitempty"`
	AllowedCIDRs         *[]string          `json:"allowed_cidrs,omitempty"`
}
type Listener struct {
	ID                     string             `json:"id"`
	Name                   string             `json:"name"`
	Description            string             `json:"description"`
	ProvisioningStatus     string             `json:"provisioning_status"`
	OperatingStatus        string             `json:"operating_status"`
	AdminStateUp           bool               `json:"admin_state_up"`
	Protocol               string             `json:"protocol"`
	ProtocolPort           *types.Integer     `json:"protocol_port"`
	ConnectionLimit        *types.Integer     `json:"connection_limit"`
	DefaultTLSContainerRef string             `json:"default_tls_container_ref"`
	ProjectID              string             `json:"project_id"`
	DefaultPoolID          string             `json:"default_pool_id"`
	L7Policies             []IDOnly           `json:"l7policies"`
	InsertHeaders          map[string]string  `json:"insert_headers"`
	CreatedAt              string             `json:"created_at"`
	UpdatedAt              string             `json:"updated_at"`
	Loadbalancers          []IDOnly           `json:"loadbalancers"`
	TimeoutClientData      *types.Integer     `json:"timeout_client_data"`
	TimeoutMemberConnect   *types.Integer     `json:"timeout_member_connect"`
	TimeoutMemberData      *types.Integer     `json:"timeout_member_data"`
	TimeoutTCPInspect      *types.Integer     `json:"timeout_tcp_inspect"`
	Tags                   []string           `json:"tags"`
	ClientAuthentication   string             `json:"client_authentication"`
	ClientCRLContainerRef  string             `json:"client_crl_container_ref"`
	AllowedCIDRs           []string           `json:"allowed_cidrs"`
	TLSVersions            []string           `json:"tls_versions"`
	ALPNProtocols          []string           `json:"alpn_protocols"`
	Ref                    *types.ResourceRef `json:"ref,omitempty"`
}
type ListenerUpdate struct {
	Name                 *string            `json:"name,omitempty"`
	Description          *string            `json:"description,omitempty"`
	AdminStateUp         *bool              `json:"admin_state_up,omitempty"`
	ConnectionLimit      *types.Integer     `json:"connection_limit,omitempty"`
	DefaultPoolID        *string            `json:"default_pool_id,omitempty"`
	InsertHeaders        *map[string]string `json:"insert_headers,omitempty"`
	TimeoutClientData    *types.Integer     `json:"timeout_client_data,omitempty"`
	TimeoutMemberConnect *types.Integer     `json:"timeout_member_connect,omitempty"`
	TimeoutMemberData    *types.Integer     `json:"timeout_member_data,omitempty"`
	TimeoutTCPInspect    *types.Integer     `json:"timeout_tcp_inspect,omitempty"`
	Tags                 *[]string          `json:"tags,omitempty"`
	AllowedCIDRs         *[]string          `json:"allowed_cidrs,omitempty"`
}
type PoolCreate struct {
	Name               *string                                 `json:"name,omitempty"`
	Description        *string                                 `json:"description,omitempty"`
	AdminStateUp       *bool                                   `json:"admin_state_up,omitempty"`
	Protocol           string                                  `json:"protocol"`
	LbAlgorithm        string                                  `json:"lb_algorithm"`
	SessionPersistence *types.Nullable[SessionPersistencePOST] `json:"session_persistence,omitempty"`
	ProjectID          *string                                 `json:"project_id,omitempty"`
	Healthmonitor      *HealthMonitorSingleCreate              `json:"healthmonitor,omitempty"`
	Members            *[]MemberSingleCreate                   `json:"members,omitempty"`
	Tags               *[]string                               `json:"tags,omitempty"`
}
type Pool struct {
	ID                 string                      `json:"id"`
	Name               string                      `json:"name"`
	Description        string                      `json:"description"`
	ProvisioningStatus string                      `json:"provisioning_status"`
	OperatingStatus    string                      `json:"operating_status"`
	AdminStateUp       bool                        `json:"admin_state_up"`
	Protocol           string                      `json:"protocol"`
	LbAlgorithm        string                      `json:"lb_algorithm"`
	SessionPersistence *SessionPersistenceResponse `json:"session_persistence"`
	ProjectID          string                      `json:"project_id"`
	Loadbalancers      []IDOnly                    `json:"loadbalancers"`
	Listeners          []IDOnly                    `json:"listeners"`
	CreatedAt          string                      `json:"created_at"`
	UpdatedAt          string                      `json:"updated_at"`
	HealthmonitorID    string                      `json:"healthmonitor_id"`
	Members            []Member                    `json:"members"`
	Tags               []string                    `json:"tags"`
	TLSContainerRef    string                      `json:"tls_container_ref"`
	CATLSContainerRef  string                      `json:"ca_tls_container_ref"`
	CRLContainerRef    string                      `json:"crl_container_ref"`
	TLSEnabled         bool                        `json:"tls_enabled"`
	TLSCiphers         string                      `json:"tls_ciphers"`
	TLSVersions        []string                    `json:"tls_versions"`
	ALPNProtocols      []string                    `json:"alpn_protocols"`
	Healthmonitor      *Monitor                    `json:"healthmonitor,omitempty"`
	Ref                *types.ResourceRef          `json:"ref,omitempty"`
}
type PoolUpdate struct {
	Name               *string                                `json:"name,omitempty"`
	Description        *string                                `json:"description,omitempty"`
	AdminStateUp       *bool                                  `json:"admin_state_up,omitempty"`
	LbAlgorithm        *string                                `json:"lb_algorithm,omitempty"`
	SessionPersistence *types.Nullable[SessionPersistencePUT] `json:"session_persistence,omitempty"`
	Tags               *[]string                              `json:"tags,omitempty"`
}
type MonitorCreate struct {
	Name           *string        `json:"name,omitempty"`
	Type           string         `json:"type"`
	Delay          types.Integer  `json:"delay"`
	Timeout        types.Integer  `json:"timeout"`
	MaxRetriesDown *types.Integer `json:"max_retries_down,omitempty"`
	MaxRetries     types.Integer  `json:"max_retries"`
	HTTPMethod     *string        `json:"http_method,omitempty"`
	URLPath        *string        `json:"url_path,omitempty"`
	ExpectedCodes  *string        `json:"expected_codes,omitempty"`
	AdminStateUp   *bool          `json:"admin_state_up,omitempty"`
	ProjectID      *string        `json:"project_id,omitempty"`
	Tags           *[]string      `json:"tags,omitempty"`
	HTTPVersion    *types.Number  `json:"http_version,omitempty"`
	DomainName     *string        `json:"domain_name,omitempty"`
}
type Monitor struct {
	ID                 string             `json:"id"`
	Name               string             `json:"name"`
	Type               string             `json:"type"`
	Delay              *types.Integer     `json:"delay"`
	Timeout            *types.Integer     `json:"timeout"`
	MaxRetries         *types.Integer     `json:"max_retries"`
	MaxRetriesDown     *types.Integer     `json:"max_retries_down"`
	HTTPMethod         string             `json:"http_method"`
	URLPath            string             `json:"url_path"`
	ExpectedCodes      string             `json:"expected_codes"`
	AdminStateUp       bool               `json:"admin_state_up"`
	ProjectID          string             `json:"project_id"`
	Pools              []IDOnly           `json:"pools"`
	ProvisioningStatus string             `json:"provisioning_status"`
	OperatingStatus    string             `json:"operating_status"`
	CreatedAt          string             `json:"created_at"`
	UpdatedAt          string             `json:"updated_at"`
	Tags               []string           `json:"tags"`
	HTTPVersion        *types.Number      `json:"http_version"`
	DomainName         string             `json:"domain_name"`
	Ref                *types.ResourceRef `json:"ref,omitempty"`
}
type MonitorUpdate struct {
	Name           *string        `json:"name,omitempty"`
	Delay          *types.Integer `json:"delay,omitempty"`
	Timeout        *types.Integer `json:"timeout,omitempty"`
	MaxRetriesDown *types.Integer `json:"max_retries_down,omitempty"`
	MaxRetries     *types.Integer `json:"max_retries,omitempty"`
	HTTPMethod     *string        `json:"http_method,omitempty"`
	URLPath        *string        `json:"url_path,omitempty"`
	ExpectedCodes  *string        `json:"expected_codes,omitempty"`
	AdminStateUp   *bool          `json:"admin_state_up,omitempty"`
	Tags           *[]string      `json:"tags,omitempty"`
	HTTPVersion    *types.Number  `json:"http_version,omitempty"`
	DomainName     *string        `json:"domain_name,omitempty"`
}
type MemberCreate struct {
	Name           *string                        `json:"name,omitempty"`
	AdminStateUp   *bool                          `json:"admin_state_up,omitempty"`
	Address        string                         `json:"address"`
	ProtocolPort   types.Integer                  `json:"protocol_port"`
	Weight         *types.Integer                 `json:"weight,omitempty"`
	Backup         *bool                          `json:"backup,omitempty"`
	SubnetID       *types.Nullable[string]        `json:"subnet_id,omitempty"`
	ProjectID      *string                        `json:"project_id,omitempty"`
	MonitorPort    *types.Nullable[types.Integer] `json:"monitor_port,omitempty"`
	MonitorAddress *types.Nullable[string]        `json:"monitor_address,omitempty"`
	Tags           *[]string                      `json:"tags,omitempty"`
}
type Member struct {
	ID                 string             `json:"id"`
	Name               string             `json:"name"`
	OperatingStatus    string             `json:"operating_status"`
	ProvisioningStatus string             `json:"provisioning_status"`
	AdminStateUp       bool               `json:"admin_state_up"`
	Address            string             `json:"address"`
	ProtocolPort       *types.Integer     `json:"protocol_port"`
	Weight             *types.Integer     `json:"weight"`
	Backup             bool               `json:"backup"`
	SubnetID           string             `json:"subnet_id"`
	ProjectID          string             `json:"project_id"`
	CreatedAt          string             `json:"created_at"`
	UpdatedAt          string             `json:"updated_at"`
	MonitorAddress     string             `json:"monitor_address"`
	MonitorPort        *types.Integer     `json:"monitor_port"`
	Tags               []string           `json:"tags"`
	Ref                *types.ResourceRef `json:"ref,omitempty"`
}
type MemberUpdate struct {
	Name           *string                        `json:"name,omitempty"`
	AdminStateUp   *bool                          `json:"admin_state_up,omitempty"`
	Weight         *types.Integer                 `json:"weight,omitempty"`
	Backup         *bool                          `json:"backup,omitempty"`
	MonitorPort    *types.Nullable[types.Integer] `json:"monitor_port,omitempty"`
	MonitorAddress *types.Nullable[string]        `json:"monitor_address,omitempty"`
	Tags           *[]string                      `json:"tags,omitempty"`
}
type StatusResponse struct {
	Loadbalancer *LoadBalancerStatusResponse `json:"loadbalancer"`
	Ref          *types.ResourceRef          `json:"ref,omitempty"`
}
type LoadBalancerStatisticsResponse struct {
	BytesIn           *types.Integer     `json:"bytes_in"`
	BytesOut          *types.Integer     `json:"bytes_out"`
	ActiveConnections *types.Integer     `json:"active_connections"`
	TotalConnections  *types.Integer     `json:"total_connections"`
	RequestErrors     *types.Integer     `json:"request_errors"`
	Ref               *types.ResourceRef `json:"ref,omitempty"`
}
type AdditionalVipsType struct {
	SubnetID  string  `json:"subnet_id"`
	IPAddress *string `json:"ip_address,omitempty"`
}
type ListenerSingleCreate struct {
	Name                 *string            `json:"name,omitempty"`
	Description          *string            `json:"description,omitempty"`
	AdminStateUp         *bool              `json:"admin_state_up,omitempty"`
	Protocol             string             `json:"protocol"`
	ProtocolPort         types.Integer      `json:"protocol_port"`
	ConnectionLimit      *types.Integer     `json:"connection_limit,omitempty"`
	DefaultPoolID        *string            `json:"default_pool_id,omitempty"`
	DefaultPool          *PoolSingleCreate  `json:"default_pool,omitempty"`
	InsertHeaders        *map[string]string `json:"insert_headers,omitempty"`
	TimeoutClientData    *types.Integer     `json:"timeout_client_data,omitempty"`
	TimeoutMemberConnect *types.Integer     `json:"timeout_member_connect,omitempty"`
	TimeoutMemberData    *types.Integer     `json:"timeout_member_data,omitempty"`
	TimeoutTCPInspect    *types.Integer     `json:"timeout_tcp_inspect,omitempty"`
	Tags                 *[]string          `json:"tags,omitempty"`
	AllowedCIDRs         *[]string          `json:"allowed_cidrs,omitempty"`
}
type PoolSingleCreate struct {
	Name               *string                                 `json:"name,omitempty"`
	Description        *string                                 `json:"description,omitempty"`
	AdminStateUp       *bool                                   `json:"admin_state_up,omitempty"`
	Protocol           *string                                 `json:"protocol,omitempty"`
	LbAlgorithm        *string                                 `json:"lb_algorithm,omitempty"`
	SessionPersistence *types.Nullable[SessionPersistencePOST] `json:"session_persistence,omitempty"`
	Healthmonitor      *HealthMonitorSingleCreate              `json:"healthmonitor,omitempty"`
	Members            *[]MemberSingleCreate                   `json:"members,omitempty"`
	Tags               *[]string                               `json:"tags,omitempty"`
}
type SessionPersistencePOST struct {
	Type                   string         `json:"type"`
	CookieName             *string        `json:"cookie_name,omitempty"`
	PersistenceTimeout     *types.Integer `json:"persistence_timeout,omitempty"`
	PersistenceGranularity *string        `json:"persistence_granularity,omitempty"`
}
type HealthMonitorSingleCreate struct {
	Name           *string        `json:"name,omitempty"`
	Type           string         `json:"type"`
	Delay          types.Integer  `json:"delay"`
	Timeout        types.Integer  `json:"timeout"`
	MaxRetriesDown *types.Integer `json:"max_retries_down,omitempty"`
	MaxRetries     types.Integer  `json:"max_retries"`
	HTTPMethod     *string        `json:"http_method,omitempty"`
	URLPath        *string        `json:"url_path,omitempty"`
	ExpectedCodes  *string        `json:"expected_codes,omitempty"`
	AdminStateUp   *bool          `json:"admin_state_up,omitempty"`
	Tags           *[]string      `json:"tags,omitempty"`
	HTTPVersion    *types.Number  `json:"http_version,omitempty"`
	DomainName     *string        `json:"domain_name,omitempty"`
}
type MemberSingleCreate struct {
	Name           *string                        `json:"name,omitempty"`
	AdminStateUp   *bool                          `json:"admin_state_up,omitempty"`
	Address        string                         `json:"address"`
	ProtocolPort   types.Integer                  `json:"protocol_port"`
	Weight         *types.Integer                 `json:"weight,omitempty"`
	Backup         *bool                          `json:"backup,omitempty"`
	SubnetID       *types.Nullable[string]        `json:"subnet_id,omitempty"`
	MonitorPort    *types.Nullable[types.Integer] `json:"monitor_port,omitempty"`
	MonitorAddress *types.Nullable[string]        `json:"monitor_address,omitempty"`
	Tags           *[]string                      `json:"tags,omitempty"`
}
type SessionPersistenceResponse struct {
	Type                   string             `json:"type"`
	CookieName             string             `json:"cookie_name"`
	PersistenceTimeout     *types.Integer     `json:"persistence_timeout"`
	PersistenceGranularity string             `json:"persistence_granularity"`
	Ref                    *types.ResourceRef `json:"ref,omitempty"`
}
type SessionPersistencePUT struct {
	Type                   *string        `json:"type,omitempty"`
	CookieName             *string        `json:"cookie_name,omitempty"`
	PersistenceTimeout     *types.Integer `json:"persistence_timeout,omitempty"`
	PersistenceGranularity *string        `json:"persistence_granularity,omitempty"`
}
type LoadBalancerStatusResponse struct {
	ID                 string                   `json:"id"`
	Name               string                   `json:"name"`
	OperatingStatus    string                   `json:"operating_status"`
	ProvisioningStatus string                   `json:"provisioning_status"`
	Listeners          []ListenerStatusResponse `json:"listeners"`
	Ref                *types.ResourceRef       `json:"ref,omitempty"`
}
type ListenerStatusResponse struct {
	ID                 string               `json:"id"`
	Name               string               `json:"name"`
	OperatingStatus    string               `json:"operating_status"`
	ProvisioningStatus string               `json:"provisioning_status"`
	Pools              []PoolStatusResponse `json:"pools"`
	Ref                *types.ResourceRef   `json:"ref,omitempty"`
}
type PoolStatusResponse struct {
	ID                 string                       `json:"id"`
	Name               string                       `json:"name"`
	ProvisioningStatus string                       `json:"provisioning_status"`
	OperatingStatus    string                       `json:"operating_status"`
	HealthMonitor      *HealthMonitorStatusResponse `json:"health_monitor"`
	Members            []MemberStatusResponse       `json:"members"`
	Ref                *types.ResourceRef           `json:"ref,omitempty"`
}
type HealthMonitorStatusResponse struct {
	ID                 string             `json:"id"`
	Name               string             `json:"name"`
	Type               string             `json:"type"`
	ProvisioningStatus string             `json:"provisioning_status"`
	OperatingStatus    string             `json:"operating_status"`
	Ref                *types.ResourceRef `json:"ref,omitempty"`
}
type MemberStatusResponse struct {
	ID                 string             `json:"id"`
	Name               string             `json:"name"`
	OperatingStatus    string             `json:"operating_status"`
	ProvisioningStatus string             `json:"provisioning_status"`
	Address            string             `json:"address"`
	ProtocolPort       *types.Integer     `json:"protocol_port"`
	Ref                *types.ResourceRef `json:"ref,omitempty"`
}

func (v *LoadBalancer) UnmarshalJSON(raw []byte) error {
	type wire LoadBalancer
	var w wire
	if e := json.Unmarshal(raw, &w); e != nil {
		return e
	}
	w.Ref = nil
	*v = LoadBalancer(w)
	return nil
}
func (v *Listener) UnmarshalJSON(raw []byte) error {
	type wire Listener
	var w wire
	if e := json.Unmarshal(raw, &w); e != nil {
		return e
	}
	w.Ref = nil
	*v = Listener(w)
	return nil
}
func (v *Pool) UnmarshalJSON(raw []byte) error {
	type wire Pool
	var w wire
	if e := json.Unmarshal(raw, &w); e != nil {
		return e
	}
	w.Ref = nil
	*v = Pool(w)
	return nil
}
func (v *Monitor) UnmarshalJSON(raw []byte) error {
	type wire Monitor
	var w wire
	if e := json.Unmarshal(raw, &w); e != nil {
		return e
	}
	w.Ref = nil
	*v = Monitor(w)
	return nil
}
func (v *Member) UnmarshalJSON(raw []byte) error {
	type wire Member
	var w wire
	if e := json.Unmarshal(raw, &w); e != nil {
		return e
	}
	w.Ref = nil
	*v = Member(w)
	return nil
}
func (v *StatusResponse) UnmarshalJSON(raw []byte) error {
	type wire StatusResponse
	var w wire
	if e := json.Unmarshal(raw, &w); e != nil {
		return e
	}
	w.Ref = nil
	*v = StatusResponse(w)
	return nil
}
func (v *LoadBalancerStatisticsResponse) UnmarshalJSON(raw []byte) error {
	type wire LoadBalancerStatisticsResponse
	var w wire
	if e := json.Unmarshal(raw, &w); e != nil {
		return e
	}
	w.Ref = nil
	*v = LoadBalancerStatisticsResponse(w)
	return nil
}
func (v *SessionPersistenceResponse) UnmarshalJSON(raw []byte) error {
	type wire SessionPersistenceResponse
	var w wire
	if e := json.Unmarshal(raw, &w); e != nil {
		return e
	}
	w.Ref = nil
	*v = SessionPersistenceResponse(w)
	return nil
}
func (v *LoadBalancerStatusResponse) UnmarshalJSON(raw []byte) error {
	type wire LoadBalancerStatusResponse
	var w wire
	if e := json.Unmarshal(raw, &w); e != nil {
		return e
	}
	w.Ref = nil
	*v = LoadBalancerStatusResponse(w)
	return nil
}
func (v *ListenerStatusResponse) UnmarshalJSON(raw []byte) error {
	type wire ListenerStatusResponse
	var w wire
	if e := json.Unmarshal(raw, &w); e != nil {
		return e
	}
	w.Ref = nil
	*v = ListenerStatusResponse(w)
	return nil
}
func (v *PoolStatusResponse) UnmarshalJSON(raw []byte) error {
	type wire PoolStatusResponse
	var w wire
	if e := json.Unmarshal(raw, &w); e != nil {
		return e
	}
	w.Ref = nil
	*v = PoolStatusResponse(w)
	return nil
}
func (v *HealthMonitorStatusResponse) UnmarshalJSON(raw []byte) error {
	type wire HealthMonitorStatusResponse
	var w wire
	if e := json.Unmarshal(raw, &w); e != nil {
		return e
	}
	w.Ref = nil
	*v = HealthMonitorStatusResponse(w)
	return nil
}
func (v *MemberStatusResponse) UnmarshalJSON(raw []byte) error {
	type wire MemberStatusResponse
	var w wire
	if e := json.Unmarshal(raw, &w); e != nil {
		return e
	}
	w.Ref = nil
	*v = MemberStatusResponse(w)
	return nil
}
