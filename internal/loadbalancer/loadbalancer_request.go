package loadbalancer

import (
	"errors"
	"fmt"
	"strings"

	"github.com/UpCloudLtd/upcloud-cloud-controller-manager/internal/utils"
	"github.com/UpCloudLtd/upcloud-go-api/v8/upcloud"
	"github.com/UpCloudLtd/upcloud-go-api/v8/upcloud/request"
	v1 "k8s.io/api/core/v1"
)

const (
	networkNamePublic  string = "public-IPv4"
	networkNamePrivate string = "private-IPv4"
)

// TODO: implement this as service method into UpCloud's SDK
type replaceLoadBalancerRequest struct {
	UUID             string                               `json:"-"`
	Name             string                               `json:"name,omitempty"`
	Plan             string                               `json:"plan,omitempty"`
	ConfiguredStatus upcloud.LoadBalancerConfiguredStatus `json:"configured_status,omitempty"`
	Frontends        []request.LoadBalancerFrontend       `json:"frontends"`
	Backends         []request.LoadBalancerBackend        `json:"backends"`
	Resolvers        []request.LoadBalancerResolver       `json:"resolvers"`
	Labels           []upcloud.Label                      `json:"labels,omitempty"`
	MaintenanceDOW   upcloud.LoadBalancerMaintenanceDOW   `json:"maintenance_dow,omitempty"`
	MaintenanceTime  string                               `json:"maintenance_time,omitempty"`
	IPAddresses      []request.LoadBalancerIPAddress      `json:"ip_addresses"`
}

func (r *replaceLoadBalancerRequest) RequestURL() string {
	return fmt.Sprintf("/load-balancer/%s", r.UUID)
}

func createLoadBalancerRequest(service *v1.Service, nodes []*v1.Node, plan upcloud.LoadBalancerPlan, privateNetworkUUID, name, zone string) (*request.CreateLoadBalancerRequest, error) {
	if err := validateServiceExternalTrafficPolicy(service); err != nil {
		err := errors.Join(err, errUnsupportedConfiguration)
		return nil, err
	}

	r := &request.CreateLoadBalancerRequest{
		Name:            name,
		Plan:            plan.Name,
		Zone:            zone,
		MaintenanceDOW:  "",
		MaintenanceTime: "",
		Networks: []request.LoadBalancerNetwork{
			{
				Name:   networkNamePublic,
				Type:   upcloud.LoadBalancerNetworkTypePublic,
				Family: upcloud.LoadBalancerAddressFamilyIPv4,
			},
			{
				Name:   networkNamePrivate,
				Type:   upcloud.LoadBalancerNetworkTypePrivate,
				Family: upcloud.LoadBalancerAddressFamilyIPv4,
				UUID:   privateNetworkUUID,
			},
		},
		ConfiguredStatus: upcloud.LoadBalancerConfiguredStatusStarted,
		Resolvers:        make([]request.LoadBalancerResolver, 0),
		IPAddresses:      make([]request.LoadBalancerIPAddress, 0),
	}
	r.Frontends = make([]request.LoadBalancerFrontend, len(service.Spec.Ports))
	r.Backends = make([]request.LoadBalancerBackend, len(service.Spec.Ports))

	// For every port configured in service object
	// we create a frontend config and corresponding backend config
	for i := range service.Spec.Ports {
		r.Frontends[i] = loadBalancerFrontendFromServicePort(&service.Spec.Ports[i])
		backend := loadBalancerBackendFromServicePort(&service.Spec.Ports[i], nodes, plan)
		if service.Spec.ExternalTrafficPolicy == v1.ServiceExternalTrafficPolicyTypeLocal {
			// When ETP is set to `Local`, Kubernetes (proxy) provides health check service running in the worker nodes,
			// that can be used to query service state on the particular node.
			// Health check returns HTTP code 503 (Service Unavailable) if service is not currently running on the node.
			// If not provided, `HealthCheckNodePort` is automatically allocated.
			backend.Properties.HealthCheckType = upcloud.LoadBalancerHealthCheckTypeHTTP
			backend.Properties.HealthCheckURL = serviceHealthCheckURL(service)
		}
		r.Backends[i] = backend
	}

	return r, nil
}

func createLoadBalancerRequestLabels(service *v1.Service, clusterID, clusterName, loadBalancerName string) upcloud.LabelSlice {
	return []upcloud.Label{
		{
			Key:   clusterIDLabel,
			Value: clusterID,
		},
		{
			Key:   clusterNameLabel,
			Value: clusterName,
		},
		{
			Key:   generatedNameLabel,
			Value: loadBalancerName,
		},
		{
			Key:   serviceExternalTrafficPolicyLabel,
			Value: strings.ToLower(string(service.Spec.ExternalTrafficPolicy)),
		},
	}
}

func validateLoadBalancerNodes(nodes []*v1.Node) error {
	if len(nodes) == 0 { // can't detect worker nodes
		return fmt.Errorf("can't find node(s) in cluster available for load balancing, check node selectors; %w", errNotFound)
	}
	seenNetworks := map[string]struct{}{}
	for _, n := range nodes {
		annotations := n.GetAnnotations()
		if annotations == nil { // node miss annotation
			return fmt.Errorf("node %s is missing annotations", n.Name)
		}
		seenNetworks[annotations[utils.PrivateNetworkUUIDAnnotation]] = struct{}{}
	}
	if len(seenNetworks) != 1 {
		return fmt.Errorf("nodes in the scope for load-balancing are not in the same private network")
	}
	return nil
}

