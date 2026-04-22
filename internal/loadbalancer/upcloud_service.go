package loadbalancer

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"

	"github.com/UpCloudLtd/upcloud-cloud-controller-manager/internal/utils"
	"github.com/UpCloudLtd/upcloud-go-api/v8/upcloud"
	"github.com/UpCloudLtd/upcloud-go-api/v8/upcloud/request"
	"github.com/google/uuid"
	v1 "k8s.io/api/core/v1"
)

var errCertificateBundleNotFound = errors.New("certificate bundle not found")

type UpCloudService interface {
	CreateLoadBalancer(ctx context.Context, r *request.CreateLoadBalancerRequest) (*upcloud.LoadBalancer, error)
	CreateLoadBalancerFrontend(ctx context.Context, r *request.CreateLoadBalancerFrontendRequest) (*upcloud.LoadBalancerFrontend, error)
	CreateLoadBalancerCertificateBundle(ctx context.Context, r *request.CreateLoadBalancerCertificateBundleRequest) (*upcloud.LoadBalancerCertificateBundle, error)
	GetLoadBalancer(ctx context.Context, r *request.GetLoadBalancerRequest) (*upcloud.LoadBalancer, error)
	GetLoadBalancers(ctx context.Context, r *request.GetLoadBalancersRequest) ([]upcloud.LoadBalancer, error)
	GetLoadBalancerCertificateBundles(ctx context.Context, r *request.GetLoadBalancerCertificateBundlesRequest) ([]upcloud.LoadBalancerCertificateBundle, error)
	GetServerDetails(ctx context.Context, r *request.GetServerDetailsRequest) (*upcloud.ServerDetails, error)
	DeleteLoadBalancerFrontend(ctx context.Context, r *request.DeleteLoadBalancerFrontendRequest) error
	DeleteLoadBalancerCertificateBundle(ctx context.Context, r *request.DeleteLoadBalancerCertificateBundleRequest) error
	DeleteLoadBalancer(ctx context.Context, r *request.DeleteLoadBalancerRequest) error
	GetLoadBalancerPlans(ctx context.Context, r *request.GetLoadBalancerPlansRequest) ([]upcloud.LoadBalancerPlan, error)
	GetZones(ctx context.Context) (*upcloud.Zones, error)
}

type UpCloudClient interface {
	Put(ctx context.Context, path string, body []byte) ([]byte, error)
}

type Service interface {
	GetLoadBalancer(ctx context.Context, clusterName string, service *v1.Service) (*upcloud.LoadBalancer, error)
	CreateLoadBalancer(ctx context.Context, service *v1.Service, nodes []*v1.Node, clusterName string) (*upcloud.LoadBalancer, error)
	DeleteLoadBalancer(ctx context.Context, lb *upcloud.LoadBalancer) error
	UpdateLoadBalancer(ctx context.Context, lb *upcloud.LoadBalancer, service *v1.Service, nodes []*v1.Node, clusterName string) error
}

type upCloudLoadBalancer struct {
	upcs   UpCloudService
	upcc   UpCloudClient
	config *Config
}

func (u *upCloudLoadBalancer) GetLoadBalancer(ctx context.Context, clusterName string, service *v1.Service) (*upcloud.LoadBalancer, error) {
	lbID := loadBalancerIDFromService(service)
	if lbID != uuid.Nil {
		return u.getLoadBalancerByID(ctx, lbID)
	}
	loadBalancerName := ""
	if annotation := service.GetAnnotations(); annotation != nil {
		loadBalancerName = annotation[loadBalancerNameAnnotation]
	}
	return u.getLoadBalancerByLabels(ctx,
		request.FilterLabel{Label: upcloud.Label{
			Key:   clusterIDLabel,
			Value: u.config.clusterID,
		}},
		request.FilterLabel{Label: upcloud.Label{
			Key:   clusterNameLabel,
			Value: clusterName,
		}},
		request.FilterLabel{Label: upcloud.Label{
			Key:   generatedNameLabel,
			Value: loadBalancerName,
		}},
	)
}

