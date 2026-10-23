# cosmos-operator v0.25.0 Safe Deployment Plan
**Date:** $(date)
**Target Version:** v0.25.0
**Custom Image:** `ghcr.io/bryanlabs/cosmos-operator:v0.25.0`
**Strategy:** Separate Operator Instance for thornode-2 Testing

## Overview

This plan implements a safe deployment strategy using a separate operator instance to test v0.25.0 on thornode-2 first, before rolling out to all chains. This approach eliminates risk to existing production chains.

## Prerequisites

- [x] v0.25.0 tag created with RPC port customization features
- [x] BryanLabs image customizations applied
- [ ] Docker image built and pushed to `ghcr.io/bryanlabs/cosmos-operator:v0.25.0`
- [ ] Kubernetes context configured for production cluster

## Phase 1: Build and Push Docker Image

```bash
# Build multi-architecture image
docker buildx build --platform linux/amd64,linux/arm64 \
  -t ghcr.io/bryanlabs/cosmos-operator:v0.25.0 \
  --push .

# Verify image exists
docker manifest inspect ghcr.io/bryanlabs/cosmos-operator:v0.25.0
```

## Phase 2: Prepare Test Operator Deployment

### 2.1 Create Test Namespace
```bash
kubectl create namespace cosmos-operator-v025
```

### 2.2 Generate Modified Manifests

Create a new directory for test deployment:
```bash
mkdir -p deployment-v025
cp -r config/ deployment-v025/
```

**Key modifications needed in `deployment-v025/`:**

#### 2.2.1 Update Manager Deployment (`deployment-v025/manager/manager.yaml`)
```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: cosmos-operator-v025-controller-manager
  namespace: cosmos-operator-v025
spec:
  selector:
    matchLabels:
      control-plane: cosmos-operator-v025-controller-manager
  template:
    metadata:
      labels:
        control-plane: cosmos-operator-v025-controller-manager
    spec:
      serviceAccountName: cosmos-operator-v025-controller-manager
      containers:
      - name: manager
        image: ghcr.io/bryanlabs/cosmos-operator:v0.25.0
        # Add label selector arguments to only watch labeled resources
        args:
        - --leader-elect
        - --zap-log-level=info
        - --label-selector=cosmos-operator-version=v025
```

#### 2.2.2 Update RBAC (`deployment-v025/rbac/`)
Update all RBAC files to use new namespace and service account names:
- `service_account.yaml`: Change name to `cosmos-operator-v025-controller-manager`
- `role_binding.yaml`: Update references to new service account
- `leader_election_role_binding.yaml`: Update references

#### 2.2.3 Update Kustomization (`deployment-v025/manager/kustomization.yaml`)
```yaml
resources:
- manager.yaml

images:
- name: ghcr.io/bryanlabs/cosmos-operator
  newTag: v0.25.0

namespace: cosmos-operator-v025
```

## Phase 3: Deploy Test Operator

### 3.1 Apply CRDs (if needed)
```bash
# Check if CRDs need updating
kubectl diff -f config/crd/bases/

# Apply if differences found
kubectl apply -f config/crd/bases/
```

### 3.2 Deploy New Operator
```bash
# Apply RBAC
kubectl apply -f deployment-v025/rbac/ -n cosmos-operator-v025

# Apply Manager
kubectl apply -f deployment-v025/manager/ -n cosmos-operator-v025
```

### 3.3 Verify Deployment
```bash
# Check pod status
kubectl -n cosmos-operator-v025 get pods -w

# Check logs
kubectl -n cosmos-operator-v025 logs deployment/cosmos-operator-v025-controller-manager -f

# Verify the operator is running but not managing any resources yet
kubectl -n cosmos-operator-v025 describe deployment cosmos-operator-v025-controller-manager
```

## Phase 4: Test with thornode-2

### 4.1 Label thornode-2 for New Operator
```bash
# Add label to make new operator manage thornode-2
kubectl label cosmosfullnode thornode-2 cosmos-operator-version=v025

# Verify label was added
kubectl get cosmosfullnode thornode-2 --show-labels
```

### 4.2 Monitor Transition
```bash
# Watch thornode-2 status
kubectl get cosmosfullnode thornode-2 -w

# Check events
kubectl describe cosmosfullnode thornode-2

# Monitor both operators
kubectl -n cosmos-operator-system logs deployment/cosmos-operator-controller-manager -f &
kubectl -n cosmos-operator-v025 logs deployment/cosmos-operator-v025-controller-manager -f &
```

## Phase 5: Test RPC Port Customization

### 5.1 Update thornode-2 with Custom RPC Port
```bash
kubectl patch cosmosfullnode thornode-2 --type='merge' -p='
{
  "spec": {
    "chainSpec": {
      "comet": {
        "rpcListenAddress": "tcp://0.0.0.0:27147"
      }
    }
  }
}'
```

