package loadbalancer

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestLoadBalancer(t *testing.T) {
	t.Parallel()
	var res1 string
	var res2 string
	res1 = loadBalancerName("development", "application", "0a73c569-a2d1-481a-917f-b0fdafd78136")
	require.Equal(t, "development-application-0a73c569-bd5c1770", res1)

	res1 = loadBalancerName("development-very-long-namespace-name", "application-very-long-cool-app-name", "0a73c569-a2d1-481a-917f-b0fdafd78136")
	require.Equal(t, "development-very-long--application-very-long-c-0a73c569-3df913a8", res1)

	res1 = loadBalancerName("development-development-development-development-1", "application", "0a73c569-a2d1-481a-917f-b0fdafd78136")
	res2 = loadBalancerName("development-development-development-development-1", "application", "0a73c569-a2d1-481a-917f-b0fdafd78136")
	require.Equal(t, res1, res2)

	res1 = loadBalancerName("development-development-development-development-1", "application", "0a73c569-a2d1-481a-917f-b0fdafd78136")
	res2 = loadBalancerName("development-development-development-development-2", "application", "0a73c569-a2d1-481a-917f-b0fdafd78136")
	require.NotEqual(t, res1, res2)

	res1 = loadBalancerName("development", "application", "0a73c569-a2d1-481a-917f-?????????")
	res2 = loadBalancerName("development", "application", "0a73c569-a2d1-481a-917f-b0fdafd78136")
	require.NotEqual(t, res1, res2)

}
