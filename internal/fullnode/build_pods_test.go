package fullnode

import (
	"fmt"
	"strconv"
	"testing"

	cosmosv1 "github.com/bryanlabs/cosmos-operator/api/v1"
	"github.com/bryanlabs/cosmos-operator/internal/diff"
	"github.com/samber/lo"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func TestBuildPods(t *testing.T) {
	t.Parallel()

	t.Run("happy path with starting ordinal", func(t *testing.T) {
		crd := &cosmosv1.CosmosFullNode{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "agoric",
				Namespace: "test",
			},
			Spec: cosmosv1.FullNodeSpec{
				Replicas:  5,
				ChainSpec: cosmosv1.ChainSpec{Network: "devnet"},
				PodTemplate: cosmosv1.PodSpec{
					Image: "busybox:latest",
				},
				InstanceOverrides: nil,
				Ordinals: cosmosv1.Ordinals{
					Start: 2,
				},
			},
		}

		cksums := make(ConfigChecksums)
		for i := 0; i < int(crd.Spec.Replicas); i++ {
			cksums[client.ObjectKey{Namespace: crd.Namespace, Name: fmt.Sprintf("agoric-%d", i+int(crd.Spec.Ordinals.Start))}] = strconv.Itoa(i + int(crd.Spec.Ordinals.Start))
		}

		pods, err := BuildPods(crd, cksums)
		require.NoError(t, err)
		require.Equal(t, 5, len(pods))

		for i, r := range pods {
			expectedOrdinal := crd.Spec.Ordinals.Start + int32(i)
			require.Equal(t, int64(expectedOrdinal), r.Ordinal(), i)
			require.NotEmpty(t, r.Revision(), i)
			require.Equal(t, strconv.Itoa(int(expectedOrdinal)), r.Object().Annotations["cosmos.strange.love/config-checksum"])
		}

		want := lo.Map([]int{2, 3, 4, 5, 6}, func(i int, _ int) string {
			return fmt.Sprintf("agoric-%d", i)
		})
		got := lo.Map(pods, func(pod diff.Resource[*corev1.Pod], _ int) string { return pod.Object().Name })
		require.Equal(t, want, got)

		pod, err := NewPodBuilder(crd).WithOrdinal(crd.Spec.Ordinals.Start).Build()
		require.NoError(t, err)
		require.Equal(t, pod.Spec, pods[0].Object().Spec)
	})

	t.Run("instance overrides with starting ordinal", func(t *testing.T) {
		const (
			image         = "agoric:latest"
			overrideImage = "some_image:custom"
			overridePod   = "agoric-7"
		)
		crd := &cosmosv1.CosmosFullNode{
			ObjectMeta: metav1.ObjectMeta{
				Name: "agoric",
			},
			Spec: cosmosv1.FullNodeSpec{
				Replicas: 6,
				PodTemplate: cosmosv1.PodSpec{
					Image: image,
				},
				InstanceOverrides: map[string]cosmosv1.InstanceOverridesSpec{
					"agoric-4":  {DisableStrategy: ptr(cosmosv1.DisablePod)},
					"agoric-6":  {DisableStrategy: ptr(cosmosv1.DisableAll)},
					overridePod: {Image: overrideImage},
				},
				Ordinals: cosmosv1.Ordinals{
					Start: 2,
				},
			},
		}

		pods, err := BuildPods(crd, nil)
		require.NoError(t, err)
		require.Equal(t, 4, len(pods))

		want := lo.Map([]int{2, 3, 5, 7}, func(i int, _ int) string {
			return fmt.Sprintf("agoric-%d", i)
		})
		got := lo.Map(pods, func(pod diff.Resource[*corev1.Pod], _ int) string { return pod.Object().Name })
		require.Equal(t, want, got)
		for _, pod := range pods {
			image := pod.Object().Spec.Containers[0].Image
			if pod.Object().Name == overridePod {
				require.Equal(t, overrideImage, image)
			} else {
				require.Equal(t, image, image)
			}
		}
	})

	t.Run("scheduled volume snapshot pod candidate with starting ordinal", func(t *testing.T) {
		crd := &cosmosv1.CosmosFullNode{
			ObjectMeta: metav1.ObjectMeta{
				Name: "agoric",
			},
			Spec: cosmosv1.FullNodeSpec{
				Replicas: 6,
				Ordinals: cosmosv1.Ordinals{Start: 2},
			},
			Status: cosmosv1.FullNodeStatus{
				ScheduledSnapshotStatus: map[string]cosmosv1.FullNodeSnapshotStatus{
					"some.scheduled.snapshot.1":       {PodCandidate: "agoric-3"},
					"some.scheduled.snapshot.2":       {PodCandidate: "agoric-4"},
					"some.scheduled.snapshot.ignored": {PodCandidate: "agoric-99"},
				},
			},
		}

		pods, err := BuildPods(crd, nil)
		require.NoError(t, err)
		require.Equal(t, 4, len(pods))

		want := lo.Map([]int{2, 5, 6, 7}, func(i int, _ int) string {
			return fmt.Sprintf("agoric-%d", i)
		})
		got := lo.Map(pods, func(pod diff.Resource[*corev1.Pod], _ int) string { return pod.Object().Name })
		require.Equal(t, want, got)
	})

	t.Run("happy path without starting ordinal", func(t *testing.T) {
		crd := &cosmosv1.CosmosFullNode{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "agoric",
				Namespace: "test",
			},
			Spec: cosmosv1.FullNodeSpec{
				Replicas:  5,
				ChainSpec: cosmosv1.ChainSpec{Network: "devnet"},
				PodTemplate: cosmosv1.PodSpec{
					Image: "busybox:latest",
				},
				InstanceOverrides: nil,
			},
		}

		cksums := make(ConfigChecksums)
		for i := 0; i < int(crd.Spec.Replicas); i++ {
			cksums[client.ObjectKey{Namespace: crd.Namespace, Name: fmt.Sprintf("agoric-%d", i+int(crd.Spec.Ordinals.Start))}] = strconv.Itoa(i + int(crd.Spec.Ordinals.Start))
		}

		pods, err := BuildPods(crd, cksums)
		require.NoError(t, err)
		require.Equal(t, 5, len(pods))

		for i, r := range pods {
			expectedOrdinal := crd.Spec.Ordinals.Start + int32(i)
			require.Equal(t, int64(expectedOrdinal), r.Ordinal(), i)
			require.NotEmpty(t, r.Revision(), i)
			require.Equal(t, strconv.Itoa(int(expectedOrdinal)), r.Object().Annotations["cosmos.strange.love/config-checksum"])
		}

		want := lo.Map([]int{0, 1, 2, 3, 4}, func(i int, _ int) string {
			return fmt.Sprintf("agoric-%d", i)
		})
		got := lo.Map(pods, func(pod diff.Resource[*corev1.Pod], _ int) string { return pod.Object().Name })
		require.Equal(t, want, got)

		pod, err := NewPodBuilder(crd).WithOrdinal(crd.Spec.Ordinals.Start).Build()
		require.NoError(t, err)
		require.Equal(t, pod.Spec, pods[0].Object().Spec)
	})

	t.Run("instance overrides without starting ordinal", func(t *testing.T) {
		const (
			image         = "agoric:latest"
			overrideImage = "some_image:custom"
			overridePod   = "agoric-5"
		)
		crd := &cosmosv1.CosmosFullNode{
			ObjectMeta: metav1.ObjectMeta{
				Name: "agoric",
			},
			Spec: cosmosv1.FullNodeSpec{
				Replicas: 6,
				PodTemplate: cosmosv1.PodSpec{
					Image: image,
				},
				InstanceOverrides: map[string]cosmosv1.InstanceOverridesSpec{
					"agoric-2":  {DisableStrategy: ptr(cosmosv1.DisablePod)},
					"agoric-4":  {DisableStrategy: ptr(cosmosv1.DisableAll)},
					overridePod: {Image: overrideImage},
				},
			},
		}

		pods, err := BuildPods(crd, nil)
		require.NoError(t, err)
		require.Equal(t, 4, len(pods))

		want := lo.Map([]int{0, 1, 3, 5}, func(i int, _ int) string {
			return fmt.Sprintf("agoric-%d", i)
		})
		got := lo.Map(pods, func(pod diff.Resource[*corev1.Pod], _ int) string { return pod.Object().Name })
		require.Equal(t, want, got)
		for _, pod := range pods {
			image := pod.Object().Spec.Containers[0].Image
			if pod.Object().Name == overridePod {
				require.Equal(t, overrideImage, image)
			} else {
				require.Equal(t, image, image)
			}
		}
	})

	t.Run("scheduled volume snapshot pod candidate without starting ordinal", func(t *testing.T) {
		crd := &cosmosv1.CosmosFullNode{
			ObjectMeta: metav1.ObjectMeta{
				Name: "agoric",
			},
			Spec: cosmosv1.FullNodeSpec{
				Replicas: 6,
			},
			Status: cosmosv1.FullNodeStatus{
				ScheduledSnapshotStatus: map[string]cosmosv1.FullNodeSnapshotStatus{
					"some.scheduled.snapshot.1":       {PodCandidate: "agoric-1"},
					"some.scheduled.snapshot.2":       {PodCandidate: "agoric-2"},
					"some.scheduled.snapshot.ignored": {PodCandidate: "agoric-99"},
				},
			},
		}

		pods, err := BuildPods(crd, nil)
		require.NoError(t, err)
		require.Equal(t, 4, len(pods))

		want := lo.Map([]int{0, 3, 4, 5}, func(i int, _ int) string {
			return fmt.Sprintf("agoric-%d", i)
		})
		got := lo.Map(pods, func(pod diff.Resource[*corev1.Pod], _ int) string { return pod.Object().Name })
		require.Equal(t, want, got)
	})
}

