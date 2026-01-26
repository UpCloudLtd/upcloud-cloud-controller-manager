// Package utils contains common utility functions that are used by multiple internal packages.
package utils //nolint:revive

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/UpCloudLtd/upcloud-go-api/v8/upcloud"
	"github.com/google/uuid"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/strategicpatch"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	// is the annotation specifying the UpCloud virtual machine UUID
	// it is implied that this annotation is added to k8s node during bootstrapping
	// by CAPU.
	vmUUIDAnnotation string = "infrastructure.cluster.x-k8s.io/upcloud-vm-uuid"

	ProviderIDPrefix string = "upcloud:////"

	// AnnoPrivateNetworkUUID is the annotation specifying the UpCloud virtual machine private
	// network UUID. This is used by load balancer provision, to check if nodes belongs to the same private networks.
	PrivateNetworkUUIDAnnotation string = "infrastructure.cluster.x-k8s.io/upcloud-vm-private-nw-uuid"
)

var errIDNotDetected = errors.New("can't detect ID")

func CreateTwoWayMergePatch(original, modified client.Object, dataStruct interface{}) ([]byte, error) {
	jsOriginal, err := json.Marshal(original)
	if err != nil {
		return nil, fmt.Errorf("failed to serialize current node object: %w", err)
	}

	jsModified, err := json.Marshal(modified)
	if err != nil {
		return nil, fmt.Errorf("failed to serialize modified node object: %w", err)
	}

	return strategicpatch.CreateTwoWayMergePatch(jsOriginal, jsModified, dataStruct)
}

func ServerUUIDFromNode(node *v1.Node) (uuid.UUID, error) {
	if node == nil {
		return uuid.Nil, errors.New("unable to resolve server UUID from the node, node is nil")
	}

	if node.Spec.ProviderID != "" {
		return uuid.Parse(strings.TrimPrefix(node.Spec.ProviderID, ProviderIDPrefix))
	}

	if annotations := node.GetAnnotations(); annotations != nil {
		if vmUUID, exist := annotations[vmUUIDAnnotation]; exist {
			return uuid.Parse(vmUUID)
		}
	}
	return uuid.Nil, fmt.Errorf("neither spec.providerID nor annotations[%s] specified for node %s; %w", vmUUIDAnnotation, node.Name, errIDNotDetected)
}

func ServerInterfacesByType(server *upcloud.ServerDetails, interfaceType string) upcloud.ServerInterfaceSlice {
	infs := make(upcloud.ServerInterfaceSlice, 0)
	if server == nil {
		return infs
	}
	for _, inf := range server.Networking.Interfaces {
		if inf.Type == interfaceType {
			infs = append(infs, inf)
		}
	}
	return infs
}

func ErrIsHTTPStatusNotFound(err error) bool {
	if err != nil {
		var p *upcloud.Problem
		return errors.As(err, &p) && p.Status == http.StatusNotFound
	}
	return false
}
