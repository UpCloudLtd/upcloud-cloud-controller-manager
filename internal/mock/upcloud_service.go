package mock

import (
	"context"
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
			Type: r.Networks[i].Type,
			Name: r.Networks[i].Name,
			UUID: id,
		}
	}
	frontends := make([]upcloud.LoadBalancerFrontend, len(r.Frontends))
	for i := range r.Frontends {
		frontends[i] = upcloud.LoadBalancerFrontend{Name: r.Frontends[i].Name}
	}
	backends := make([]upcloud.LoadBalancerBackend, len(r.Backends))
	for i := range r.Backends {
		backends[i] = upcloud.LoadBalancerBackend{Name: r.Backends[i].Name}
	}
	lb := upcloud.LoadBalancer{
		UUID:             uuid.NewString(),
		Name:             r.Name,
		Zone:             r.Zone,
		Plan:             r.Plan,
		NetworkUUID:      r.NetworkUUID,
		Networks:         networks,
		Labels:           r.Labels,
		Frontends:        frontends,
		Backends:         backends,
		CreatedAt:        time.Now().UTC(),
		UpdatedAt:        time.Now().UTC(),
		MaintenanceDOW:   r.MaintenanceDOW,
		MaintenanceTime:  r.MaintenanceTime,
		DNSName:          fmt.Sprintf("%s.example.com", r.Name),
		OperationalState: upcloud.LoadBalancerOperationalStateRunning,
	}
	u.loadBalancers = append(u.loadBalancers, lb)
	return &lb, nil
}

func (u *UpCloudService) GetLoadBalancer(_ context.Context, r *request.GetLoadBalancerRequest) (*upcloud.LoadBalancer, error) {
	u.mu.Lock()
	defer u.mu.Unlock()
	for i := range u.loadBalancers {
		if u.loadBalancers[i].UUID == r.UUID {
			return &u.loadBalancers[i], nil
		}
	}
	return nil, &upcloud.Problem{Status: http.StatusNotFound}
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
	return &upcloud.LoadBalancerFrontend{Name: r.Frontend.Name}, nil
}

func (u *UpCloudService) CreateLoadBalancerCertificateBundle(_ context.Context, r *request.CreateLoadBalancerCertificateBundleRequest) (*upcloud.LoadBalancerCertificateBundle, error) {
	u.mu.Lock()
	defer u.mu.Unlock()
	b := upcloud.LoadBalancerCertificateBundle{Name: r.Name, Type: r.Type}
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
