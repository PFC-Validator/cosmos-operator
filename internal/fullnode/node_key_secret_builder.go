package fullnode

import (
	"fmt"

	cosmosv1 "github.com/bryanlabs/cosmos-operator/api/v1"
	"github.com/bryanlabs/cosmos-operator/internal/diff"
	"github.com/bryanlabs/cosmos-operator/internal/kube"
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// nodeKeySecretSuffix is appended to the instance name to form the Secret name.
const nodeKeySecretSuffix = "-node-key"

// NodeKeySecretName returns the Secret holding one instance's node key.
//
// A node key is the node's p2p identity, so each replica needs its own. Peers dial
// <node-id>@<address> and reject a handshake that presents a different identity, which is why
// these are per instance and never shared.
func NodeKeySecretName(crd *cosmosv1.CosmosFullNode, ordinal int32) string {
	return kube.ToName(instanceName(crd, ordinal) + nodeKeySecretSuffix)
}

// BuildNodeKeySecrets returns the Secrets that hold each instance's node key.
//
// Node keys are identity material, so they belong in a Secret rather than a ConfigMap. Secrets can
// be encrypted at rest and are conventionally granted narrower read access.
func BuildNodeKeySecrets(crd *cosmosv1.CosmosFullNode, nodeKeys NodeKeys) ([]diff.Resource[*corev1.Secret], error) {
	secrets := make([]diff.Resource[*corev1.Secret], 0, crd.Spec.Replicas)
	startOrdinal := crd.Spec.Ordinals.Start

	for i := startOrdinal; i < startOrdinal+crd.Spec.Replicas; i++ {
		instance := instanceName(crd, i)
		nodeKey, ok := nodeKeys[client.ObjectKey{Name: instance, Namespace: crd.Namespace}]
		if !ok {
			return nil, kube.UnrecoverableError(fmt.Errorf("node key not found for %s", instance))
		}

		var secret corev1.Secret
		secret.Name = NodeKeySecretName(crd, i)
		secret.Namespace = crd.Namespace
		secret.Kind = "Secret"
		secret.APIVersion = "v1"
		secret.Type = corev1.SecretTypeOpaque
		secret.Labels = defaultLabels(crd,
			kube.InstanceLabel, instance,
			kube.ComponentLabel, "node-key",
		)
		// Data is base64 encoded by the API server. Using Data rather than StringData keeps the
		// built object equal to what is read back, so the differ does not see a phantom change.
		secret.Data = map[string][]byte{
			nodeKeyFile: nodeKey.MarshaledNodeKey,
		}
		kube.NormalizeMetadata(&secret.ObjectMeta)
		secrets = append(secrets, diff.Adapt(&secret, int(i-startOrdinal)))
	}

	return secrets, nil
}
