package loadbalancer_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/UpCloudLtd/upcloud-cloud-controller-manager/internal/loadbalancer"
	"github.com/UpCloudLtd/upcloud-cloud-controller-manager/internal/logger"
	"github.com/UpCloudLtd/upcloud-cloud-controller-manager/internal/mock"
	"github.com/UpCloudLtd/upcloud-cloud-controller-manager/internal/utils"
	"github.com/UpCloudLtd/upcloud-go-api/v8/upcloud"
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

	m, err := initManager(ctx)
	require.NoError(t, err)

	service := v1.Service{
		ObjectMeta: metav1.ObjectMeta{Name: "test-service", Namespace: "default"},
	}
	nodes := []*v1.Node{
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

func initManager(ctx context.Context) (cloudprovider.LoadBalancer, error) {
	client := mock.NewControllerClientBuilder().ClientOrDie("test-client")
	upcs := mock.NewUpCloudService(
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
