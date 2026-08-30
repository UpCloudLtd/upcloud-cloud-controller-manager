package loadbalancer

import (
	"strings"
	"testing"

	"github.com/UpCloudLtd/upcloud-go-api/v8/upcloud"
	"github.com/stretchr/testify/require"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func testNode(name, internalIP string) *v1.Node {
	return &v1.Node{
		ObjectMeta: metav1.ObjectMeta{Name: name},
		Status: v1.NodeStatus{
			Addresses: []v1.NodeAddress{
				{Type: v1.NodeHostName, Address: name},
				{Type: v1.NodeInternalIP, Address: internalIP},
			},
		},
	}
}

// TestLoadBalancerBackendFromServicePortUsesInternalIPByDefault pins that a
// nil address map leaves the long-standing behaviour untouched.
func TestLoadBalancerBackendFromServicePortUsesInternalIPByDefault(t *testing.T) {
	t.Parallel()

	nodes := []*v1.Node{testNode("node-1", "10.0.0.1"), testNode("node-2", "10.0.0.2")}
	port := &v1.ServicePort{Name: "http", Port: 80, NodePort: 32000}

	backend := loadBalancerBackendFromServicePort(port, nodes, upcloud.LoadBalancerPlan{PerServerMaxSessions: 1000}, nil)

	require.Len(t, backend.Members, 2)
	require.Equal(t, "10.0.0.1", backend.Members[0].IP)
	require.Equal(t, "10.0.0.2", backend.Members[1].IP)
	require.Equal(t, 32000, backend.Members[0].Port)
}

// TestLoadBalancerBackendFromServicePortPrefersResolvedAddresses covers the
// meshed-cluster case: the node's InternalIP is an overlay address the load
// balancer cannot reach, so the member must carry the resolved private
// address instead.
func TestLoadBalancerBackendFromServicePortPrefersResolvedAddresses(t *testing.T) {
	t.Parallel()

	nodes := []*v1.Node{testNode("node-1", "172.30.2.2"), testNode("node-2", "172.30.2.3")}
	port := &v1.ServicePort{Name: "http", Port: 80, NodePort: 32000}
	addresses := map[string]string{"node-1": "10.173.209.3", "node-2": "10.173.209.4"}

	backend := loadBalancerBackendFromServicePort(port, nodes, upcloud.LoadBalancerPlan{PerServerMaxSessions: 1000}, addresses)

	require.Len(t, backend.Members, 2)
	require.Equal(t, "10.173.209.3", backend.Members[0].IP)
	require.Equal(t, "10.173.209.4", backend.Members[1].IP)
	for _, m := range backend.Members {
		require.NotContains(t, m.IP, "172.30.", "overlay address must never reach the load balancer")
	}
}

// TestLoadBalancerBackendFromServicePortSkipsUnresolvedNodes keeps a node
// whose address could not be resolved out of the backend rather than
// enrolling it with an empty IP.
func TestLoadBalancerBackendFromServicePortSkipsUnresolvedNodes(t *testing.T) {
	t.Parallel()

	nodes := []*v1.Node{testNode("node-1", "172.30.2.2"), testNode("node-2", "172.30.2.3")}
	port := &v1.ServicePort{Name: "http", Port: 80, NodePort: 32000}

	backend := loadBalancerBackendFromServicePort(port, nodes, upcloud.LoadBalancerPlan{}, map[string]string{"node-1": "10.173.209.3"})

	require.Len(t, backend.Members, 1)
	require.Equal(t, "10.173.209.3", backend.Members[0].IP)
}

func TestServerAddressOnNetwork(t *testing.T) {
	t.Parallel()

	server := &upcloud.ServerDetails{
		Server: upcloud.Server{UUID: "00674f78-8d6c-4781-9233-f58b86a202fe"},
		Networking: upcloud.ServerNetworking{
			Interfaces: upcloud.ServerInterfaceSlice{
				{
					Type:        upcloud.NetworkTypeUtility,
					Network:     "utility-net",
					IPAddresses: upcloud.IPAddressSlice{{Family: upcloud.IPAddressFamilyIPv4, Address: "10.1.24.100"}},
				},
				{
					Type:        upcloud.NetworkTypePrivate,
					Network:     "sdn-net",
					IPAddresses: upcloud.IPAddressSlice{{Family: upcloud.IPAddressFamilyIPv4, Address: "10.173.209.4"}},
				},
			},
		},
	}

	got, err := serverAddressOnNetwork(server, "sdn-net")
	require.NoError(t, err)
	require.Equal(t, "10.173.209.4", got, "must pick the address on the load balancer's own network, not the utility one")

	_, err = serverAddressOnNetwork(server, "other-net")
	require.Error(t, err)
	require.True(t, strings.Contains(err.Error(), "other-net"))

	_, err = serverAddressOnNetwork(nil, "sdn-net")
	require.Error(t, err)
}

// TestNewConfigDefaultsBackendAddressSource keeps existing deployments, whose
// config predates the key, on the InternalIP behaviour.
func TestNewConfigDefaultsBackendAddressSource(t *testing.T) {
	t.Parallel()

	require.Equal(t, BackendAddressSourceNodeInternalIP,
		NewConfig("id", "name", DefaultPlan, 10, "").backendAddressSource)
	require.Equal(t, BackendAddressSourcePrivateNetwork,
		NewConfig("id", "name", DefaultPlan, 10, BackendAddressSourcePrivateNetwork).backendAddressSource)
}