func validateServiceExternalTrafficPolicy(service *v1.Service) error {
	// service.Spec.HealthCheckNodePort should be automatically allocated if not defined, but check it just in case it still gets 0 value.
	// HealthCheckNodePort value 0 means not set.
	if service.Spec.ExternalTrafficPolicy == v1.ServiceExternalTrafficPolicyTypeLocal && service.Spec.HealthCheckNodePort == 0 {
		return errors.New("external traffic policy validation failed, 'HealthCheckNodePort' property not set")
	}
	return nil
}

func updateCreateLoadBalancerRequestPlan(r *request.CreateLoadBalancerRequest, plan upcloud.LoadBalancerPlan) {
	for i := range r.Backends {
		for y := range r.Backends[i].Members {
			r.Backends[i].Members[y].MaxSessions = plan.PerServerMaxSessions
		}
	}
}

func loadBalancerToCreateRequest(lb *upcloud.LoadBalancer) *request.CreateLoadBalancerRequest {
	networks := make([]request.LoadBalancerNetwork, len(lb.Networks))
	for i := range lb.Networks {
		networks[i] = request.LoadBalancerNetwork{
			Name:   lb.Networks[i].Name,
			Type:   lb.Networks[i].Type,
			Family: lb.Networks[i].Family,
			UUID:   lb.Networks[i].UUID,
		}
	}
	frontends := make([]request.LoadBalancerFrontend, len(lb.Frontends))
	for i := range lb.Frontends {
		rules := make([]request.LoadBalancerFrontendRule, len(lb.Frontends[i].Rules))
		for j := range lb.Frontends[i].Rules {
			rules[j] = request.LoadBalancerFrontendRule{
				Name:     lb.Frontends[i].Rules[j].Name,
				Priority: lb.Frontends[i].Rules[j].Priority,
				Matchers: lb.Frontends[i].Rules[j].Matchers,
				Actions:  lb.Frontends[i].Rules[j].Actions,
			}
		}
		tlsConfigs := make([]request.LoadBalancerFrontendTLSConfig, len(lb.Frontends[i].TLSConfigs))
		for j := range lb.Frontends[i].TLSConfigs {
			tlsConfigs[j] = request.LoadBalancerFrontendTLSConfig{
				Name:                  lb.Frontends[i].TLSConfigs[j].Name,
				CertificateBundleUUID: lb.Frontends[i].TLSConfigs[j].CertificateBundleUUID,
			}
		}
		frontends[i] = request.LoadBalancerFrontend{
			Name:           lb.Frontends[i].Name,
			Mode:           lb.Frontends[i].Mode,
			Port:           lb.Frontends[i].Port,
			DefaultBackend: lb.Frontends[i].DefaultBackend,
			Rules:          rules,
			TLSConfigs:     tlsConfigs,
			Properties:     lb.Frontends[i].Properties,
			Networks:       lb.Frontends[i].Networks,
		}
	}
	backends := make([]request.LoadBalancerBackend, len(lb.Backends))
	for i := range lb.Backends {
		members := make([]request.LoadBalancerBackendMember, len(lb.Backends[i].Members))
		for j := range lb.Backends[i].Members {
			members[j] = request.LoadBalancerBackendMember{
				Name:        lb.Backends[i].Members[j].Name,
				Weight:      lb.Backends[i].Members[j].Weight,
				MaxSessions: lb.Backends[i].Members[j].MaxSessions,
				Enabled:     lb.Backends[i].Members[j].Enabled,
				Type:        lb.Backends[i].Members[j].Type,
				IP:          lb.Backends[i].Members[j].IP,
				Port:        lb.Backends[i].Members[j].Port,
			}
		}
		tlsConfigs := make([]request.LoadBalancerBackendTLSConfig, len(lb.Backends[i].TLSConfigs))
		for j := range lb.Backends[i].TLSConfigs {
			tlsConfigs[i] = request.LoadBalancerBackendTLSConfig{
				Name:                  lb.Backends[i].TLSConfigs[j].Name,
				CertificateBundleUUID: lb.Backends[i].TLSConfigs[j].CertificateBundleUUID,
			}
		}
		backends[i] = request.LoadBalancerBackend{
			Name:       lb.Backends[i].Name,
			Resolver:   lb.Backends[i].Resolver,
			Properties: lb.Backends[i].Properties,
			Members:    members,
			TLSConfigs: tlsConfigs,
		}
	}
	resolvers := make([]request.LoadBalancerResolver, len(lb.Resolvers))
	for i := range resolvers {
		resolvers[i] = request.LoadBalancerResolver{
			Name:         lb.Resolvers[i].Name,
			Nameservers:  lb.Resolvers[i].Nameservers,
			Retries:      lb.Resolvers[i].Retries,
			Timeout:      lb.Resolvers[i].Timeout,
			TimeoutRetry: lb.Resolvers[i].TimeoutRetry,
			CacheValid:   lb.Resolvers[i].CacheValid,
			CacheInvalid: lb.Resolvers[i].CacheInvalid,
		}
	}
	ipAddresses := make([]request.LoadBalancerIPAddress, len(lb.IPAddresses))
	for i := range ipAddresses {
		ipAddresses[i] = request.LoadBalancerIPAddress{
			NetworkName: lb.IPAddresses[i].NetworkName,
			Address:     lb.IPAddresses[i].Address,
		}
	}
	return &request.CreateLoadBalancerRequest{
		Name:             lb.Name,
		Plan:             lb.Plan,
		Zone:             lb.Zone,
		NetworkUUID:      lb.NetworkUUID,
		Networks:         networks,
		ConfiguredStatus: lb.ConfiguredStatus,
		Frontends:        frontends,
		Backends:         backends,
		Resolvers:        resolvers,
		Labels:           lb.Labels,
		MaintenanceDOW:   lb.MaintenanceDOW,
		MaintenanceTime:  lb.MaintenanceTime,
		IPAddresses:      ipAddresses,
	}
}
