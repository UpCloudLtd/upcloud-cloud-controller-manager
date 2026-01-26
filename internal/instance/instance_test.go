package instance_test

import (
	"context"
	"testing"
	"time"

	"github.com/UpCloudLtd/upcloud-cloud-controller-manager/internal/instance"
	"github.com/UpCloudLtd/upcloud-cloud-controller-manager/internal/logger"
	"github.com/UpCloudLtd/upcloud-cloud-controller-manager/internal/mock"
	"github.com/UpCloudLtd/upcloud-cloud-controller-manager/internal/utils"
	"github.com/UpCloudLtd/upcloud-go-api/v8/upcloud"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	cloudprovider "k8s.io/cloud-provider"
)

const serverUUID string = "006ec10e-bbc8-452f-8987-d80f77595474"

func TestInstancesV2(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()

	m, err := initManager(ctx)
	require.NoError(t, err)

	t.Run("InstanceExists", func(t *testing.T) {
		t.Parallel()

		exists, err := m.InstanceExists(ctx, &v1.Node{
			Spec: v1.NodeSpec{ProviderID: utils.ProviderIDPrefix + serverUUID},
		})
		require.NoError(t, err)
		require.True(t, exists)

		// This should match node selector
		exists, err = m.InstanceExists(ctx, &v1.Node{
			ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"node-role.kubernetes.io/control-plane": ""}},
			Spec:       v1.NodeSpec{ProviderID: utils.ProviderIDPrefix + serverUUID},
		})
		require.NoError(t, err)
		require.False(t, exists)
	})

	t.Run("InstanceShutdown", func(t *testing.T) {
		shutdown, err := m.InstanceShutdown(ctx, &v1.Node{
			Spec: v1.NodeSpec{ProviderID: utils.ProviderIDPrefix + serverUUID},
		})
		require.NoError(t, err)
		require.True(t, shutdown)
	})

	t.Run("InstanceMetadata", func(t *testing.T) {
		meta, err := m.InstanceMetadata(ctx, &v1.Node{
			ObjectMeta: metav1.ObjectMeta{Name: "test-node"},
			Spec:       v1.NodeSpec{ProviderID: utils.ProviderIDPrefix + serverUUID},
		})
		require.NoError(t, err)
		require.Equal(t, utils.ProviderIDPrefix+serverUUID, meta.ProviderID)
	})
}

func initManager(ctx context.Context) (cloudprovider.InstancesV2, error) {
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
	if _, err := client.CoreV1().Nodes().Create(ctx, &v1.Node{
		ObjectMeta: metav1.ObjectMeta{Name: "test-node"},
		Spec:       v1.NodeSpec{ProviderID: utils.ProviderIDPrefix + serverUUID},
	}, metav1.CreateOptions{}); err != nil {
		return nil, err
	}
	nodeSelector, _ := metav1.LabelSelectorAsSelector(&metav1.LabelSelector{
		MatchExpressions: []metav1.LabelSelectorRequirement{
			{
				Key:      "node-role.kubernetes.io/control-plane",
				Operator: metav1.LabelSelectorOpDoesNotExist,
			},
		},
	})
	log := logger.NewKlog()
	return instance.NewInstancesManager(upcs, nodeSelector, client.CoreV1().Nodes(), log), nil
}
