package cloud_test

import (
	"testing"

	"github.com/UpCloudLtd/upcloud-cloud-controller-manager/internal/cloud"
	"github.com/UpCloudLtd/upcloud-cloud-controller-manager/internal/mock"
	"github.com/stretchr/testify/require"
	cloudprovider "k8s.io/cloud-provider"
)

func TestCloudProvider(t *testing.T) {
	t.Parallel()

	p, err := cloud.NewCloudProviderFromConfig(mock.ConfigReader())
	require.NoError(t, err)

	p.Initialize(mock.NewControllerClientBuilder(), nil)

	t.Run("ProviderName", func(t *testing.T) {
		t.Parallel()
		require.True(t, cloudprovider.IsCloudProvider(p.ProviderName()))
		require.Equal(t, cloud.ProviderName, p.ProviderName())
	})

	t.Run("HasClusterID", func(t *testing.T) {
		t.Parallel()
		require.True(t, p.HasClusterID())
	})

	t.Run("LoadBalancer", func(t *testing.T) {
		t.Parallel()
		_, implemented := p.LoadBalancer()
		require.True(t, implemented)
	})

	t.Run("InstancesV2", func(t *testing.T) {
		t.Parallel()
		_, implemented := p.InstancesV2()
		require.True(t, implemented)
	})

	t.Run("Instances", func(t *testing.T) {
		t.Parallel()
		_, implemented := p.Instances()
		require.False(t, implemented)
	})

	t.Run("Zones", func(t *testing.T) {
		t.Parallel()
		_, implemented := p.Zones()
		require.False(t, implemented)
	})

	t.Run("Clusters", func(t *testing.T) {
		t.Parallel()
		_, implemented := p.Clusters()
		require.False(t, implemented)
	})

	t.Run("Routes", func(t *testing.T) {
		t.Parallel()
		_, implemented := p.Routes()
		require.False(t, implemented)
	})
}
