package mock

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/UpCloudLtd/upcloud-go-api/v8/upcloud"
	"github.com/UpCloudLtd/upcloud-go-api/v8/upcloud/request"
	"github.com/google/uuid"
)

type UpCloudService struct {
	servers            []upcloud.ServerDetails
	loadBalancers      []upcloud.LoadBalancer
	certificateBundles []upcloud.LoadBalancerCertificateBundle

	mu sync.Mutex
}

func (u *UpCloudService) GetServers(_ context.Context) (*upcloud.Servers, error) {
	u.mu.Lock()
	defer u.mu.Unlock()

	s := make([]upcloud.Server, len(u.servers))
	for i := range u.servers {
		s[i] = u.servers[i].Server
	}
	return &upcloud.Servers{Servers: s}, nil
}

func (u *UpCloudService) GetServerDetails(_ context.Context, r *request.GetServerDetailsRequest) (*upcloud.ServerDetails, error) {
	u.mu.Lock()
	defer u.mu.Unlock()
	for i := range u.servers {
		if u.servers[i].UUID == r.UUID {
			return &u.servers[i], nil
		}
	}
	return nil, &upcloud.Problem{Status: http.StatusNotFound}
}

func (u *UpCloudService) CreateLoadBalancer(_ context.Context, r *request.CreateLoadBalancerRequest) (*upcloud.LoadBalancer, error) {
	u.mu.Lock()
	defer u.mu.Unlock()
	networks := make([]upcloud.LoadBalancerNetwork, len(r.Networks))
	for i := range r.Networks {
		var id string
		if r.Networks[i].Type == upcloud.LoadBalancerNetworkTypePrivate {
			id = uuid.NewString()
		}
		networks[i] = upcloud.LoadBalancerNetwork{
			Type:   r.Networks[i].Type,
			Name:   r.Networks[i].Name,
			UUID:   id,
			Family: r.Networks[i].Family,
		}
	}
	frontends := make([]upcloud.LoadBalancerFrontend, len(r.Frontends))
	for i := range r.Frontends {
		frontends[i] = upcloud.LoadBalancerFrontend{Name: r.Frontends[i].Name}
	}
	backends := make([]upcloud.LoadBalancerBackend, len(r.Backends))
	for i := range r.Backends {
		backends[i] = upcloud.LoadBalancerBackend{
			Name:       r.Backends[i].Name,
			Properties: r.Backends[i].Properties,
		}
	}
	ipAddresses := make([]upcloud.LoadBalancerFloatingIPAddress, len(r.IPAddresses))
	for i := range ipAddresses {
		ipAddresses[i] = upcloud.LoadBalancerFloatingIPAddress{
			NetworkName: r.IPAddresses[i].NetworkName,
			Address:     r.IPAddresses[i].Address,
		}
	}

	labels := make([]upcloud.Label, len(r.Labels))
	copy(labels, r.Labels)

	lb := upcloud.LoadBalancer{
		UUID:             uuid.NewString(),
		Name:             r.Name,
		Zone:             r.Zone,
		Plan:             r.Plan,
		NetworkUUID:      r.NetworkUUID,
		Networks:         networks,
		Labels:           labels,
		Frontends:        frontends,
		Backends:         backends,
		CreatedAt:        time.Now().UTC(),
		UpdatedAt:        time.Now().UTC(),
		MaintenanceDOW:   r.MaintenanceDOW,
		MaintenanceTime:  r.MaintenanceTime,
		DNSName:          fmt.Sprintf("%s.example.com", r.Name),
		OperationalState: upcloud.LoadBalancerOperationalStateRunning,
		IPAddresses:      ipAddresses,
		ConfiguredStatus: r.ConfiguredStatus,
	}
	u.loadBalancers = append(u.loadBalancers, lb)
	return &lb, nil
}

func (u *UpCloudService) GetLoadBalancer(_ context.Context, r *request.GetLoadBalancerRequest) (*upcloud.LoadBalancer, error) {
	u.mu.Lock()
	defer u.mu.Unlock()
	for i := range u.loadBalancers {
		if u.loadBalancers[i].UUID == r.UUID {
			lb := upcloud.LoadBalancer{}
			copyLoadBalancer(&lb, &u.loadBalancers[i])
			return &lb, nil
		}
	}
	return nil, &upcloud.Problem{Status: http.StatusNotFound}
}

func copyLoadBalancer(dst, src *upcloud.LoadBalancer) {
	j, err := json.Marshal(src)
	if err != nil {
		panic(err)
	}
	if err = json.Unmarshal(j, &dst); err != nil {
		panic(err)
	}
}

