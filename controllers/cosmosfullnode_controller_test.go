package controllers

import (
	"errors"
	"testing"
	"time"

	cosmosv1 "github.com/bryanlabs/cosmos-operator/api/v1"
	"github.com/bryanlabs/cosmos-operator/internal/kube"
	"github.com/stretchr/testify/require"
	"k8s.io/client-go/tools/record"
)

// The controllers package had no tests. These cover the reconcile cadence knob, including the
// guard that stops a bad value turning into a hot reconcile loop.
func TestWithReconcilePeriod(t *testing.T) {
	t.Run("defaults to the historical cadence", func(t *testing.T) {
		r := NewFullNode(nil, nil, nil, nil)
		require.Equal(t, defaultReconcilePeriod, r.reconcilePeriod)
		require.Equal(t, 60*time.Second, r.reconcilePeriod)
	})

	t.Run("applies a supplied period", func(t *testing.T) {
		r := NewFullNode(nil, nil, nil, nil, WithReconcilePeriod(5*time.Minute))
		require.Equal(t, 5*time.Minute, r.reconcilePeriod)
	})

	for _, bad := range []time.Duration{0, -1 * time.Second} {
		t.Run("ignores non-positive period "+bad.String(), func(t *testing.T) {
			r := NewFullNode(nil, nil, nil, nil, WithReconcilePeriod(bad))
			require.Equal(t, defaultReconcilePeriod, r.reconcilePeriod,
				"a non-positive period must not be applied, it would requeue immediately forever")
		})
	}

	t.Run("later options win", func(t *testing.T) {
		r := NewFullNode(nil, nil, nil, nil,
			WithReconcilePeriod(10*time.Second),
			WithReconcilePeriod(30*time.Second),
		)
		require.Equal(t, 30*time.Second, r.reconcilePeriod)
	})
}

// resultWithErr decides whether a failed reconcile is retried or parked for a human. Getting it
// backwards either wedges a recoverable chain or hides an unrecoverable one behind endless retries.
func TestResultWithErr(t *testing.T) {
	newReconciler := func() (*CosmosFullNodeReconciler, *record.FakeRecorder) {
		rec := record.NewFakeRecorder(10)
		r := NewFullNode(nil, rec, nil, nil)
		return r, rec
	}

	t.Run("transient errors requeue and are retried", func(t *testing.T) {
		r, rec := newReconciler()
		var crd cosmosv1.CosmosFullNode

		result, err := r.resultWithErr(&crd, kube.TransientError(errors.New("api server hiccup")))

		require.Error(t, err)
		require.Equal(t, requeueResult, result)
		require.NotZero(t, result.RequeueAfter, "a transient error must schedule another attempt")
		require.Equal(t, cosmosv1.FullNodePhaseTransientError, crd.Status.Phase)
		require.Contains(t, *crd.Status.StatusMessage, "system is retrying")
		require.Contains(t, <-rec.Events, "ErrorTransient")
	})

	t.Run("unrecoverable errors stop and ask for a human", func(t *testing.T) {
		r, rec := newReconciler()
		var crd cosmosv1.CosmosFullNode

		result, err := r.resultWithErr(&crd, kube.UnrecoverableError(errors.New("bad config")))

		require.Error(t, err)
		require.Equal(t, stopResult, result)
		require.Zero(t, result.RequeueAfter, "an unrecoverable error must not spin")
		require.False(t, result.Requeue)
		require.Equal(t, cosmosv1.FullNodePhaseError, crd.Status.Phase)
		require.Contains(t, *crd.Status.StatusMessage, "human intervention required")
		require.Contains(t, <-rec.Events, "Error")
	})

	t.Run("a mixed batch is transient only if every error is", func(t *testing.T) {
		r, _ := newReconciler()
		var crd cosmosv1.CosmosFullNode

		errs := &kube.ReconcileErrors{}
		errs.Append(kube.TransientError(errors.New("transient")))
		errs.Append(kube.UnrecoverableError(errors.New("fatal")))

		result, _ := r.resultWithErr(&crd, errs)
		require.Equal(t, stopResult, result,
			"one unrecoverable error in the batch must not be retried forever")
		require.Equal(t, cosmosv1.FullNodePhaseError, crd.Status.Phase)
	})
}
