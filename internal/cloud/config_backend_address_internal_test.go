package cloud

import (
	"strings"
	"testing"

	"github.com/UpCloudLtd/upcloud-cloud-controller-manager/internal/loadbalancer"
	"github.com/UpCloudLtd/upcloud-cloud-controller-manager/internal/mock"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/labels"
)

func configWith(source string) string {
	return `
apiVersion: infrastructure.cluster.x-k8s.io/v1alpha1
kind: CCMConfig
metadata:
  name: ccm-config
data:
  clusterName: test-cluster
  loadBalancerBackendAddressSource: "` + source + `"
  apiCredentials:
    user: "testuser"
    password: "testpass"
`
}

// TestNewConfigBackendAddressSourceDefaults keeps a config that predates the
// key - every existing deployment - on the InternalIP behaviour.
func TestNewConfigBackendAddressSourceDefaults(t *testing.T) {
	t.Parallel()

	got, err := newConfig(mock.ConfigReader())
	require.NoError(t, err)
	require.Equal(t, loadbalancer.BackendAddressSourceNodeInternalIP, got.Data.LoadBalancerBackendAddressSource)
}

func TestNewConfigBackendAddressSourceAccepted(t *testing.T) {
	t.Parallel()

	for _, source := range []string{
		loadbalancer.BackendAddressSourceNodeInternalIP,
		loadbalancer.BackendAddressSourcePrivateNetwork,
	} {
		got, err := newConfig(strings.NewReader(configWith(source)))
		require.NoError(t, err)
		require.Equal(t, source, got.Data.LoadBalancerBackendAddressSource)
	}
}

// TestNewConfigBackendAddressSourceRejectsUnknown fails fast at startup
// rather than silently falling back and leaving unreachable members.
func TestNewConfigBackendAddressSourceRejectsUnknown(t *testing.T) {
	t.Parallel()

	_, err := newConfig(strings.NewReader(configWith("private-ip")))
	require.Error(t, err)
	require.Contains(t, err.Error(), "loadBalancerBackendAddressSource")
}

// TestNewConfigParsesTheChartRenderedConfig feeds the exact ConfigMap body the
// ankra-charts upcloud-ccm chart renders for a meshed cluster, so the two
// halves of the fix - the chart writing the keys and the controller reading
// them - cannot drift apart silently. Regenerate with:
//
//	helm template t ./upcloud-ccm -f values.yaml -s templates/configmap.yaml
func TestNewConfigParsesTheChartRenderedConfig(t *testing.T) {
	// No t.Parallel: the credentials come from the environment, as they do
	// in the chart (extraEnv wires UPCLOUD_API_TOKEN from a Secret).
	const rendered = `apiVersion: infrastructure.cluster.x-k8s.io/v1alpha1
kind: CCMConfig
metadata:
  name: ccm-config
data:
  clusterName: "upcloud-3edd8155"
  clusterID: "3edd8155-8f44-4f32-a51e-4026d63b0e44"
  loadBalancerPlan: "development"
  loadBalancerMaxBackendMembers: 100
  loadBalancerBackendAddressSource: "private-network"
  nodeScopeSelector:
    matchLabels:
      topology.kubernetes.io/zone: fi-hel1
`
	t.Setenv("UPCLOUD_API_TOKEN", "token")

	got, err := newConfig(strings.NewReader(rendered))
	require.NoError(t, err)
	require.Equal(t, "upcloud-3edd8155", got.Data.ClusterName)
	require.Equal(t, loadbalancer.BackendAddressSourcePrivateNetwork, got.Data.LoadBalancerBackendAddressSource)
	require.Equal(t, 100, got.Data.LoadBalancerMaxBackendMembers)

	// The selector the chart used to drop must now actually scope nodes.
	require.True(t, got.Data.nodeScopeLabelSelector.Matches(labels.Set{
		"topology.kubernetes.io/zone": "fi-hel1",
	}))
	require.False(t, got.Data.nodeScopeLabelSelector.Matches(labels.Set{
		"topology.kubernetes.io/zone": "se-sto1",
	}))
}
