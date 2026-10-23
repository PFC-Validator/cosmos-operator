package statefuljob

import (
	cosmosalpha "github.com/bryanlabs/cosmos-operator/api/v1alpha1"
	"github.com/bryanlabs/cosmos-operator/internal/kube"
)

// ResourceName is the name of all resources created by the controller.
func ResourceName(crd *cosmosalpha.StatefulJob) string {
	return kube.ToName(crd.Name)
}