func (u *upCloudLoadBalancer) getLoadBalancerByLabels(ctx context.Context, filter ...request.QueryFilter) (*upcloud.LoadBalancer, error) {
	if len(filter) == 0 {
		return nil, fmt.Errorf("not enough information to fetch load balancer using labels; %w", errNotFound)
	}
	loadbalancers, err := u.upcs.GetLoadBalancers(ctx, &request.GetLoadBalancersRequest{Filters: filter})
	if err != nil {
		return nil, fmt.Errorf("failed load balancers with labels %s; %w", queryFilterToString(filter), err)
	}
	if len(loadbalancers) == 0 {
		return nil, fmt.Errorf("failed to load balancer with labels %s; %w", queryFilterToString(filter), errNotFound)
	}
	if len(loadbalancers) > 1 {
		return nil, fmt.Errorf("load balancer with labels %s returned multiple results; %w", queryFilterToString(filter), errAmbiguity)
	}
	return &loadbalancers[0], nil
}

func (u *upCloudLoadBalancer) getLoadBalancerByID(ctx context.Context, loadBalancerID uuid.UUID) (*upcloud.LoadBalancer, error) {
	lb, err := u.upcs.GetLoadBalancer(ctx, &request.GetLoadBalancerRequest{UUID: loadBalancerID.String()})
	if err != nil {
		if utils.ErrIsHTTPStatusNotFound(err) {
			return nil, fmt.Errorf("%w: load balancer with id %s: %v", errNotFound, loadBalancerID.String(), err)
		}
		return nil, fmt.Errorf("can't get load balancer info with id %s: %w", loadBalancerID.String(), err)
	}
	return lb, nil
}

func (u *upCloudLoadBalancer) CreateLoadBalancer(ctx context.Context, service *v1.Service, nodes []*v1.Node, clusterName string) (*upcloud.LoadBalancer, error) {
	config, err := u.createRequestFromService(ctx, service, nodes, clusterName)
	if err != nil {
		return nil, fmt.Errorf("failed to create new load balancer request; %w", err)
	}
	frontends := config.Frontends
	config.Frontends = make([]request.LoadBalancerFrontend, 0)
	lb, err := u.upcs.CreateLoadBalancer(ctx, config)
	if err != nil {
		return lb, err
	}
	if err := u.autoProvisionCertificates(ctx, lb, frontends); err != nil {
		return lb, fmt.Errorf("failed to auto provision TLS certificates; %w", err)
	}
	for i := range frontends {
		if _, err := u.upcs.CreateLoadBalancerFrontend(ctx, &request.CreateLoadBalancerFrontendRequest{
			ServiceUUID: lb.UUID,
			Frontend:    frontends[i],
		}); err != nil {
			return lb, fmt.Errorf("failed to create load balancer %s frontend; %w", lb.Name, err)
		}
	}
	return lb, err
}

func (u *upCloudLoadBalancer) createRequestFromService(ctx context.Context, service *v1.Service, nodes []*v1.Node, clusterName string) (*request.CreateLoadBalancerRequest, error) {
	if err := validateLoadBalancerNodes(nodes); err != nil {
		err := errors.Join(err, errUnsupportedConfiguration)
		return nil, fmt.Errorf("validating nodes failed; %w", err)
	}
	// fetch server details of the single node to determine zone and private network for load balancer
	server, err := u.serverDetails(ctx, nodes[0])
	if err != nil {
		return nil, err
	}
	privateInterfaces := utils.ServerInterfacesByType(server, upcloud.NetworkTypePrivate)
	if len(privateInterfaces) < 1 {
		return nil, fmt.Errorf("private network interface not found for node %s", nodes[0].Name)
	}
	zone, err := u.serviceZone(ctx, server.Zone)
	if err != nil {
		return nil, err
	}
	return u.newCreateRequest(ctx, service, nodes, clusterName, privateInterfaces[0].Network, zone)
}

