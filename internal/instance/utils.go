package instance

import (
	"errors"
	"fmt"

	"github.com/UpCloudLtd/upcloud-cloud-controller-manager/internal/utils"
	"github.com/UpCloudLtd/upcloud-go-api/v8/upcloud"
	v1 "k8s.io/api/core/v1"
	cloudprovider "k8s.io/cloud-provider"
)

var (
	errUnsupportedConfiguration = errors.New("unsupported configuration")
	errOutOfScope               = errors.New("out of scope")
)

func instanceMetaFromServerDetails(server *upcloud.ServerDetails) (*cloudprovider.InstanceMetadata, error) {
	meta := cloudprovider.InstanceMetadata{
		ProviderID:   utils.ProviderIDPrefix + server.UUID,
		InstanceType: "", // deprecated
		NodeAddresses: []v1.NodeAddress{
			{
				Type:    v1.NodeHostName,
				Address: server.Hostname,
			},
		},
		Zone:   server.Zone, // UpCloud server lacks region -> so zone = region
		Region: server.Zone,
	}

	privateIP := serverFirstIPAddressByType(server, upcloud.NetworkTypePrivate)
	if privateIP == "" {
		// node must have 1 private network interface
		return nil, fmt.Errorf("%w: server %s doesn't have private network interfaces, there should be at least one", errUnsupportedConfiguration, server.Hostname)
	}
	meta.NodeAddresses = append(meta.NodeAddresses, v1.NodeAddress{
		Type:    v1.NodeInternalIP,
		Address: privateIP,
	})

	if publicIP := serverFirstIPAddressByType(server, upcloud.NetworkTypePublic); publicIP != "" {
		meta.NodeAddresses = append(meta.NodeAddresses, v1.NodeAddress{
			Type:    v1.NodeExternalIP,
			Address: publicIP,
		})
	}
	return &meta, nil
}

func serverFirstIPAddressByType(server *upcloud.ServerDetails, interfaceType string) string {
	if server == nil {
		return ""
	}
	for _, inf := range server.Networking.Interfaces {
		if inf.Type == interfaceType && len(inf.IPAddresses) > 0 {
			return inf.IPAddresses[0].Address
		}
	}
	return ""
}