func TestBuildPodsInjectedImagePins(t *testing.T) {
	t.Parallel()

	opPin := "ghcr.io/pfc-validator/cosmos-operator:v0.26.4-pfc@sha256:1111111111111111111111111111111111111111111111111111111111111111"
	toolPin := "ghcr.io/pfc-validator/infra-toolkit:v0.2.0@sha256:2222222222222222222222222222222222222222222222222222222222222222"

	crd := &cosmosv1.CosmosFullNode{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "agoric",
			Namespace: "test",
		},
		Spec: cosmosv1.FullNodeSpec{
			Replicas:    1,
			ChainSpec:   cosmosv1.ChainSpec{Network: "devnet"},
			PodTemplate: cosmosv1.PodSpec{Image: "busybox:latest"},
			InjectedImages: &cosmosv1.InjectedImagesSpec{
				Operator:     &opPin,
				InfraToolkit: &toolPin,
			},
		},
	}

	pods, err := BuildPods(crd, nil)
	require.NoError(t, err)
	require.Len(t, pods, 1)

	pod := pods[0].Object()

	// Healthcheck sidecar carries the operator pin.
	var health *corev1.Container
	for i := range pod.Spec.Containers {
		if pod.Spec.Containers[i].Name == "healthcheck" {
			health = &pod.Spec.Containers[i]
		}
	}
	require.NotNil(t, health)
	require.Equal(t, opPin, health.Image)

	// Init containers: every infra-toolkit stamp becomes the pin, the
	// version-check stamp becomes the operator pin, and no default
	// infra-toolkit ref survives.
	gotInit := lo.Map(pod.Spec.InitContainers, func(c corev1.Container, _ int) string { return c.Image })
	require.NotContains(t, gotInit, infraToolImage)
	require.Contains(t, gotInit, toolPin)
	require.Contains(t, gotInit, opPin)
}
