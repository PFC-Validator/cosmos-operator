package v1_test

import (
	"testing"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	ctrl "sigs.k8s.io/controller-runtime"

	cosmosv1 "github.com/bryanlabs/cosmos-operator/api/v1"
	"github.com/stretchr/testify/require"
)

// Canary mode must register the CosmosFullNode Go types under the
// CanaryCosmosFullNode kind and ONLY that kind, so every watch, fetch, own and
// status write resolves through the scheme to canarycosmosfullnodes.
func TestCanaryAddToScheme(t *testing.T) {
	s := runtime.NewScheme()
	require.NoError(t, cosmosv1.CanaryAddToScheme(s))

	crd := &cosmosv1.CosmosFullNode{}
	gvks, _, err := s.ObjectKinds(crd)
	require.NoError(t, err)
	require.Len(t, gvks, 1)
	require.Equal(t, schema.GroupVersionKind{
		Group: "cosmos.strange.love", Version: "v1", Kind: "CanaryCosmosFullNode",
	}, gvks[0])

	// Unstructured lookups use the forward map.
	u := &unstructured.Unstructured{}
	u.SetGroupVersionKind(gvks[0])
	require.NoError(t, s.Convert(u, crd, nil))

	// Owner references are computed from the scheme: a canary-mode operator
	// stamps CanaryCosmosFullNode owner refs, so garbage collection and
	// IsControlledBy checks work against the canary kind.
	crd.Name = "gaia"
	crd.UID = "deadbeef"
	var pod corev1.Pod
	require.NoError(t, ctrl.SetControllerReference(crd, &pod, s))
	require.Equal(t, "CanaryCosmosFullNode", pod.OwnerReferences[0].Kind)
	require.Equal(t, "cosmos.strange.love/v1", pod.OwnerReferences[0].APIVersion)
}
