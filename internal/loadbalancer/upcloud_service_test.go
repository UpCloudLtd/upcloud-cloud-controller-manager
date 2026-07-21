package loadbalancer_test

import (
	"testing"

	"github.com/UpCloudLtd/upcloud-cloud-controller-manager/internal/loadbalancer"
	"github.com/UpCloudLtd/upcloud-cloud-controller-manager/internal/mock"
	"github.com/UpCloudLtd/upcloud-go-api/v8/upcloud"
	"github.com/UpCloudLtd/upcloud-go-api/v8/upcloud/request"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestService(t *testing.T) {
	t.Parallel()

	upcs := newUpCloudMockService()

	clusterID := uuid.NewString()
	lbPlan := "essentials"

	lbs := loadbalancer.NewLoadBalancerService(upcs, mock.NewUpCloudClient(),
		loadbalancer.NewConfig(clusterID, "test-cluster", lbPlan, 100))

	nodes := newNodes()

	t.Run("CertificateAutoProvisioning", func(t *testing.T) {
		s := newService("myspace-1", "mylb", 443)
		lb, err := lbs.CreateLoadBalancer(t.Context(), &s, nodes, "test-cluster-certs")
		require.NoError(t, err)
		wantBundleName := clusterID + "-myspace-1-mylb-tls"
		requireBundleExists(t, lb, wantBundleName)

		require.NoError(t, lbs.UpdateLoadBalancer(t.Context(), lb, &s, nodes, "test-cluster-certs"))
	})

	t.Run("CertificateAutoProvisioning using bundle name", func(t *testing.T) {
		s := newService("myspace", "mylb", 8443)
		config := &request.CreateLoadBalancerRequest{
			Frontends: []request.LoadBalancerFrontend{
				{
					TLSConfigs: []request.LoadBalancerFrontendTLSConfig{
						{Name: "needs-certificate"},
					},
				},
			},
		}
		s.Annotations = map[string]string{
			"service.beta.kubernetes.io/upcloud-load-balancer-config": loadBalancerRequestToJSON(t, config),
		}
		lb, err := lbs.CreateLoadBalancer(t.Context(), &s, nodes, "test-cluster-certs")
		require.NoError(t, err)
		wantBundleName := clusterID + "-myspace-mylb-tls"
		requireBundleExists(t, lb, wantBundleName)
	})
}

func newService(namespace, name string, port int32) v1.Service {
	return v1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: namespace,
			Name:      name,
		},
		Spec: v1.ServiceSpec{
			Ports: []v1.ServicePort{
				{Port: port},
			},
		},
	}
}

func requireBundleExists(t *testing.T, lb *upcloud.LoadBalancer, bundleName string) {
	t.Helper()

	require.Len(t, lb.Frontends, 1)
	require.NotEmpty(t, lb.Frontends[0].TLSConfigs)
	require.NotEmpty(t, lb.Frontends[0].TLSConfigs[0].CertificateBundleUUID)
	require.Equal(t, bundleName, lb.Frontends[0].TLSConfigs[0].Name)
}
