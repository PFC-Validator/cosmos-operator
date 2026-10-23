package fullnode

import (
	"encoding/json"
	"testing"

	cosmosv1 "github.com/bryanlabs/cosmos-operator/api/v1"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func mockNodeKeys(t *testing.T, crd *cosmosv1.CosmosFullNode) NodeKeys {
	t.Helper()
	keys := make(NodeKeys)
	for i := crd.Spec.Ordinals.Start; i < crd.Spec.Ordinals.Start+crd.Spec.Replicas; i++ {
		nk, err := randNodeKey()
		require.NoError(t, err)
		marshaled, err := json.Marshal(nk)
		require.NoError(t, err)
		keys[client.ObjectKey{Name: instanceName(crd, i), Namespace: crd.Namespace}] = NodeKeyRepresenter{
			NodeKey:          *nk,
			MarshaledNodeKey: marshaled,
		}
	}
	return keys
}

func TestBuildNodeKeySecrets(t *testing.T) {
	t.Run("one secret per replica, each with a distinct key", func(t *testing.T) {
		var crd cosmosv1.CosmosFullNode
		crd.Name = "osmosis"
		crd.Namespace = "test"
		crd.Spec.Replicas = 3
		keys := mockNodeKeys(t, &crd)

		secrets, err := BuildNodeKeySecrets(&crd, keys)
		require.NoError(t, err)
		require.Len(t, secrets, 3)

		seen := map[string]bool{}
		for i, s := range secrets {
			obj := s.Object()
			require.Equal(t, corev1.SecretTypeOpaque, obj.Type)
			require.Contains(t, obj.Data, nodeKeyFile)
			require.NotEmpty(t, obj.Data[nodeKeyFile])
			// A shared node key would make two pods present the same p2p identity, which peers
			// reject on handshake. Uniqueness is the whole point of per-instance secrets.
			require.False(t, seen[string(obj.Data[nodeKeyFile])], "duplicate node key at ordinal %d", i)
			seen[string(obj.Data[nodeKeyFile])] = true
		}
	})

	t.Run("secret name is derived from the instance", func(t *testing.T) {
		var crd cosmosv1.CosmosFullNode
		crd.Name = "osmosis"
		crd.Namespace = "test"
		crd.Spec.Replicas = 1
		crd.Spec.Ordinals.Start = 5
		keys := mockNodeKeys(t, &crd)

		secrets, err := BuildNodeKeySecrets(&crd, keys)
		require.NoError(t, err)
		require.Equal(t, "osmosis-5-node-key", secrets[0].Object().Name)
		require.Equal(t, "osmosis-5-node-key", NodeKeySecretName(&crd, 5))
	})

	t.Run("preserves exact bytes so the differ sees no phantom change", func(t *testing.T) {
		var crd cosmosv1.CosmosFullNode
		crd.Name = "osmosis"
		crd.Namespace = "test"
		crd.Spec.Replicas = 1
		keys := mockNodeKeys(t, &crd)
		want := keys[client.ObjectKey{Name: "osmosis-0", Namespace: "test"}].MarshaledNodeKey

		secrets, err := BuildNodeKeySecrets(&crd, keys)
		require.NoError(t, err)
		require.Equal(t, want, secrets[0].Object().Data[nodeKeyFile])

		again, err := BuildNodeKeySecrets(&crd, keys)
		require.NoError(t, err)
		require.Equal(t, secrets, again, "building twice from the same input must be deterministic")
	})

	t.Run("errors when a node key is missing", func(t *testing.T) {
		var crd cosmosv1.CosmosFullNode
		crd.Name = "osmosis"
		crd.Namespace = "test"
		crd.Spec.Replicas = 2
		keys := mockNodeKeys(t, &crd)
		delete(keys, client.ObjectKey{Name: "osmosis-1", Namespace: "test"})

		_, err := BuildNodeKeySecrets(&crd, keys)
		require.Error(t, err)
		require.Contains(t, err.Error(), "osmosis-1")
	})
}
