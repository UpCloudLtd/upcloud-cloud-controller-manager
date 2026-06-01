package loadbalancer

import (
	"net/http"
	"testing"

	"github.com/UpCloudLtd/upcloud-go-api/v8/upcloud"
	"github.com/UpCloudLtd/upcloud-go-api/v8/upcloud/request"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func TestCreateLoadBalancerRequestsEqual(t *testing.T) {
	t.Parallel()

	id := uuid.New()

	t.Run("Equal", func(t *testing.T) {
		t.Parallel()
		r1 := createRequest(id)
		r2 := createRequest(id)
		require.NoError(t, createLoadBalancerRequestsEqual(&r1, &r2))
	})

	t.Run("Networks", func(t *testing.T) {
		t.Parallel()
		r1 := createRequest(id)
		r2 := createRequest(id)
		r2.Networks = networks(uuid.New())
		require.Equal(t, fieldValueNotEqualError("networks"), createLoadBalancerRequestsEqual(&r1, &r2))
	})

	t.Run("Frontends", func(t *testing.T) {
		t.Parallel()
		r1 := createRequest(id)
		r2 := createRequest(id)
		r2.Frontends[0].Rules[0].Actions[0].Type = "test"
		require.Equal(t, fieldValueNotEqualError("frontends"), createLoadBalancerRequestsEqual(&r1, &r2))
	})

	t.Run("Backends", func(t *testing.T) {
		t.Parallel()
		r1 := createRequest(id)
		r2 := createRequest(id)
		r2.Backends[0].Members[0].Name = "test-1"
		require.Equal(t, fieldValueNotEqualError("backends"), createLoadBalancerRequestsEqual(&r1, &r2))
	})
	t.Run("Name", func(t *testing.T) {
		t.Parallel()
		r1 := createRequest(id)
		r2 := createRequest(id)
		r2.Name = "test-1"
		require.Equal(t, fieldValueNotEqualError("name"), createLoadBalancerRequestsEqual(&r1, &r2))
	})
	t.Run("IPAddresses", func(t *testing.T) {
		t.Parallel()
		r1 := createRequest(id)
		r2 := createRequest(id)
		r2.IPAddresses[0].Address = "0.0.0.1"
		require.Equal(t, fieldValueNotEqualError("IP addresses"), createLoadBalancerRequestsEqual(&r1, &r2))
	})
}

func createRequest(id uuid.UUID) request.CreateLoadBalancerRequest {
	return request.CreateLoadBalancerRequest{
		Name:             "test-lb",
		Plan:             DefaultPlan,
		Zone:             "fi-hel2",
		NetworkUUID:      id.String(),
		ConfiguredStatus: upcloud.LoadBalancerConfiguredStatusStarted,
		MaintenanceDOW:   "sunday",
		MaintenanceTime:  "20:01:01Z",
		Networks:         networks(id),
		Frontends:        frontends(id),
		Backends:         backends(id),
		Resolvers:        resolvers(),
		Labels:           upcloudLabels(id),
		IPAddresses:      ipAddresses(),
	}
}

func networks(id uuid.UUID) []request.LoadBalancerNetwork {
	return []request.LoadBalancerNetwork{
		{
			Name:   networkNamePublic,
			Type:   upcloud.LoadBalancerNetworkTypePublic,
			Family: upcloud.LoadBalancerAddressFamilyIPv4,
		},
		{
			Name:   networkNamePrivate,
			Type:   upcloud.LoadBalancerNetworkTypePrivate,
			Family: upcloud.LoadBalancerAddressFamilyIPv4,
			UUID:   id.String(),
		},
	}
}

func frontends(id uuid.UUID) []request.LoadBalancerFrontend {
	return []request.LoadBalancerFrontend{
		{
			Name:           "fe-1",
			Mode:           upcloud.LoadBalancerModeHTTP,
			Port:           80,
			DefaultBackend: "be-1",
			Rules:          frontendRules(),
			TLSConfigs:     frontendTLS(id),
			Properties:     &upcloud.LoadBalancerFrontendProperties{},
			Networks:       frontendNetworks(),
		},
	}
}

func frontendNetworks() []upcloud.LoadBalancerFrontendNetwork {
	return []upcloud.LoadBalancerFrontendNetwork{
		{
			Name: networkNamePublic,
		},
	}
}

func frontendTLS(id uuid.UUID) []request.LoadBalancerFrontendTLSConfig {
	return []request.LoadBalancerFrontendTLSConfig{
		{
			Name:                  "bundle-1",
			CertificateBundleUUID: id.String(),
		},
	}
}

func frontendRules() []request.LoadBalancerFrontendRule {
	return []request.LoadBalancerFrontendRule{
		{
			Name:              "rule-1",
			Priority:          10,
			MatchingCondition: upcloud.LoadBalancerMatchingConditionAnd,
			Matchers: []upcloud.LoadBalancerMatcher{
				request.NewLoadBalancerPathMatcher(upcloud.LoadBalancerStringMatcherMethodExact, "/admin", nil),
			},
			Actions: []upcloud.LoadBalancerAction{
				request.NewLoadBalancerHTTPReturnAction(http.StatusForbidden, "", ""),
			},
		},
	}
}

func backends(id uuid.UUID) []request.LoadBalancerBackend {
	return []request.LoadBalancerBackend{
		{
			Name:       "be-1",
			Resolver:   "",
			Members:    backendMembers(),
			Properties: &upcloud.LoadBalancerBackendProperties{},
			TLSConfigs: backendTLS(id),
		},
	}
}

func backendMembers() []request.LoadBalancerBackendMember {
	return []request.LoadBalancerBackendMember{
		{
			Name:        "",
			Weight:      0,
			MaxSessions: 0,
			Enabled:     false,
			Type:        "",
			IP:          "",
			Port:        0,
		},
	}
}

func backendTLS(id uuid.UUID) []request.LoadBalancerBackendTLSConfig {
	return []request.LoadBalancerBackendTLSConfig{
		{
			Name:                  "bundle-2",
			CertificateBundleUUID: id.String(),
		},
	}
}

func resolvers() []request.LoadBalancerResolver {
	return []request.LoadBalancerResolver{
		{
			Name:         "dns-1",
			Nameservers:  []string{"10.0.0.1"},
			Retries:      10,
			Timeout:      10,
			TimeoutRetry: 10,
			CacheValid:   10,
			CacheInvalid: 10,
		},
	}
}

func upcloudLabels(id uuid.UUID) []upcloud.Label {
	return []upcloud.Label{
		{
			Key:   clusterIDLabel,
			Value: id.String(),
		},
	}
}

func ipAddresses() []request.LoadBalancerIPAddress {
	return []request.LoadBalancerIPAddress{
		{
			NetworkName: networkNamePublic,
			Address:     "0.0.0.0",
		},
	}
}