func (u *upCloudLoadBalancer) UpdateLoadBalancer(ctx context.Context, lb *upcloud.LoadBalancer, service *v1.Service, nodes []*v1.Node, clusterName string) error {
	if lb == nil {
		return fmt.Errorf("failed to update load balancer, load balancer object not provided; %w", errAmbiguity)
	}

	privateInterface, err := loadBalancerPrivateNetwork(lb)
	if err != nil {
		return fmt.Errorf("failed to update load balancer; %w", err)
	}
	config, err := u.newCreateRequest(ctx, service, nodes, clusterName, privateInterface.UUID, lb.Zone)
	if err != nil {
		return fmt.Errorf("failed to create new load balancer request; %w", err)
	}
	if err := u.autoProvisionCertificates(ctx, lb, config.Frontends); err != nil {
		return fmt.Errorf("failed to auto provision TLS certificates; %w", err)
	}
	notEqualErr := createLoadBalancerRequestsEqual(loadBalancerToCreateRequest(lb), config)
	if notEqualErr == nil {
		return nil
	}
	r := replaceLoadBalancerRequest{
		UUID:             lb.UUID,
		Name:             config.Name,
		Plan:             config.Plan,
		ConfiguredStatus: config.ConfiguredStatus,
		Labels:           config.Labels,
		Frontends:        config.Frontends,
		Backends:         config.Backends,
		Resolvers:        config.Resolvers,
		MaintenanceDOW:   config.MaintenanceDOW,
		MaintenanceTime:  config.MaintenanceTime,
		IPAddresses:      config.IPAddresses,
	}
	b, err := json.Marshal(r)
	if err != nil {
		return fmt.Errorf("error marshaling load balancer update request: %w", err)
	}
	if _, err := u.upcc.Put(ctx, r.RequestURL(), b); err != nil {
		return fmt.Errorf("error updating load balancer: %w", errorAsProblem(err))
	}
	return notEqualErr
}

func (u *upCloudLoadBalancer) newCreateRequest(ctx context.Context, service *v1.Service, nodes []*v1.Node, clusterName, privateNetworkUUID, zone string) (*request.CreateLoadBalancerRequest, error) {
	plans, err := u.plans(ctx)
	if err != nil {
		return nil, err
	}
	namePrefix := clusterName
	if u.config.clusterID != "" {
		namePrefix = u.config.clusterID
	}
	name := loadBalancerName(service.GetNamespace(), service.GetName(), namePrefix)
	config, err := createLoadBalancerRequest(service, nodes, plans[u.config.loadBalancerPlan], privateNetworkUUID, name, zone)
	if err != nil {
		return nil, err
	}
	config.Labels = createLoadBalancerRequestLabels(service, u.config.clusterID, clusterName, name)
	if err = mergeLoadBalancerConfigFromServiceAnnotations(service, config, plans); err != nil {
		return nil, err
	}
	return config, nil
}

func (u *upCloudLoadBalancer) serverDetails(ctx context.Context, node *v1.Node) (*upcloud.ServerDetails, error) {
	serverUUID, err := utils.ServerUUIDFromNode(node)
	if err != nil {
		return nil, fmt.Errorf("resolving server ID from node %s failed; %w", node.Name, err)
	}
	server, err := u.upcs.GetServerDetails(ctx, &request.GetServerDetailsRequest{UUID: serverUUID.String()})
	if err != nil {
		return nil, fmt.Errorf("fetching server details for node %s failed; %w", node.Name, err)
	}
	return server, nil
}

func (u *upCloudLoadBalancer) plans(ctx context.Context) (map[string]upcloud.LoadBalancerPlan, error) {
	p, err := u.upcs.GetLoadBalancerPlans(ctx, &request.GetLoadBalancerPlansRequest{})
	if err != nil {
		return nil, err
	}
	plans := make(map[string]upcloud.LoadBalancerPlan)
	for i := range p {
		plans[p[i].Name] = p[i]
	}
	return plans, nil
}

func (u *upCloudLoadBalancer) serviceZone(ctx context.Context, zone string) (string, error) {
	z, err := u.upcs.GetZones(ctx)
	if err != nil {
		return "", fmt.Errorf("can't retrieve UpCloud zone details: %w", err)
	}
	// LBs can be deployed only to public zones so we need to use parent zone if nodes are running in private zone.
	for i := range z.Zones {
		if z.Zones[i].ID == zone {
			if !z.Zones[i].Public.Bool() && z.Zones[i].ParentZone != "" {
				return z.Zones[i].ParentZone, nil
			}
			return zone, nil
		}
	}
	return zone, nil
}

