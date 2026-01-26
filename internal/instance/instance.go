package instance

import (
	"context"
	"fmt"
	"strings"

	"github.com/UpCloudLtd/upcloud-cloud-controller-manager/internal/logger"
	"github.com/UpCloudLtd/upcloud-cloud-controller-manager/internal/utils"
	"github.com/UpCloudLtd/upcloud-go-api/v8/upcloud"
	"github.com/UpCloudLtd/upcloud-go-api/v8/upcloud/request"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/types"
	corev1 "k8s.io/client-go/kubernetes/typed/core/v1"
	cloudprovider "k8s.io/cloud-provider"
)

type UpCloudService interface {
	GetServers(ctx context.Context) (*upcloud.Servers, error)
	GetServerDetails(ctx context.Context, r *request.GetServerDetailsRequest) (*upcloud.ServerDetails, error)
}

// manager is instance manager that implements cloudprovider.InstancesV2 interface
type manager struct {
	upcs         UpCloudService
	nodeSelector labels.Selector
	nodeClient   corev1.NodeInterface
	log          logger.Logger
}

// InstanceExists returns true if the instance for the given node exists according to the cloud provider.
// Use the node.name or node.spec.providerID field to find the node in the cloud provider.
func (m *manager) InstanceExists(ctx context.Context, node *v1.Node) (bool, error) {
	m.log.Infof("checking if instance %s exists", node.Name)
	if !m.nodeSelectorMatches(node) {
		m.log.Infof("instance %s doesn't match node selector, ignoring", node.Name)
		return false, nil
	}

	serverUUID, err := utils.ServerUUIDFromNode(node)
	if err != nil {
		return false, err
	}

	_, err = m.upcs.GetServerDetails(ctx, &request.GetServerDetailsRequest{UUID: serverUUID.String()})

	if utils.ErrIsHTTPStatusNotFound(err) {
		// explicitly return nil error if server UUID is provided but server is not found
		return false, nil
	}
	return err == nil, err
}

// InstanceShutdown returns true if the instance is shutdown according to the cloud provider.
// Use the node.name or node.spec.providerID field to find the node in the cloud provider.
func (m *manager) InstanceShutdown(ctx context.Context, node *v1.Node) (bool, error) {
	m.log.Infof("checking if instance %s is shutdown", node.Name)
	if !m.nodeSelectorMatches(node) {
		m.log.Infof("instance %s doesn't match node selector, ignoring", node.Name)
		return false, errOutOfScope
	}

	serverUUID, err := utils.ServerUUIDFromNode(node)
	if err != nil {
		return false, err
	}

	d, err := m.upcs.GetServerDetails(ctx, &request.GetServerDetailsRequest{UUID: serverUUID.String()})
	if err != nil {
		return false, err
	}
	return d.State == upcloud.ServerStateStopped, err
}

// InstanceMetadata returns the instance's metadata. The values returned in InstanceMetadata are
// translated into specific fields and labels in the Node object on registration.
// Implementations should always check node.spec.providerID first when trying to discover the instance
// for a given node. In cases where node.spec.providerID is empty, implementations can use other
// properties of the node like its name, labels and annotations.
func (m *manager) InstanceMetadata(ctx context.Context, node *v1.Node) (*cloudprovider.InstanceMetadata, error) {
	m.log.Infof("checking instance %s metadata", node.Name)
	if !m.nodeSelectorMatches(node) {
		m.log.Infof("instance %s doesn't match node selector, ignoring", node.Name)
		return &cloudprovider.InstanceMetadata{}, nil
	}
	server, err := m.nodeServerDetails(ctx, node)
	if err != nil {
		return nil, err
	}
	meta, err := instanceMetaFromServerDetails(server)
	if err != nil {
		return meta, err
	}

	return meta, m.syncNodeAnnotations(ctx, node, server)
}

func (m *manager) nodeServerDetails(ctx context.Context, node *v1.Node) (*upcloud.ServerDetails, error) {
	if serverUUID, err := utils.ServerUUIDFromNode(node); err == nil {
		s, err := m.upcs.GetServerDetails(ctx, &request.GetServerDetailsRequest{UUID: serverUUID.String()})
		if utils.ErrIsHTTPStatusNotFound(err) {
			return nil, cloudprovider.InstanceNotFound
		}
		return s, err
	}
	m.log.Infof("couldn't find server UUID from node %s object, trying to search servers having hostname=%s or title=%s", node.Name, node.Name, node.Name)
	servers, err := m.upcs.GetServers(ctx)
	if err != nil {
		return nil, err
	}
	for _, s := range servers.Servers {
		// TODO: This is too ambiguous e.g. zone can be anything, so we should at least do something like:
		// if m.zone == s.Zone && (s.Hostname == node.Name || strings.Contains(s.Title, node.Name))
		// .. but do we even need to support case where provider ID (or annotation) is not set?
		if s.Hostname == node.Name || strings.Contains(s.Title, node.Name) {
			return m.upcs.GetServerDetails(ctx, &request.GetServerDetailsRequest{UUID: s.UUID})
		}
	}
	return nil, fmt.Errorf("%w: server with name %s", cloudprovider.InstanceNotFound, node.Name)
}

func (m *manager) syncNodeAnnotations(ctx context.Context, node *v1.Node, server *upcloud.ServerDetails) error {
	privateInterfaces := utils.ServerInterfacesByType(server, upcloud.NetworkTypePrivate)
	if len(privateInterfaces) < 1 {
		return fmt.Errorf("unable to sync node %s annotations, private network interface not found", node.Name)
	}

	annotations := node.GetAnnotations()
	if annotations == nil {
		annotations = make(map[string]string)
	}
	if v, ok := annotations[utils.PrivateNetworkUUIDAnnotation]; ok && v == privateInterfaces[0].Network {
		// node already has correct annotation
		return nil
	}
	cpNode := node.DeepCopy()
	if cpNode.Annotations == nil {
		cpNode.Annotations = make(map[string]string)
	}
	cpNode.Annotations[utils.PrivateNetworkUUIDAnnotation] = privateInterfaces[0].Network
	return m.patchNode(ctx, node, cpNode)
}

func (m *manager) patchNode(ctx context.Context, original, modified *v1.Node) error {
	patch, err := utils.CreateTwoWayMergePatch(original, modified, v1.Node{})
	if err != nil {
		return fmt.Errorf("failed to create 2-way merge patch: %w", err)
	}
	if len(patch) == 0 || string(patch) == "{}" {
		return nil
	}
	m.log.Infof("patching node %s", original.Name)
	_, err = m.nodeClient.Patch(ctx, original.Name, types.StrategicMergePatchType, patch, metav1.PatchOptions{})
	if err != nil {
		return fmt.Errorf("failed to patch node object %s: %w", original.Name, err)
	}
	return nil
}

func (m *manager) nodeSelectorMatches(node *v1.Node) bool {
	if m.nodeSelector == nil {
		return true
	}
	return m.nodeSelector.Matches(labels.Set(node.Labels))
}

func NewInstancesManager(
	upCloudService UpCloudService,
	nodeSelector labels.Selector,
	nodeClient corev1.NodeInterface,
	log logger.Logger,
) cloudprovider.InstancesV2 {
	return &manager{upcs: upCloudService, nodeSelector: nodeSelector, nodeClient: nodeClient, log: log}
}