### 5.2 Verify All Components Updated
```bash
# Check pod ports
kubectl describe pod -l app.kubernetes.io/name=thornode-2

# Check service ports  
kubectl get svc -l app.kubernetes.io/name=thornode-2 -o yaml

# Check readiness probes
kubectl get pod -l app.kubernetes.io/name=thornode-2 -o jsonpath='{.items[*].spec.containers[*].readinessProbe}'

# Test RPC connectivity on new port
kubectl port-forward svc/thornode-2-rpc 27147:27147 &
curl http://localhost:27147/health
```

### 5.3 Test Additional Features
```bash
# Test P2P port customization
kubectl patch cosmosfullnode thornode-2 --type='merge' -p='
{
  "spec": {
    "chainSpec": {
      "comet": {
        "p2pListenAddress": "tcp://0.0.0.0:27146"
      }
    }
  }
}'

# Verify P2P service updated
kubectl get svc -l app.kubernetes.io/name=thornode-2,app.kubernetes.io/component=p2p
```

## Phase 6: Validation Checklist

- [ ] New operator pods running successfully
- [ ] thornode-2 managed by new operator (check logs)
- [ ] Custom RPC port (27147) working correctly
- [ ] Custom P2P port (27146) working correctly  
- [ ] All container ports updated in pods
- [ ] Service ports updated correctly
- [ ] Readiness/liveness probes using correct ports
- [ ] Health checks working on new ports
- [ ] No errors in operator logs
- [ ] thornode-2 chain functioning normally
- [ ] Peer connectivity maintained

## Phase 7: Rollback Plan (if needed)

### 7.1 Quick Rollback
```bash
# Remove label to return thornode-2 to original operator
kubectl label cosmosfullnode thornode-2 cosmos-operator-version-

# Verify original operator resumed management
kubectl -n cosmos-operator-system logs deployment/cosmos-operator-controller-manager
```

### 7.2 Cleanup Test Operator
```bash
# Delete test operator
kubectl delete namespace cosmos-operator-v025

# Remove test deployment files
rm -rf deployment-v025/
```

## Phase 8: Production Migration (after successful testing)

### 8.1 Gradual Migration
```bash
# Label additional chains one by one
kubectl label cosmosfullnode <chain-name> cosmos-operator-version=v025

# Monitor each migration
kubectl get cosmosfullnode <chain-name> -w
```

### 8.2 Complete Migration
Once all chains are successfully migrated:

```bash
# Scale down old operator
kubectl -n cosmos-operator-system scale deployment cosmos-operator-controller-manager --replicas=0

# Rename new operator to standard names (optional)
# Or keep both for future testing
```

## Monitoring and Troubleshooting

### Key Log Locations
```bash
# New operator logs
kubectl -n cosmos-operator-v025 logs deployment/cosmos-operator-v025-controller-manager

# Original operator logs  
kubectl -n cosmos-operator-system logs deployment/cosmos-operator-controller-manager

# thornode-2 pod logs
kubectl logs -l app.kubernetes.io/name=thornode-2 -c node

# Health check logs
kubectl logs -l app.kubernetes.io/name=thornode-2 -c healthcheck
```

### Common Issues and Solutions

1. **Operator not picking up thornode-2:**
   - Verify label is correct: `cosmos-operator-version=v025`
   - Check operator label selector configuration

2. **RPC port not updating:**
   - Check pod events: `kubectl describe pod -l app.kubernetes.io/name=thornode-2`
   - Verify configmap contains correct port settings

3. **Health checks failing:**
   - Verify readiness probe port matches RPC port
   - Check health check sidecar configuration

## Success Criteria

- ✅ New operator successfully manages thornode-2
- ✅ Custom RPC port (27147) fully functional
- ✅ All related components (services, probes) updated correctly
- ✅ Zero impact on other chains
- ✅ Easy rollback capability maintained
- ✅ thornode-2 chain health unchanged

## Post-Implementation Notes

- Document any issues encountered and resolutions
- Record performance impact (if any)
- Note any additional configuration needed
- Plan for migrating remaining chains

---

**Execution Status:**
- [ ] Phase 1: Build Image - Completed ____
- [ ] Phase 2: Prepare Manifests - Completed ____  
- [ ] Phase 3: Deploy Test Operator - Completed ____
- [ ] Phase 4: Test thornode-2 - Completed ____
- [ ] Phase 5: Test RPC Ports - Completed ____
- [ ] Phase 6: Validation - Completed ____
- [ ] Phase 7: Production Migration - Completed ____

**Notes:**
_Add execution notes, issues encountered, and resolutions here_

---

**Sign-off:**
- Executed by: _________________________
- Date/Time: _________________________  
- Verified by: _________________________
- Date/Time: _________________________