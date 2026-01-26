package utils_test

import (
	"errors"
	"net/http"
	"testing"

	"github.com/UpCloudLtd/upcloud-cloud-controller-manager/internal/utils"
	"github.com/UpCloudLtd/upcloud-go-api/v8/upcloud"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestCreateTwoWayMergePatch(t *testing.T) {
	t.Parallel()

	original := v1.Service{Spec: v1.ServiceSpec{ExternalName: "original"}}
	modified := v1.Service{Spec: v1.ServiceSpec{ExternalName: "modified"}}
	got, err := utils.CreateTwoWayMergePatch(&original, &modified, v1.Service{})
	require.NoError(t, err)
	require.Equal(t, `{"spec":{"externalName":"modified"}}`, string(got))
}

func TestServerUUIDFromNode(t *testing.T) {
	t.Parallel()

	want := uuid.New()
	providerID := utils.ProviderIDPrefix + want.String()
	got, err := utils.ServerUUIDFromNode(&v1.Node{Spec: v1.NodeSpec{ProviderID: providerID}})
	require.NoError(t, err)
	require.Equal(t, want, got)

	got, err = utils.ServerUUIDFromNode(&v1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Annotations: map[string]string{
				"infrastructure.cluster.x-k8s.io/upcloud-vm-uuid": want.String(),
			},
		},
	})
	require.NoError(t, err)
	require.Equal(t, want, got)
}

func TestServerInterfacesByType(t *testing.T) {
	t.Parallel()

	want := upcloud.ServerInterfaceSlice{
		{Index: 1, Type: "private"},
		{Index: 3, Type: "private"},
	}
	got := utils.ServerInterfacesByType(&upcloud.ServerDetails{
		Networking: upcloud.ServerNetworking{
			Interfaces: upcloud.ServerInterfaceSlice{
				{Index: 0, Type: "public"},
				{Index: 1, Type: "private"},
				{Index: 2, Type: "utility"},
				{Index: 3, Type: "private"},
			},
		},
	}, "private")
	require.Equal(t, want, got)
}

func TestErrIsHTTPStatusNotFound(t *testing.T) {
	t.Parallel()

	require.True(t, utils.ErrIsHTTPStatusNotFound(&upcloud.Problem{Status: http.StatusNotFound}))
	require.False(t, utils.ErrIsHTTPStatusNotFound(&upcloud.Problem{Status: http.StatusInternalServerError}))
	require.False(t, utils.ErrIsHTTPStatusNotFound(errors.New("test")))
}
