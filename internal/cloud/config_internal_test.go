package cloud

import (
	"testing"

	"github.com/UpCloudLtd/upcloud-cloud-controller-manager/internal/mock"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/labels"
)

func TestNewConfig(t *testing.T) {
	t.Parallel()

	got, err := newConfig(mock.ConfigReader())
	require.NoError(t, err)

	require.Equal(t, "CCMConfig", got.Kind)
	require.Equal(t, "ccm-config", got.Name)
	require.Equal(t, "test-cluster", got.Data.ClusterName)
	require.Equal(t, "0de1e792-2f7c-40df-b1d4-38c0a5a5400b", got.Data.ClusterID)
	require.Equal(t, "development", got.Data.LoadBalancerPlan)
	require.Equal(t, 3, got.Data.LoadBalancerMaxBackendMembers)
	require.Equal(t, "testuser", got.Data.APICredentials.User)
	require.Equal(t, "testpass", got.Data.APICredentials.Password)

	require.True(t, got.Data.nodeScopeLabelSelector.Matches(labels.Set{
		"node-role.kubernetes.io/data-plane": "",
	}))
	require.False(t, got.Data.nodeScopeLabelSelector.Matches(labels.Set{
		"node-role.kubernetes.io/control-plane": "",
	}))
	require.False(t, got.Data.nodeScopeLabelSelector.Matches(labels.Set{
		"node-role.kubernetes.io/master": "",
	}))
	require.False(t, got.Data.nodeScopeLabelSelector.Matches(labels.Set{
		"node.kubernetes.io/exclude-from-external-load-balancers": "",
	}))
}
