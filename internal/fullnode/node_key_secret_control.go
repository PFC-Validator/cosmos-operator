package fullnode

import (
	"context"
	"fmt"

	cosmosv1 "github.com/bryanlabs/cosmos-operator/api/v1"
	"github.com/bryanlabs/cosmos-operator/internal/diff"
	"github.com/bryanlabs/cosmos-operator/internal/kube"
	corev1 "k8s.io/api/core/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// NodeKeySecretControl creates or updates the Secrets holding node keys.
type NodeKeySecretControl struct {
	build  func(*cosmosv1.CosmosFullNode, NodeKeys) ([]diff.Resource[*corev1.Secret], error)
	client Client
}

// NewNodeKeySecretControl returns a valid NodeKeySecretControl.
func NewNodeKeySecretControl(client Client) NodeKeySecretControl {
	return NodeKeySecretControl{
		build:  BuildNodeKeySecrets,
		client: client,
	}
}

// Reconcile creates or updates the node key Secrets mounted into pods.
//
// Deletes are deliberately not performed here. Removing a node key Secret would change the node's
// p2p identity on the next reconcile, so scaling down leaves the Secret behind and the owner
// reference cleans it up when the CosmosFullNode itself is deleted.
func (c NodeKeySecretControl) Reconcile(ctx context.Context, log kube.Logger, crd *cosmosv1.CosmosFullNode, nodeKeys NodeKeys) kube.ReconcileError {
	var secrets corev1.SecretList
	if err := c.client.List(ctx, &secrets,
		client.InNamespace(crd.Namespace),
		client.MatchingFields{kube.ControllerOwnerField: crd.Name},
	); err != nil {
		return kube.TransientError(fmt.Errorf("list existing node key secrets: %w", err))
	}

	current := ptrSlice(secrets.Items)

	want, err := c.build(crd, nodeKeys)
	if err != nil {
		return kube.UnrecoverableError(err)
	}

	diffed := diff.New(current, want)

	for _, secret := range diffed.Creates() {
		log.Info("Creating node key secret", "secretName", secret.Name)
		if err := ctrl.SetControllerReference(crd, secret, c.client.Scheme()); err != nil {
			return kube.TransientError(fmt.Errorf("set controller reference on node key secret %s: %w", secret.Name, err))
		}
		if err := kube.CreateOrUpdate(ctx, c.client, secret); err != nil {
			return kube.TransientError(fmt.Errorf("create node key secret %s: %w", secret.Name, err))
		}
	}

	for _, secret := range diffed.Updates() {
		log.Info("Updating node key secret", "secretName", secret.Name)
		if err := c.client.Update(ctx, secret); err != nil {
			return kube.TransientError(fmt.Errorf("update node key secret %s: %w", secret.Name, err))
		}
	}

	return nil
}
