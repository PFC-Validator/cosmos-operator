package fullnode

import (
	"context"
	"errors"
	"testing"

	cosmosv1 "github.com/bryanlabs/cosmos-operator/api/v1"
	"github.com/stretchr/testify/require"
	"golang.org/x/sync/errgroup"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type threadUnsafeClient struct {
	client.Client
	UpdateCount int
}

func (t *threadUnsafeClient) Get(ctx context.Context, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
	return nil
}

func (t *threadUnsafeClient) Update(ctx context.Context, obj client.Object, opts ...client.UpdateOption) error {
	t.UpdateCount++
	return nil
}

func (t *threadUnsafeClient) Status() client.StatusWriter { return subWriter{update: t.Update} }

func TestStatusClient_SyncUpdate(t *testing.T) {
	type mClient = mockClient[*cosmosv1.CosmosFullNode]

	ctx := context.Background()

	t.Run("happy path", func(t *testing.T) {
		var (
			mock    mClient
			stubCRD cosmosv1.CosmosFullNode
		)
		stubCRD.Status.Phase = "test-phase"
		stubCRD.Name = "test"
		stubCRD.Namespace = "default"
		mock.Object = stubCRD

		c := NewStatusClient(&mock)
		key := client.ObjectKey{Name: "test", Namespace: "default"}
		msg := ptr("Here's test message")
		err := c.SyncUpdate(ctx, key, func(status *cosmosv1.FullNodeStatus) {
			status.StatusMessage = msg
		})

		require.NoError(t, err)

		require.Equal(t, key, mock.GetObjectKey)
		require.Equal(t, 1, mock.UpdateCount)

		updated := mock.LastUpdateObject
		want := stubCRD.DeepCopy()
		want.Status.StatusMessage = msg
		require.Equal(t, want.ObjectMeta, updated.ObjectMeta)
		require.Equal(t, want.Status, updated.Status)
	})

	t.Run("concurrency", func(t *testing.T) {
		var mock threadUnsafeClient
		c := NewStatusClient(&mock)
		key := client.ObjectKey{Name: "test", Namespace: "default"}
		const total = 10
		var eg errgroup.Group
		for i := 0; i < total; i++ {
			eg.Go(func() error {
				return c.SyncUpdate(ctx, key, func(status *cosmosv1.FullNodeStatus) {})
			})
		}

		require.NoError(t, eg.Wait())
		require.Equal(t, 10, mock.UpdateCount)
	})

	t.Run("get error", func(t *testing.T) {
		var (
			mock mClient
		)
		mock.GetObjectErr = errors.New("get boom")

		c := NewStatusClient(&mock)
		key := client.ObjectKey{Name: "test", Namespace: "default"}
		err := c.SyncUpdate(ctx, key, nil)

		require.Error(t, err)
		require.EqualError(t, err, "get boom")
		require.Nil(t, mock.LastUpdateObject)
	})

	t.Run("update error", func(t *testing.T) {
		var (
			mock    mClient
			stubCRD cosmosv1.CosmosFullNode
		)
		mock.Object = stubCRD
		mock.UpdateErr = errors.New("update boom")

		c := NewStatusClient(&mock)
		key := client.ObjectKey{Name: "test", Namespace: "default"}
		err := c.SyncUpdate(ctx, key, func(status *cosmosv1.FullNodeStatus) {})

		require.Error(t, err)
		require.EqualError(t, err, "update boom")
	})

	t.Run("retries on conflict then succeeds", func(t *testing.T) {
		var stubCRD cosmosv1.CosmosFullNode
		stubCRD.Name = "test"
		stubCRD.Namespace = "default"
		mock := &conflictClient{conflictsRemaining: 2}
		mock.Object = stubCRD

		c := NewStatusClient(mock)
		key := client.ObjectKey{Name: "test", Namespace: "default"}

		var updateCalls int
		err := c.SyncUpdate(ctx, key, func(status *cosmosv1.FullNodeStatus) {
			updateCalls++
			status.Phase = "after-retry"
		})

		require.NoError(t, err)
		// Two rejected attempts plus the one that lands.
		require.Equal(t, 3, mock.UpdateCount)
		// Each attempt must re-read, otherwise the retry rebuilds on the same stale resourceVersion.
		require.Equal(t, 3, mock.GetCount)
		require.Equal(t, 3, updateCalls)
		require.Equal(t, cosmosv1.FullNodePhase("after-retry"), mock.LastStatus.Phase)
	})

	t.Run("gives up on persistent conflict", func(t *testing.T) {
		var stubCRD cosmosv1.CosmosFullNode
		mock := &conflictClient{conflictsRemaining: 1000}
		mock.Object = stubCRD

		c := NewStatusClient(mock)
		key := client.ObjectKey{Name: "test", Namespace: "default"}
		err := c.SyncUpdate(ctx, key, func(status *cosmosv1.FullNodeStatus) {})

		require.Error(t, err)
		require.True(t, apierrors.IsConflict(err), "expected a conflict error, got %v", err)
		// Bounded, so a wedged object cannot spin forever inside one reconcile.
		require.Less(t, mock.UpdateCount, 1000)
	})

	t.Run("does not retry non-conflict errors", func(t *testing.T) {
		var stubCRD cosmosv1.CosmosFullNode
		mock := &conflictClient{updateErr: errors.New("not a conflict")}
		mock.Object = stubCRD

		c := NewStatusClient(mock)
		key := client.ObjectKey{Name: "test", Namespace: "default"}
		err := c.SyncUpdate(ctx, key, func(status *cosmosv1.FullNodeStatus) {})

		require.Error(t, err)
		require.EqualError(t, err, "not a conflict")
		require.Equal(t, 1, mock.UpdateCount)
	})

	t.Run("reads through the supplied reader", func(t *testing.T) {
		var stubCRD cosmosv1.CosmosFullNode
		stubCRD.Status.Phase = "from-reader"
		stubCRD.Name = "test"
		stubCRD.Namespace = "default"

		writer := &conflictClient{}
		writer.Object = cosmosv1.CosmosFullNode{}
		reader := &conflictClient{}
		reader.Object = stubCRD

		c := NewStatusClientWithReader(writer, reader)
		key := client.ObjectKey{Name: "test", Namespace: "default"}

		var observed cosmosv1.FullNodePhase
		err := c.SyncUpdate(ctx, key, func(status *cosmosv1.FullNodeStatus) {
			observed = status.Phase
		})

		require.NoError(t, err)
		require.Equal(t, cosmosv1.FullNodePhase("from-reader"), observed, "should read via reader, not the writing client")
		require.Equal(t, 1, reader.GetCount)
		require.Equal(t, 0, writer.GetCount)
		require.Equal(t, 1, writer.UpdateCount)
	})
}

// conflictClient returns apierrors.NewConflict for the first conflictsRemaining status updates,
// then succeeds. It counts Gets so a test can prove each retry re-read the object.
type conflictClient struct {
	client.Client

	Object      any
	updateErr   error
	GetCount    int
	UpdateCount int

	conflictsRemaining int
	LastStatus         cosmosv1.FullNodeStatus
}

func (c *conflictClient) Get(ctx context.Context, key client.ObjectKey, obj client.Object, _ ...client.GetOption) error {
	c.GetCount++
	if ref, ok := obj.(*cosmosv1.CosmosFullNode); ok && c.Object != nil {
		*ref = c.Object.(cosmosv1.CosmosFullNode)
	}
	return nil
}

func (c *conflictClient) Update(ctx context.Context, obj client.Object, _ ...client.UpdateOption) error {
	c.UpdateCount++
	if crd, ok := obj.(*cosmosv1.CosmosFullNode); ok {
		c.LastStatus = crd.Status
	}
	if c.updateErr != nil {
		return c.updateErr
	}
	if c.conflictsRemaining > 0 {
		c.conflictsRemaining--
		return apierrors.NewConflict(
			schema.GroupResource{Group: "cosmos.strange.love", Resource: "cosmosfullnodes"},
			obj.GetName(),
			errors.New("the object has been modified"),
		)
	}
	return nil
}

func (c *conflictClient) Status() client.StatusWriter { return subWriter{update: c.Update} }

// subWriter adapts a plain Update func to client.SubResourceWriter, whose Create signature differs
// from Client.Create so a single type cannot satisfy both interfaces.
type subWriter struct {
	update func(context.Context, client.Object, ...client.UpdateOption) error
}

func (w subWriter) Create(context.Context, client.Object, client.Object, ...client.SubResourceCreateOption) error {
	panic("implement me")
}

func (w subWriter) Update(ctx context.Context, obj client.Object, _ ...client.SubResourceUpdateOption) error {
	return w.update(ctx, obj)
}

func (w subWriter) Patch(context.Context, client.Object, client.Patch, ...client.SubResourcePatchOption) error {
	panic("implement me")
}