// autoProvisionCertificates if there is frontend listening 443 port in HTTP mode, without TLS configs defined.
func (u *upCloudLoadBalancer) autoProvisionCertificates(ctx context.Context, lb *upcloud.LoadBalancer, frontends []request.LoadBalancerFrontend) error {
	for i := range frontends {
		if certificateAutoProvisioningNeeded(frontends[i]) {
			bundleName := loadBalancerCertBundleName(loadBalancerGeneratedName(lb))
			bundle, err := u.getOrCreateCertificateBundle(ctx, bundleName, loadBalancerDNSName(lb))
			if err != nil {
				return err
			}
			frontends[i].TLSConfigs = []request.LoadBalancerFrontendTLSConfig{
				{
					Name:                  bundle.Name,
					CertificateBundleUUID: bundle.UUID,
				},
			}
		}
	}
	return nil
}

func (u *upCloudLoadBalancer) getOrCreateCertificateBundle(ctx context.Context, name string, hostname ...string) (*upcloud.LoadBalancerCertificateBundle, error) {
	b, err := u.getCertificateBundleByName(ctx, name, upcloud.LoadBalancerCertificateBundleTypeDynamic)
	if err != nil && !errors.Is(err, errCertificateBundleNotFound) {
		return b, err
	}
	if b != nil {
		// Double check that this certificate contains hostnames that we expect.
		// There is possibility that generated `name` is duplicate due to how name shortener works currently.
		slices.Sort(hostname)
		slices.Sort(b.Hostnames)
		if slices.Equal(b.Hostnames, hostname) {
			return b, nil
		}
	}

	return u.upcs.CreateLoadBalancerCertificateBundle(ctx, &request.CreateLoadBalancerCertificateBundleRequest{
		Type:      upcloud.LoadBalancerCertificateBundleTypeDynamic,
		KeyType:   "ecdsa",
		Name:      name,
		Hostnames: hostname,
	})
}

func (u *upCloudLoadBalancer) getCertificateBundleByName(ctx context.Context, name string, budleType upcloud.LoadBalancerCertificateBundleType) (*upcloud.LoadBalancerCertificateBundle, error) {
	bundles, err := u.upcs.GetLoadBalancerCertificateBundles(ctx, &request.GetLoadBalancerCertificateBundlesRequest{})
	if err != nil {
		return nil, err
	}
	for _, b := range bundles {
		if b.Name == name && b.Type == budleType {
			return &b, nil
		}
	}
	return nil, errCertificateBundleNotFound
}

func (u *upCloudLoadBalancer) DeleteLoadBalancer(ctx context.Context, lb *upcloud.LoadBalancer) error {
	bundleName := loadBalancerCertBundleName(loadBalancerGeneratedName(lb))

	// Remove TLS frontends first, so that auto provisioned TLS budle can be deleted.
	for _, frontend := range lb.Frontends {
		if len(frontend.TLSConfigs) == 0 {
			continue
		}
		if err := u.upcs.DeleteLoadBalancerFrontend(ctx, &request.DeleteLoadBalancerFrontendRequest{
			ServiceUUID: lb.UUID,
			Name:        frontend.Name,
		}); err != nil {
			return fmt.Errorf("failed to delete load balancer frontend %s; %w", frontend.Name, err)
		}
	}
	bundle, err := u.getCertificateBundleByName(ctx, bundleName, upcloud.LoadBalancerCertificateBundleTypeDynamic)
	if err != nil && !errors.Is(err, errCertificateBundleNotFound) {
		return fmt.Errorf("failed to delete load balancer; %w", err)
	}
	if bundle != nil {
		err := u.upcs.DeleteLoadBalancerCertificateBundle(ctx, &request.DeleteLoadBalancerCertificateBundleRequest{UUID: bundle.UUID})
		// Bundle can be already deleted (404) or it might be used by other LB (409).
		if err != nil && !utils.ErrIsHTTPStatusNotFound(err) && !errIsHTTPStatusConflict(err) {
			return fmt.Errorf("failed to delete load balancer certificate bundle; %w", err)
		}
	}
	if err := u.upcs.DeleteLoadBalancer(ctx, &request.DeleteLoadBalancerRequest{UUID: lb.UUID}); err != nil && !utils.ErrIsHTTPStatusNotFound(err) {
		return fmt.Errorf("failed to delete load balancer; %w", err)
	}

	return nil
}

func NewLoadBalancerService(upcs UpCloudService, upcc UpCloudClient, config *Config) Service {
	return &upCloudLoadBalancer{upcs: upcs, config: config, upcc: upcc}
}
