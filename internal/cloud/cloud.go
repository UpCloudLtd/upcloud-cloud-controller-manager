package cloud

import (
	"io"

	"github.com/UpCloudLtd/upcloud-cloud-controller-manager/internal/instance"
	"github.com/UpCloudLtd/upcloud-cloud-controller-manager/internal/loadbalancer"
	"github.com/UpCloudLtd/upcloud-cloud-controller-manager/internal/logger"
	upc "github.com/UpCloudLtd/upcloud-go-api/v8/upcloud/client"
	"github.com/UpCloudLtd/upcloud-go-api/v8/upcloud/service"
	v1 "k8s.io/api/core/v1"
	"k8s.io/client-go/kubernetes/scheme"
	v1types "k8s.io/client-go/kubernetes/typed/core/v1"
	"k8s.io/client-go/tools/record"
	cloudprovider "k8s.io/cloud-provider"
)

const ProviderName string = "upcloud"

// cloud implements cloudprovider.Interface
type cloud struct {
	cfg           config
	instances     cloudprovider.InstancesV2
	loadbalancers cloudprovider.LoadBalancer
	log           logger.Logger
}

// Initialize provides the cloud with a kubernetes client builder and may spawn goroutines
// to perform housekeeping or run custom controllers specific to the cloud provider.
// Any tasks started here should be cleaned up when the stop channel closes.
func (c *cloud) Initialize(clientBuilder cloudprovider.ControllerClientBuilder, _ <-chan struct{}) {
	c.log.Infof("initializing cloud manager for cluster %s", c.cfg.Data.ClusterID)
	client := clientBuilder.ClientOrDie("upcloud-shared-informers")
	broadcaster := record.NewBroadcaster()
	broadcaster.StartStructuredLogging(0)
	broadcaster.StartRecordingToSink(&v1types.EventSinkImpl{Interface: client.CoreV1().Events("")})
	eventRecorder := broadcaster.NewRecorder(scheme.Scheme, v1.EventSource{Component: "service-controller"})

	upCloudClient := upc.New(c.cfg.Data.APICredentials.User, c.cfg.Data.APICredentials.Password)
	upCloudService := service.New(upCloudClient)

	c.instances = instance.NewInstancesManager(
		upCloudService,
		c.cfg.Data.nodeScopeLabelSelector,
		client.CoreV1().Nodes(),
		c.log,
	)

	loadbalancerConfig := loadbalancer.NewConfig(
		c.cfg.Data.ClusterID,
		c.cfg.Data.LoadBalancerPlan,
		c.cfg.Data.LoadBalancerMaxBackendMembers,
	)
	loadBalancerService := loadbalancer.NewLoadBalancerService(upCloudService, upCloudClient, loadbalancerConfig)
	c.loadbalancers = loadbalancer.NewLoadBalancerManager(
		loadBalancerService,
		loadbalancerConfig,
		client.CoreV1(),
		eventRecorder,
		c.log,
	)
}

// LoadBalancer returns a balancer interface. Also returns true if the interface is supported, false otherwise.
func (c *cloud) LoadBalancer() (cloudprovider.LoadBalancer, bool) {
	if c.loadbalancers != nil {
		return c.loadbalancers, true
	}
	return nil, false
}

// Instances returns an instances interface. Also returns true if the interface is supported, false otherwise.
func (c *cloud) Instances() (cloudprovider.Instances, bool) {
	return nil, false
}

// InstancesV2 is an implementation for instances and should only be implemented by external cloud providers.
// Implementing InstancesV2 is behaviorally identical to Instances but is optimized to significantly reduce
// API calls to the cloud provider when registering and syncing nodes. Implementation of this interface will
// disable calls to the Zones interface. Also returns true if the interface is supported, false otherwise.
func (c *cloud) InstancesV2() (cloudprovider.InstancesV2, bool) {
	if c.instances != nil {
		return c.instances, true
	}
	return nil, false
}

// Zones returns a zones interface. Also returns true if the interface is supported, false otherwise.
// This interface will not be called if InstancesV2 is enabled.

// Deprecated: Zones is deprecated in favor of retrieving zone/region information from InstancesV2.
func (c *cloud) Zones() (cloudprovider.Zones, bool) {
	return nil, false
}

// Clusters returns a clusters interface.  Also returns true if the interface is supported, false otherwise.
func (c *cloud) Clusters() (cloudprovider.Clusters, bool) {
	return nil, false
}

// Routes returns a routes interface along with whether the interface is supported.
func (c *cloud) Routes() (cloudprovider.Routes, bool) {
	return nil, false
}

// ProviderName returns the cloud provider ID.
func (c *cloud) ProviderName() string {
	return ProviderName
}

// HasClusterID returns true if a ClusterID is required and set.
func (c *cloud) HasClusterID() bool {
	return c.cfg.Data.ClusterID != ""
}

func NewCloudProviderFromConfig(f io.Reader) (cloudprovider.Interface, error) {
	c, err := newConfig(f)
	if err != nil {
		return nil, err
	}
	return &cloud{
		cfg: c,
		log: logger.NewKlog(),
	}, nil
}

func init() {
	cloudprovider.RegisterCloudProvider(ProviderName, NewCloudProviderFromConfig)
}
