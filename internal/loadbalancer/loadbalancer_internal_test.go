package loadbalancer

import (
	"fmt"
	"testing"

	"github.com/UpCloudLtd/upcloud-cloud-controller-manager/internal/logger"
	"github.com/UpCloudLtd/upcloud-cloud-controller-manager/internal/mock"
	"github.com/UpCloudLtd/upcloud-cloud-controller-manager/internal/utils"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/tools/record"
)

func TestLoadBalancerInternal(t *testing.T) {
	t.Parallel()

	cli := mock.NewControllerClientBuilder().ClientOrDie("test-client")
	cfg := NewConfig(uuid.NewString(), "test-cluster", "essentials", 10)
	broadcaster := record.NewBroadcaster()
	m := &manager{
		svc:           nil,
		coreV1Client:  cli.CoreV1(),
		eventRecorder: broadcaster.NewRecorder(scheme.Scheme, v1.EventSource{Component: "service-controller"}),
		config:        cfg,
		log:           logger.NewKlog(),
	}

	t.Run("configuredClusterName", func(t *testing.T) {
		t.Parallel()

		want := "test-name"
		got := m.configuredClusterName(want)
		require.Equal(t, cfg.clusterName, got)

		cfg.clusterName = ""
		got = m.configuredClusterName(want)
		require.Equal(t, want, got)
	})

	t.Run("limitNodes", func(t *testing.T) {
		t.Parallel()

		lbsvc := &v1.Service{
			ObjectMeta: metav1.ObjectMeta{Name: "test-service"},
			Spec: v1.ServiceSpec{
				Type: v1.ServiceTypeLoadBalancer,
			},
		}
		_, err := cli.CoreV1().Services("default").Create(t.Context(), lbsvc, metav1.CreateOptions{})
		require.NoError(t, err)
		nodes := m.limitNodes(newNodes(cfg.maxBackendMembers+2), lbsvc)
		require.Len(t, nodes, cfg.maxBackendMembers)

		wantNodeCount := 10
		nodes = m.limitNodes(newNodes(wantNodeCount), lbsvc)
		require.Len(t, nodes, wantNodeCount)
	})
}

func newNodes(n int) []*v1.Node {
	nodes := make([]*v1.Node, n)

	netUUID := uuid.NewString()
	for i := range n {
		nodes[i] = &v1.Node{
			ObjectMeta: metav1.ObjectMeta{
				Name: fmt.Sprintf("node-%d", i),
				Annotations: map[string]string{
					utils.PrivateNetworkUUIDAnnotation: netUUID,
				},
			},
			Spec: v1.NodeSpec{ProviderID: uuid.NewString()},
		}
	}
	return nodes
}