func (u *UpCloudService) GetLoadBalancers(_ context.Context, _ *request.GetLoadBalancersRequest) ([]upcloud.LoadBalancer, error) {
	return u.loadBalancers, nil
}

func (u *UpCloudService) GetLoadBalancerPlans(_ context.Context, _ *request.GetLoadBalancerPlansRequest) ([]upcloud.LoadBalancerPlan, error) {
	return []upcloud.LoadBalancerPlan{{Name: "development"}}, nil
}

func (u *UpCloudService) GetZones(_ context.Context) (*upcloud.Zones, error) {
	return &upcloud.Zones{Zones: []upcloud.Zone{{ID: "fi-hel2"}}}, nil
}

func (u *UpCloudService) CreateLoadBalancerFrontend(_ context.Context, r *request.CreateLoadBalancerFrontendRequest) (*upcloud.LoadBalancerFrontend, error) {
	u.mu.Lock()
	defer u.mu.Unlock()
	for i := range u.loadBalancers {
		if u.loadBalancers[i].UUID == r.ServiceUUID {
			fe := u.newLoadBalancerFrontendFromRequest(&r.Frontend)
			u.loadBalancers[i].Frontends = append(u.loadBalancers[i].Frontends, fe)
			return &fe, nil
		}
	}
	return nil, &upcloud.Problem{Status: http.StatusNotFound}
}

func (u *UpCloudService) newLoadBalancerFrontendFromRequest(r *request.LoadBalancerFrontend) upcloud.LoadBalancerFrontend {
	tlsconfigs := make([]upcloud.LoadBalancerFrontendTLSConfig, 0)
	for i := range r.TLSConfigs {
		for j := range u.certificateBundles {
			if u.certificateBundles[j].Name == r.TLSConfigs[i].Name {
				tlsconfigs = append(tlsconfigs, upcloud.LoadBalancerFrontendTLSConfig{
					Name:                  r.TLSConfigs[i].Name,
					CertificateBundleUUID: u.certificateBundles[j].UUID,
				})
			}
		}
	}
	return upcloud.LoadBalancerFrontend{
		Name:           r.Name,
		TLSConfigs:     tlsconfigs,
		Mode:           r.Mode,
		Port:           r.Port,
		DefaultBackend: r.DefaultBackend,
		Networks:       r.Networks,
		Properties:     r.Properties,
	}
}

func (u *UpCloudService) CreateLoadBalancerCertificateBundle(_ context.Context, r *request.CreateLoadBalancerCertificateBundleRequest) (*upcloud.LoadBalancerCertificateBundle, error) {
	u.mu.Lock()
	defer u.mu.Unlock()

	for i := range u.certificateBundles {
		if u.certificateBundles[i].Name == r.Name {
			return nil, &upcloud.Problem{Status: http.StatusBadRequest, Title: "cert bundle already exists"}
		}
	}

	hostnames := make([]string, len(r.Hostnames)+1)
	copy(hostnames, r.Hostnames)
	// Append random hostname to ensure that we support also bundles with custom hostnames.
	hostnames[len(r.Hostnames)] = "example.com"
	b := upcloud.LoadBalancerCertificateBundle{
		UUID:      uuid.NewString(),
		Name:      r.Name,
		Type:      r.Type,
		Hostnames: hostnames,
	}
	u.certificateBundles = append(u.certificateBundles, b)
	return &b, nil
}

func (u *UpCloudService) GetLoadBalancerCertificateBundles(_ context.Context, _ *request.GetLoadBalancerCertificateBundlesRequest) ([]upcloud.LoadBalancerCertificateBundle, error) {
	return u.certificateBundles, nil
}

func (u *UpCloudService) DeleteLoadBalancerFrontend(_ context.Context, _ *request.DeleteLoadBalancerFrontendRequest) error {
	return nil
}

func (u *UpCloudService) DeleteLoadBalancerCertificateBundle(_ context.Context, _ *request.DeleteLoadBalancerCertificateBundleRequest) error {
	return nil
}

func (u *UpCloudService) DeleteLoadBalancer(ctx context.Context, r *request.DeleteLoadBalancerRequest) error {
	_, err := u.GetLoadBalancer(ctx, &request.GetLoadBalancerRequest{UUID: r.UUID})
	return err
}

func NewUpCloudService(server ...upcloud.ServerDetails) *UpCloudService {
	return &UpCloudService{servers: server}
}
