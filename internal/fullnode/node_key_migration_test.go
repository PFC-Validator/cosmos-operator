package fullnode

import (
	"context"
	"encoding/json"
	"testing"

	cosmosv1 "github.com/bryanlabs/cosmos-operator/api/v1"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// Regenerating a node key changes the node's p2p identity, so every peer holding the old
// <node-id>@<address> fails the handshake. These tests pin the resolution order that makes
// upgrading from ConfigMap-stored keys non-destructive.
func TestNodeKeyCollector_SecretMigration(t *testing.T) {
	ctx := context.Background()
	const namespace = "test"

	newCRD := func() *cosmosv1.CosmosFullNode {
		var crd cosmosv1.CosmosFullNode
		crd.Name = "osmosis"
		crd.Namespace = namespace
		crd.Spec.Replicas = 1
		return &crd
	}

	// A syntactically valid node key to plant in fixtures.
	validKey := func(t *testing.T) []byte {
		t.Helper()
		nk, err := randNodeKey()
		require.NoError(t, err)
		b, err := json.Marshal(nk)
		require.NoError(t, err)
		return b
	}

	key := client.ObjectKey{Name: "osmosis-0", Namespace: namespace}

	t.Run("adopts an existing ConfigMap key instead of generating a new identity", func(t *testing.T) {
		existing := validKey(t)
		var mock mockClient[*corev1.ConfigMap]
		mock.ObjectList = corev1.ConfigMapList{Items: []corev1.ConfigMap{{
			ObjectMeta: metaFor("osmosis-0", namespace),
			Data:       map[string]string{nodeKeyFile: string(existing)},
		}}}

		got, err := NewNodeKeyCollector(&mock).Collect(ctx, newCRD())
		require.NoError(t, err)
		require.Equal(t, existing, got[key].MarshaledNodeKey,
			"upgrading must preserve the p2p identity already in use")
	})

	t.Run("prefers the secret over the configmap", func(t *testing.T) {
		fromSecret, fromConfigMap := validKey(t), validKey(t)
		require.NotEqual(t, fromSecret, fromConfigMap)

		var mock multiListClient
		mock.configMaps = corev1.ConfigMapList{Items: []corev1.ConfigMap{{
			ObjectMeta: metaFor("osmosis-0", namespace),
			Data:       map[string]string{nodeKeyFile: string(fromConfigMap)},
		}}}
		mock.secrets = corev1.SecretList{Items: []corev1.Secret{{
			ObjectMeta: metaFor("osmosis-0-node-key", namespace),
			Data:       map[string][]byte{nodeKeyFile: fromSecret},
		}}}

		got, err := NewNodeKeyCollector(&mock).Collect(ctx, newCRD())
		require.NoError(t, err)
		require.Equal(t, fromSecret, got[key].MarshaledNodeKey,
			"once migrated, the Secret is authoritative and a stale ConfigMap must not win")
	})

	t.Run("generates only when no key exists anywhere", func(t *testing.T) {
		var mock multiListClient
		got, err := NewNodeKeyCollector(&mock).Collect(ctx, newCRD())
		require.NoError(t, err)
		require.NotEmpty(t, got[key].MarshaledNodeKey)
		require.Equal(t, "tendermint/PrivKeyEd25519", got[key].NodeKey.PrivKey.Type)
	})

	t.Run("each replica gets a distinct identity", func(t *testing.T) {
		crd := newCRD()
		crd.Spec.Replicas = 3
		var mock multiListClient

		got, err := NewNodeKeyCollector(&mock).Collect(ctx, crd)
		require.NoError(t, err)
		require.Len(t, got, 3)

		ids := map[string]bool{}
		for k, v := range got {
			id := v.NodeKey.ID()
			require.False(t, ids[id], "duplicate node ID for %s", k.Name)
			ids[id] = true
		}
	})
}

func metaFor(name, namespace string) metav1.ObjectMeta {
	return metav1.ObjectMeta{Name: name, Namespace: namespace}
}

// multiListClient serves ConfigMaps and Secrets from the same client, which the collector needs
// because it consults both.
type multiListClient struct {
	mockClient[*corev1.ConfigMap]
	configMaps corev1.ConfigMapList
	secrets    corev1.SecretList
}

func (m *multiListClient) List(ctx context.Context, list client.ObjectList, _ ...client.ListOption) error {
	switch ref := list.(type) {
	case *corev1.ConfigMapList:
		*ref = m.configMaps
	case *corev1.SecretList:
		*ref = m.secrets
	}
	return nil
}
