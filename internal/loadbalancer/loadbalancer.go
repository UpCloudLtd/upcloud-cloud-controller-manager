package loadbalancer

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/UpCloudLtd/upcloud-cloud-controller-manager/internal/logger"
	"github.com/UpCloudLtd/upcloud-cloud-controller-manager/internal/utils"
	"github.com/UpCloudLtd/upcloud-go-api/v8/upcloud"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/types"
	corev1 "k8s.io/client-go/kubernetes/typed/core/v1"
	"k8s.io/client-go/tools/record"
	cloudprovider "k8s.io/cloud-provider"
	"k8s.io/cloud-provider/api"
)

var errUnsupportedConfiguration = errors.New("unsupported configuration")

// manager is load balancer manager that implements cloudprovider.LoadBalancer interface.
type manager struct {
	svc           Service
	nodeSelector  labels.Selector
	eventRecorder record.EventRecorder
	coreV1Client  corev1.CoreV1Interface

	config *Config

	log logger.Logger
	mu  sync.Mutex
}

// GetLoadBalancer returns whether the specified load balancer exists, and
// if so, what its status is.
// Implementations must treat the *v1.Service parameter as read-only and not modify it.
// Parameter 'clusterName' is the name of the cluster as presented to kube-controller-manager
func (m *manager) GetLoadBalancer(ctx context.Context, clusterName string, service *v1.Service) (status *v1.LoadBalancerStatus, exists bool, err error) {
	m.log.Infof("getting load balancer %s/%s status", clusterName, service.GetName())
	lb, err := m.svc.GetLoadBalancer(ctx, clusterName, service)
	if err != nil {
		if errors.Is(err, errNotFound) {
			return nil, false, nil
		}
		return nil, false, err
	}
	return &v1.LoadBalancerStatus{
		Ingress: []v1.LoadBalancerIngress{
			{
				Hostname: loadBalancerDNSName(lb),
			},
		},
	}, true, nil
}

// GetLoadBalancerName returns the name of the load balancer. Implementations must treat the
// *v1.Service parameter as read-only and not modify it.
func (m *manager) GetLoadBalancerName(_ context.Context, clusterName string, service *v1.Service) string {
	m.log.Infof("getting load balancer %s/%s name", clusterName, service.GetName())
	annotations := service.GetAnnotations()
	if annotations == nil {
		return service.Name
	}
	if name, ok := annotations[loadBalancerNameAnnotation]; ok {
		return name
	}
	return service.Name
}

// EnsureLoadBalancer creates a new load balancer 'name', or updates the existing one. Returns the status of the balancer
// Implementations must treat the *v1.Service and *v1.Node
// parameters as read-only and not modify them.
// Parameter 'clusterName' is the name of the cluster as presented to kube-controller-manager
func (m *manager) EnsureLoadBalancer(ctx context.Context, clusterName string, service *v1.Service, nodes []*v1.Node) (*v1.LoadBalancerStatus, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.log.Infof("ensuring load balancer %s/%s", clusterName, service.GetName())
	lb, err := m.svc.GetLoadBalancer(ctx, clusterName, service)
	if err != nil && !errors.Is(err, errNotFound) {
		return nil, err
	}

	// Filter nodes using node selectors, first from the config and then using service annotation if defined.
	nodes = selectNodesByLabels(nodes, m.nodeSelector)
	if len(nodes) > 0 {
		serviceNodeSelector, err := nodeSelectorFromService(service)
		if err != nil {
			return nil, fmt.Errorf("failed to parse node selector from annotations[%s]; %w", loadBalancerNodeSelectorAnnotation, err)
		}
		nodes = selectNodesByLabels(nodes, serviceNodeSelector)
	}
	if len(nodes) > 0 {
		nodes = m.limitNodes(nodes, service)
	}

	if lb != nil {
		return m.ensureExistingLoadBalancer(ctx, clusterName, service, nodes, lb)
	}
	return m.ensureNewLoadBalancer(ctx, clusterName, service, nodes)
}

