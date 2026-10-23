package controllers

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	cosmosv1 "github.com/bryanlabs/cosmos-operator/api/v1"
	cosmosv1alpha1 "github.com/bryanlabs/cosmos-operator/api/v1alpha1"
	"github.com/bryanlabs/cosmos-operator/internal/cosmos"
	"github.com/bryanlabs/cosmos-operator/internal/fullnode"
	snapshotv1 "github.com/kubernetes-csi/external-snapshotter/client/v6/apis/volumesnapshot/v1"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"
)

// TestManagerStartsWithAllControllers boots a real API server and registers every controller the
// operator registers, which is the only way to catch startup failures that no unit test sees.
//
// This exists because v0.26.0 shipped a crash: three controllers watch CosmosFullNode and all
// defaulted to the same controller name, which controller-runtime 0.15+ rejects with "controller
// with name cosmosfullnode already exists". Every package test passed; the operator would not boot.
//
// Skips unless KUBEBUILDER_ASSETS is set. Run via `make test-envtest`.
func TestManagerStartsWithAllControllers(t *testing.T) {
	if os.Getenv("KUBEBUILDER_ASSETS") == "" {
		t.Skip("KUBEBUILDER_ASSETS not set; run `make test-envtest`")
	}

	scheme := runtime.NewScheme()
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
	utilruntime.Must(cosmosv1.AddToScheme(scheme))
	utilruntime.Must(cosmosv1alpha1.AddToScheme(scheme))
	utilruntime.Must(snapshotv1.AddToScheme(scheme))

	testEnv := &envtest.Environment{
		CRDDirectoryPaths:     []string{filepath.Join("..", "config", "crd", "bases")},
		ErrorIfCRDPathMissing: true,
	}
	cfg, err := testEnv.Start()
	require.NoError(t, err, "failed to start envtest control plane")
	t.Cleanup(func() { _ = testEnv.Stop() })

	mgr, err := ctrl.NewManager(cfg, ctrl.Options{
		Scheme:  scheme,
		Metrics: metricsserver.Options{BindAddress: "0"},
	})
	require.NoError(t, err)

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	t.Cleanup(cancel)

	// Register everything main.go registers, in the same order.
	cacheController := cosmos.NewCacheController(nil, mgr.GetClient(), mgr.GetEventRecorderFor(cosmos.CacheControllerName))
	require.NoError(t, cacheController.SetupWithManager(ctx, mgr), "cache controller")

	statusClient := newStatusClientForTest(mgr)

	require.NoError(t,
		NewFullNode(mgr.GetClient(), mgr.GetEventRecorderFor(cosmosv1.CosmosFullNodeController), statusClient, cacheController).
			SetupWithManager(ctx, mgr),
		"fullnode controller")

	require.NoError(t,
		NewSelfHealing(mgr.GetClient(), mgr.GetEventRecorderFor(cosmosv1.SelfHealingController), statusClient, nil, cacheController).
			SetupWithManager(ctx, mgr),
		"selfhealing controller: a duplicate controller name fails here")

	// VolumeSnapshot is a third-party CRD that envtest does not install, and main.go treats its
	// absence as a supported condition, so mirror that rather than requiring it.
	snapshotErr := IndexVolumeSnapshots(ctx, mgr)

	require.NoError(t,
		NewStatefulJob(mgr.GetClient(), mgr.GetEventRecorderFor(cosmosv1alpha1.StatefulJobController), snapshotErr != nil).
			SetupWithManager(ctx, mgr),
		"statefuljob controller")

	require.NoError(t,
		NewScheduledVolumeSnapshotReconciler(
			mgr.GetClient(),
			mgr.GetEventRecorderFor(cosmosv1alpha1.ScheduledVolumeSnapshotController),
			statusClient,
			cacheController,
			snapshotErr != nil,
		).SetupWithManager(ctx, mgr),
		"scheduledvolumesnapshot controller")
}

func newStatusClientForTest(mgr ctrl.Manager) *fullnode.StatusClient {
	return fullnode.NewStatusClientWithReader(mgr.GetClient(), mgr.GetAPIReader())
}
