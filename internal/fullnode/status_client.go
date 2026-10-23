package fullnode

import (
	"context"
	"sync"

	cosmosv1 "github.com/bryanlabs/cosmos-operator/api/v1"
	"k8s.io/client-go/util/retry"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type semaphore chan struct{}

func newSem() semaphore {
	return make(semaphore, 1)
}

func (s semaphore) Acquire() {
	s <- struct{}{}
}

func (s semaphore) Release() {
	<-s
}

type StatusClient struct {
	sems   sync.Map
	client client.Client
	reader client.Reader
}

func NewStatusClient(c client.Client) *StatusClient {
	return &StatusClient{client: c, reader: c}
}

// NewStatusClientWithReader returns a StatusClient that reads through the given reader instead of
// the caching client.
//
// Pass manager.GetAPIReader() here. The cached client can return an object whose resourceVersion
// lags the API server, and Status().Update() rejects a stale resourceVersion with a conflict. That
// turns into a hot loop: the update fails, the reconcile aborts before it applies the change the
// status was signaling, the triggering condition never clears, and the next pass repeats it.
func NewStatusClientWithReader(c client.Client, reader client.Reader) *StatusClient {
	return &StatusClient{client: c, reader: reader}
}

// SyncUpdate synchronizes updates to a CosmosFullNode's status subresource per client.ObjectKey.
// There are several controllers that update a fullnode's status to signal the fullnode controller to take action
// and update the cluster state.
//
// This method minimizes accidentally overwriting status fields by several actors.
//
// The read-modify-write is retried on conflict. The semaphore below serializes writers inside this
// process, but it cannot prevent a conflict caused by a stale read, so both guards are needed.
//
// Server-side-apply, in theory, would be a solution. During testing, however, it resulted in many conflict errors
// and would require non-trivial migration to clear existing deployment's metadata.managedFields.
func (client *StatusClient) SyncUpdate(ctx context.Context, key client.ObjectKey, update func(status *cosmosv1.FullNodeStatus)) error {
	sem, _ := client.sems.LoadOrStore(key, newSem())
	sem.(semaphore).Acquire()
	defer sem.(semaphore).Release()

	return retry.RetryOnConflict(retry.DefaultRetry, func() error {
		var crd cosmosv1.CosmosFullNode
		if err := client.reader.Get(ctx, key, &crd); err != nil {
			return err
		}

		update(&crd.Status)

		return client.client.Status().Update(ctx, &crd)
	})
}
