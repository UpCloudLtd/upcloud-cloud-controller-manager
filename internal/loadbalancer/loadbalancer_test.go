package loadbalancer_test

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/UpCloudLtd/upcloud-cloud-controller-manager/internal/loadbalancer"
	"github.com/UpCloudLtd/upcloud-cloud-controller-manager/internal/logger"
	"github.com/UpCloudLtd/upcloud-cloud-controller-manager/internal/mock"
	"github.com/UpCloudLtd/upcloud-cloud-controller-manager/internal/utils"
	"github.com/UpCloudLtd/upcloud-go-api/v8/upcloud"
	"github.com/UpCloudLtd/upcloud-go-api/v8/upcloud/request"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/scheme"
	v1types "k8s.io/client-go/kubernetes/typed/core/v1"
	"k8s.io/client-go/tools/record"
	cloudprovider "k8s.io/cloud-provider"
)

const serverUUID string = "006ec10e-bbc8-452f-8987-d80f77595484"

func TestLoadBalancer(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()

	m, err := newManager(ctx)
	require.NoError(t, err)

	service := v1.Service{
		ObjectMeta: metav1.ObjectMeta{Name: "test-service", Namespace: "default"},
	}
	nodes := newNodes()

	t.Run("EnsureLoadBalancer", func(t *testing.T) {
		status, err := m.EnsureLoadBalancer(ctx, "", &service, nodes)
		require.NoError(t, err)
		require.Len(t, status.Ingress, 1)
		require.True(t, strings.HasSuffix(status.Ingress[0].Hostname, "-default-test-service.example.com"))
	})

	t.Run("GetLoadBalancer", func(t *testing.T) {
		t.Parallel()
		status, exists, err := m.GetLoadBalancer(ctx, "", &service)
		require.NoError(t, err)
		require.True(t, exists)
		require.Len(t, status.Ingress, 1)
		require.True(t, strings.HasSuffix(status.Ingress[0].Hostname, "-default-test-service.example.com"))
	})

	t.Run("GetLoadBalancerName", func(t *testing.T) {
		t.Parallel()
		name := m.GetLoadBalancerName(ctx, "", &service)
		require.Equal(t, "test-service", name)
	})

	t.Run("UpdateLoadBalancer", func(t *testing.T) {
		t.Parallel()
		nodes = append(nodes, &v1.Node{
			ObjectMeta: metav1.ObjectMeta{
				Name: "node-2",
				Annotations: map[string]string{
					utils.PrivateNetworkUUIDAnnotation: uuid.NewString(),
				},
			},
			Spec: v1.NodeSpec{ProviderID: serverUUID},
		})
		require.NoError(t, m.UpdateLoadBalancer(ctx, "", &service, nodes))
	})

	t.Run("EnsureLoadBalancerDeleted", func(t *testing.T) {
		// NOTE: This should be safe to run in parallel because API mock doesn't actually remove LB.
		t.Parallel()
		require.NoError(t, m.EnsureLoadBalancerDeleted(ctx, "", &service))
	})
}

func TestLoadBalancerCustomConfig(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()

	upcs := newUpCloudMockService()

	m, err := newManagerWithService(ctx, upcs)
	require.NoError(t, err)

	config := &request.CreateLoadBalancerRequest{
		IPAddresses: []request.LoadBalancerIPAddress{
			{
				NetworkName: "public-IPv4",
				Address:     "0.0.0.0",
			},
		},
	}
	service := v1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-service",
			Namespace: "default",
			Annotations: map[string]string{
				"service.beta.kubernetes.io/upcloud-load-balancer-config": loadBalancerRequestToJSON(t, config),
			},
		},
	}
	nodes := newNodes()

	t.Run("IPAddresses", func(t *testing.T) {
		_, err := m.EnsureLoadBalancer(ctx, "", &service, nodes)
		require.NoError(t, err)
		lbs, err := upcs.GetLoadBalancers(ctx, &request.GetLoadBalancersRequest{})
		require.NoError(t, err)
		require.Len(t, lbs, 1)
		lb := lbs[0]
		require.Len(t, lb.IPAddresses, 1)
		require.Equal(t, config.IPAddresses[0].NetworkName, lb.IPAddresses[0].NetworkName)
		require.Equal(t, config.IPAddresses[0].Address, lb.IPAddresses[0].Address)
	})
}

func newManager(ctx context.Context) (cloudprovider.LoadBalancer, error) {
	upcs := newUpCloudMockService()
	return newManagerWithService(ctx, upcs)
}

func newManagerWithService(ctx context.Context, upcs loadbalancer.UpCloudService) (cloudprovider.LoadBalancer, error) {
	client := mock.NewControllerClientBuilder().ClientOrDie("test-client")
	if _, err := client.CoreV1().Services("default").Create(ctx, &v1.Service{
		ObjectMeta: metav1.ObjectMeta{Name: "test-service"},
		Spec: v1.ServiceSpec{
			Type: v1.ServiceTypeLoadBalancer,
		},
	}, metav1.CreateOptions{}); err != nil {
		return nil, err
	}

	config := loadbalancer.NewConfig(uuid.NewString(), "development", 10)
	loadBalancerService := loadbalancer.NewLoadBalancerService(upcs, mock.NewUpCloudClient(), config)
	broadcaster := record.NewBroadcaster()
	broadcaster.StartStructuredLogging(0)
	broadcaster.StartRecordingToSink(&v1types.EventSinkImpl{Interface: client.CoreV1().Events("")})
	eventRecorder := broadcaster.NewRecorder(scheme.Scheme, v1.EventSource{Component: "service-controller"})
	log := logger.NewKlog()
	return loadbalancer.NewLoadBalancerManager(loadBalancerService, config, client.CoreV1(), eventRecorder, log), nil
}

func newUpCloudMockService() *mock.UpCloudService {
	return mock.NewUpCloudService(
		upcloud.ServerDetails{Server: upcloud.Server{UUID: uuid.NewString()}},
		upcloud.ServerDetails{Server: upcloud.Server{UUID: uuid.NewString()}},
		upcloud.ServerDetails{
			Server: upcloud.Server{UUID: serverUUID, State: upcloud.ServerStateStopped},
			Networking: upcloud.ServerNetworking{
				Interfaces: upcloud.ServerInterfaceSlice{
					{
						Type:        upcloud.NetworkTypePrivate,
						IPAddresses: upcloud.IPAddressSlice{{Address: "10.0.0.10"}},
					},
				},
			},
		},
	)
}

func newNodes() []*v1.Node {
	return []*v1.Node{
		{
			ObjectMeta: metav1.ObjectMeta{
				Name: "node-1",
				Annotations: map[string]string{
					utils.PrivateNetworkUUIDAnnotation: uuid.NewString(),
				},
			},
			Spec: v1.NodeSpec{ProviderID: serverUUID},
		},
	}
}

func loadBalancerRequestToJSON(t *testing.T, r *request.CreateLoadBalancerRequest) string {
	t.Helper()
	b, err := json.MarshalIndent(r, "", "\t")
	require.NoError(t, err)
	return string(b)
}
