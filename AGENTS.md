# AGENTS.md — cosmos-operator

A Kubernetes operator (kubebuilder / controller-runtime) that runs Cosmos SDK / CometBFT
blockchain nodes. **BryanLabs FORK of `strangelove-ventures/cosmos-operator`.** It automates the
full lifecycle of chain fullnodes: pods, per-pod config, peer/node-key collection, PVC snapshots,
storage auto-scaling, and self-healing.

## Where it's used
- Deployed as `cosmos-operator-controller-manager` in the `cosmos-operator-system` namespace
  (image `ghcr.io/bryanlabs/cosmos-operator:v0.25.1`).
- It manages the `CosmosFullNode` CRs that run the chain nodes in the `fullnodes` namespace
  (cosmoshub, provider/testnet, dydx, etc.). Those nodes are what `snapshot-processor` and the
  snapshots / explorer / upgrades services read from.

## What it manages (CRDs, group `cosmos.strange.love`, unchanged from upstream)
- **CosmosFullNode** (`v1`, `api/v1/`) — the flagship CRD. StatefulSet-like, but each pod gets its
  own PVC and per-pod config (peers, node key). Supports FullNode / Sentry / Seed types.
- **ScheduledVolumeSnapshot** (`v1alpha1`) — recurring VolumeSnapshot backups of a fullnode PVC;
  briefly deletes a pod to snapshot, so it needs `replicas >= 2`.
- **StatefulJob** (`v1alpha1`) — runs a Job on a schedule hydrated from a VolumeSnapshot (e.g.
  upload chain data to object storage).
- **SelfHeal** is a sub-spec of CosmosFullNode (not its own CRD): PVC auto-scaling and height-drift
  mitigation, run by `SelfHealingReconciler`.

## BryanLabs customizations (vs upstream)
Branch `codex/cosmos-operator-v0.25.1-manifest` (safety copy: `backup-bryanlabs-customizations`):
- **Module renamed** `strangelove-ventures/cosmos-operator` to `bryanlabs/cosmos-operator` (hard
  fork; all imports use the bryanlabs path, do not mix upstream paths).
- **Dynamic P2P/RPC ports** driven by `CometConfig.P2PPort()/RPCPort()` from the CR spec instead of
  hardcoded constants (lets non-standard chains like Thornode use 27147/27146).
- **Init images** default to `ghcr.io/bryanlabs/infra-toolkit` (not the strangelove one). See the
  `infra-toolkit` repo.
- **Manager image pinned** to `ghcr.io/bryanlabs/cosmos-operator:v0.25.1` in
  `config/manager/kustomization.yaml`.
- Multi-version rollout via `--label-selector=cosmos-operator-version=<tag>` so several operator
  instances can watch disjoint CR sets (see `DEPLOYMENT_PLAN_v0.25.0.md`).

## Code map
- `api/v1/`, `api/v1alpha1/` — CRD type definitions.
- `controllers/` — the 4 reconcilers (cosmosfullnode, selfhealing, scheduledvolumesnapshot, statefuljob).
- `internal/fullnode/` — pod/configmap/PVC/service/RBAC builders, peer + node-key collectors, drift
  detection, PVC autoscaler, genesis/addrbook fetch.
- `internal/volsnapshot/`, `internal/statefuljob/`, `internal/cosmos/` (shared status cache),
  `internal/healthcheck/` (CometBFT RPC health), `internal/version/`.
- `config/` — CRD bases, manager Deployment, RBAC, samples. `docs/` — architecture + runbooks.

## Build & deploy
- `Dockerfile`: 3-stage (rocksdb libs, then `golang:1.23-alpine` CGO build with `rocksdb pebbledb`
  build tags, then `scratch`). Version injected via ldflags into `internal/version`.
  `local.Dockerfile` is the native-arch dev build.
- Image `ghcr.io/bryanlabs/cosmos-operator`. Build amd64 with `--builder worker1`
  (the shared BuildKit on worker-1; see the `image-build` skill).
- Make targets: `make manifests generate test build`, `make docker-build` / `docker-push`,
  `make deploy IMG=...`, `make deploy-prerelease`.

## Gotchas
- Go module path is `bryanlabs`, not `strangelove`; keep imports consistent.
- CRD API group is still `cosmos.strange.love` (domain unchanged); in-cluster CRs use that group.
- Depends on `ghcr.io/bryanlabs/infra-toolkit` for init containers (maintain it alongside).
- RocksDB build needs CGO plus a matching toolchain (the Dockerfile handles it).
- ScheduledVolumeSnapshot will not evict a pod to snapshot unless `replicas >= 2`.
- Chain node images must be heighliner-compatible (uid:gid 1025:1025).

## Keep current
When you change CRDs, controllers, the fork's port/image customizations, or the build, update this
file in the same change.