func (m *manager) ensureExistingLoadBalancer(ctx context.Context, clusterName string, service *v1.Service, nodes []*v1.Node, lb *upcloud.LoadBalancer) (*v1.LoadBalancerStatus, error) {
	if lb.OperationalState != upcloud.LoadBalancerOperationalStateRunning {
		// requeue until LB is running
		return nil, api.NewRetryError(
			fmt.Sprintf("load balancer is not running, current state is %s", lb.OperationalState),
			time.Second*10,
		)
	}
	if err := m.svc.UpdateLoadBalancer(ctx, lb, service, nodes, clusterName); err != nil {
		if !errors.Is(err, errConfigNotEqual) {
			return nil, fmt.Errorf("failed to update load balancer;  %w", err)
		}
		m.log.Infof("updated load balancer %s due to changes in the config (%s)", lb.Name, err)
		m.eventRecorder.Event(service, v1.EventTypeNormal, changesDetectedEventType, fmt.Sprintf("load balancer updated (%s)", err.Error()))
		if lb, err = m.svc.GetLoadBalancer(ctx, clusterName, service); err != nil {
			return nil, api.NewRetryError("failed to refresh load balancer details", time.Second*10)
		}
	} else {
		m.eventRecorder.Event(service, v1.EventTypeNormal, noChangesDetectedEventType, "load balancer is up to date")
	}
	return &v1.LoadBalancerStatus{
		Ingress: []v1.LoadBalancerIngress{
			{
				Hostname: loadBalancerDNSName(lb),
			},
		},
	}, nil
}

func (m *manager) ensureNewLoadBalancer(ctx context.Context, clusterName string, service *v1.Service, nodes []*v1.Node) (*v1.LoadBalancerStatus, error) {
	m.eventRecorder.Event(service, v1.EventTypeNormal, newLoadBalancerEventType, "Creating load balancer")
	lb, err := m.svc.CreateLoadBalancer(ctx, service, nodes, clusterName)
	// Patch service object as soon as we have LB UUID so that we don't loose reference.
	if lb.UUID != "" && !serviceHasAnnotation(service, loadBalancerIDAnnotation) {
		modifiedService := service.DeepCopy()
		updateServiceAnnotations(modifiedService, lb)
		if perr := m.patchService(ctx, service, modifiedService); perr != nil {
			err = errors.Join(err, perr)
		}
	}
	if err != nil {
		return nil, fmt.Errorf("failed to create load balancer;  %w", err)
	}
	if lb.OperationalState != upcloud.LoadBalancerOperationalStateRunning {
		return nil, api.NewRetryError(
			fmt.Sprintf("waiting load balancer to enter running state, current state is %s", lb.OperationalState),
			time.Second*20,
		)
	}
	return &v1.LoadBalancerStatus{
		Ingress: []v1.LoadBalancerIngress{
			{
				Hostname: loadBalancerDNSName(lb),
			},
		},
	}, nil
}

// UpdateLoadBalancer updates hosts under the specified load balancer.
// Implementations must treat the *v1.Service and *v1.Node
// parameters as read-only and not modify them.
// Parameter 'clusterName' is the name of the cluster as presented to kube-controller-manager
func (m *manager) UpdateLoadBalancer(ctx context.Context, clusterName string, service *v1.Service, nodes []*v1.Node) error {
	m.log.Infof("updating load balancer %s/%s", clusterName, service.GetName())
	lb, err := m.svc.GetLoadBalancer(ctx, clusterName, service)
	if err != nil {
		return err
	}
	// Filter nodes using node selectors, first from the config and then using service annotation if defined.
	nodes = selectNodesByLabels(nodes, m.nodeSelector)
	if len(nodes) > 0 {
		serviceNodeSelector, err := nodeSelectorFromService(service)
		if err != nil {
			return fmt.Errorf("failed to parse node selector from annotations[%s]; %w", loadBalancerNodeSelectorAnnotation, err)
		}
		nodes = selectNodesByLabels(nodes, serviceNodeSelector)
	}
	if err := m.svc.UpdateLoadBalancer(ctx, lb, service, nodes, clusterName); err != nil {
		if !errors.Is(err, errConfigNotEqual) {
			return fmt.Errorf("failed to update load balancer;  %w", err)
		}
		m.log.Infof("updated load balancer %s due to changes in the config (%s)", lb.Name, err)
	}
	return nil
}

