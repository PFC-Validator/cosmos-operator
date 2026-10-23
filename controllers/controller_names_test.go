package controllers

import (
	"testing"

	cosmosv1 "github.com/bryanlabs/cosmos-operator/api/v1"
	cosmosv1alpha1 "github.com/bryanlabs/cosmos-operator/api/v1alpha1"
	"github.com/bryanlabs/cosmos-operator/internal/cosmos"
	"github.com/stretchr/testify/require"
)

// controller-runtime requires controller names to be unique within a manager, and rejects a
// duplicate at startup with "controller with name X already exists". Several controllers watch
// CosmosFullNode, so without an explicit Named() they all default to the same name and the
// operator crashes on boot. This caught nothing before the 0.19 upgrade because 0.13 allowed it.
func TestControllerNamesAreUnique(t *testing.T) {
	names := []string{
		cosmosv1.CosmosFullNodeController,
		cosmosv1.SelfHealingController,
		cosmos.CacheControllerName,
		cosmosv1alpha1.ScheduledVolumeSnapshotController,
	}

	seen := make(map[string]bool, len(names))
	for _, n := range names {
		require.NotEmpty(t, n, "a controller name must not be empty, it would default to the kind")
		require.False(t, seen[n], "duplicate controller name %q; the manager refuses to start", n)
		seen[n] = true
	}
}
