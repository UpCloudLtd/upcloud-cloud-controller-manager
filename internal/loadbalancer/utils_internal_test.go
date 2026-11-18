package loadbalancer

import (
	"testing"

	"github.com/stretchr/testify/require"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
)

func TestNodeSelectorFromService(t *testing.T) {
	t.Parallel()

	testLabelSelectorString(t, "matchlabels:\n  role: system", labels.Set{"role": "system"})
	testLabelSelectorString(t, `{"matchlabels":{"role":"db"}}`, labels.Set{"role": "db"})
}

func testLabelSelectorString(t *testing.T, selector string, expectMatch labels.Set) {
	t.Helper()

	svc := v1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Annotations: map[string]string{
				loadBalancerNodeSelectorAnnotation: selector,
			},
		},
	}
	got, err := nodeSelectorFromService(&svc)
	require.NoError(t, err)
	require.False(t, got.Empty())
	require.True(t, got.Matches(expectMatch), "selector '%s' doesn't match labels '%s'", selector, expectMatch)
}
