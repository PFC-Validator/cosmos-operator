package fullnode

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"

	cosmosv1 "github.com/bryanlabs/cosmos-operator/api/v1"
	"github.com/bryanlabs/cosmos-operator/internal/kube"
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

type NodeKey struct {
	PrivKey NodeKeyPrivKey `json:"priv_key"`
}

type NodeKeyPrivKey struct {
	Type  string             `json:"type"`
	Value ed25519.PrivateKey `json:"value"`
}

func (nk NodeKey) ID() string {
	pub := nk.PrivKey.Value.Public()
	hash := sha256.Sum256(pub.(ed25519.PublicKey))
	return hex.EncodeToString(hash[:20])
}

func randNodeKey() (*NodeKey, error) {
	_, pk, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return nil, fmt.Errorf("failed to generate ed25519 node key: %w", err)
	}
	return &NodeKey{
		PrivKey: NodeKeyPrivKey{
			Type:  "tendermint/PrivKeyEd25519",
			Value: pk,
		},
	}, nil
}

// NodeKeyRepresenter represents a NodeKey and its marshaled form. Since NodeKeys can be pulled from ConfigMaps, we store the marshaled form to avoid re-marshaling during ConfigMap creation.
type NodeKeyRepresenter struct {
	NodeKey          NodeKey
	MarshaledNodeKey []byte
}

// Namespace maps an ObjectKey using the instance name to NodeKey.
type NodeKeys map[client.ObjectKey]NodeKeyRepresenter

// NodeKeyCollector finds and collects node key information.
type NodeKeyCollector struct {
	client Client
}

func NewNodeKeyCollector(client Client) *NodeKeyCollector {
	return &NodeKeyCollector{
		client: client,
	}
}

// Collect node key information given the crd.
//
// Node keys are resolved in this order, and the order matters because generating a new key changes
// the node's p2p identity and breaks any peer that has the old <node-id>@<address> recorded:
//
//  1. The operator-managed Secret, which is where node keys live.
//  2. The instance ConfigMap, which is where they used to live. Adopting the existing value here is
//     what makes upgrading non-destructive; the key is then written to the Secret and dropped from
//     the ConfigMap.
//  3. A freshly generated key, only when the instance has no key anywhere.
func (c NodeKeyCollector) Collect(ctx context.Context, crd *cosmosv1.CosmosFullNode) (NodeKeys, kube.ReconcileError) {
	logger := log.FromContext(ctx)
	nodeKeys := make(NodeKeys)

	var cms corev1.ConfigMapList
	if err := c.client.List(ctx, &cms,
		client.InNamespace(crd.Namespace),
		client.MatchingFields{kube.ControllerOwnerField: crd.Name},
	); err != nil {
		return nil, kube.TransientError(fmt.Errorf("list existing configmaps: %w", err))
	}

	var secretList corev1.SecretList
	if err := c.client.List(ctx, &secretList,
		client.InNamespace(crd.Namespace),
		client.MatchingFields{kube.ControllerOwnerField: crd.Name},
	); err != nil {
		return nil, kube.TransientError(fmt.Errorf("list existing node key secrets: %w", err))
	}

	currentCms := ptrSlice(cms.Items)
	currentSecrets := ptrSlice(secretList.Items)

	for i := crd.Spec.Ordinals.Start; i < crd.Spec.Ordinals.Start+crd.Spec.Replicas; i++ {
		var secret corev1.Secret
		secret.Name = NodeKeySecretName(crd, i)
		secret.Namespace = crd.Namespace
		secret = *kube.FindOrDefaultCopy(currentSecrets, &secret)

		var confMap corev1.ConfigMap
		confMap.Name = instanceName(crd, i)
		confMap.Namespace = crd.Namespace
		confMap = *kube.FindOrDefaultCopy(currentCms, &confMap)

		var nodeKeyContent []byte
		var source string
		switch {
		case len(secret.Data[nodeKeyFile]) > 0:
			nodeKeyContent = secret.Data[nodeKeyFile]
			source = "secret"
		case confMap.Data[nodeKeyFile] != "":
			nodeKeyContent = []byte(confMap.Data[nodeKeyFile])
			source = "configmap"
		}

		var nodeKey NodeKey
		var marshaledNodeKey []byte

		if len(nodeKeyContent) > 0 {
			if err := json.Unmarshal(nodeKeyContent, &nodeKey); err != nil {
				return nil, kube.UnrecoverableError(fmt.Errorf("unmarshal node key: %w", err))
			}

			// Preserve the exact bytes. Re-marshaling is not deterministic and would show up as a
			// spurious update on every reconcile.
			marshaledNodeKey = nodeKeyContent

			if source == "configmap" {
				logger.Info("Adopting node key from configmap into secret, p2p identity is preserved",
					"ordinal", i, "secretName", NodeKeySecretName(crd, i))
			}
		} else {
			rNodeKey, err := randNodeKey()
			if err != nil {
				return nil, kube.UnrecoverableError(fmt.Errorf("generate node key: %w", err))
			}
			nodeKey = *rNodeKey

			marshaledNodeKey, err = json.Marshal(nodeKey)
			if err != nil {
				return nil, kube.UnrecoverableError(fmt.Errorf("marshal node key: %w", err))
			}
			logger.Info("Generating new node key", "ordinal", i)
		}

		nodeKeys[client.ObjectKey{Name: instanceName(crd, i), Namespace: crd.Namespace}] = NodeKeyRepresenter{
			NodeKey:          nodeKey,
			MarshaledNodeKey: marshaledNodeKey,
		}
	}
	return nodeKeys, nil
}