// EnsureLoadBalancerDeleted deletes the specified load balancer if it
// exists, returning nil if the load balancer specified either didn't exist or
// was successfully deleted.
// This construction is useful because many cloud providers' load balancers
// have multiple underlying components, meaning a Get could say that the LB
// doesn't exist even if some part of it is still laying around.
// Implementations must treat the *v1.Service parameter as read-only and not modify it.
// Parameter 'clusterName' is the name of the cluster as presented to kube-controller-manager
func (m *manager) EnsureLoadBalancerDeleted(ctx context.Context, clusterName string, service *v1.Service) error {
	m.log.Infof("ensuring load balancer %s/%s is deleted", clusterName, service.GetName())
	lb, err := m.svc.GetLoadBalancer(ctx, clusterName, service)
	if err != nil {
		if errors.Is(err, errNotFound) {
			m.log.Infof("load balancer %s/%s not found", clusterName, service.GetName())
			return nil
		}
		return err
	}
	return m.svc.DeleteLoadBalancer(ctx, lb)
}

func (m *manager) patchService(ctx context.Context, original, modified *v1.Service) error {
	patch, err := utils.CreateTwoWayMergePatch(original, modified, v1.Service{})
	if err != nil {
		return fmt.Errorf("failed to create 2-way merge patch: %w", err)
	}
	if len(patch) == 0 || string(patch) == "{}" {
		return nil
	}
	_, err = m.coreV1Client.Services(original.GetNamespace()).Patch(ctx, original.Name, types.StrategicMergePatchType, patch, metav1.PatchOptions{})
	if err != nil {
		return fmt.Errorf("failed to patch service object %s: %w", original.Name, err)
	}
	return nil
}

func (m *manager) limitNodes(nodes []*v1.Node, service *v1.Service) []*v1.Node {
	if m.config.maxBackendMembers == 0 || len(nodes) <= m.config.maxBackendMembers {
		return nodes
	}

	// TODO(pn): Should we do some special handling if service.Spec.ExternalTrafficPolicy equals to v1.ServiceExternalTrafficPolicyTypeLocal?
	// If ETP local is in use, it's more likely that LB won't work at all (all BEs can be down due to health check failing).
	m.eventRecorder.Eventf(service, v1.EventTypeWarning, nodeCountLimitReached,
		"Node count limit %d/%d reached, use node selector annotation to limit the number of nodes to ensure load balancer is working properly.",
		len(nodes), m.config.maxBackendMembers)

	// Use copy of `nodes` to avoid side effects
	nodesCopy := make([]*v1.Node, len(nodes))
	copy(nodesCopy, nodes)

	// Sort nodes so that our first N nodes will stay the same as long as possible and we don't cause e.g. some strange infinite reconcile loop.
	slices.SortStableFunc(nodesCopy, func(a, b *v1.Node) int {
		if a == nil || b == nil {
			return 0
		}
		if n := a.GetCreationTimestamp().Compare(b.GetCreationTimestamp().Time); n != 0 {
			return n
		}
		return strings.Compare(a.Name, b.Name)
	})

	n := make([]*v1.Node, m.config.maxBackendMembers)
	for i := range n {
		n[i] = nodesCopy[i]
	}
	m.log.Infof("Nodes filtered by the config, using %d nodes out of %d.", m.config.maxBackendMembers, len(nodes))
	return n
}

func NewLoadBalancerManager(
	svc Service,
	config *Config,
	coreV1Client corev1.CoreV1Interface,
	eventRecorder record.EventRecorder,
	log logger.Logger,
) cloudprovider.LoadBalancer {
	return &manager{svc: svc, coreV1Client: coreV1Client, eventRecorder: eventRecorder, config: config, log: log}
}
