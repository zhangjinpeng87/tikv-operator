# Rolling Upgrade with Leader Eviction - Operational Guide

## Current State: What's Missing

The current `tikv-operator` implementation **does not support automated rolling upgrades with leader eviction**. You need to perform these operations manually.

### Missing Features

1. ❌ **PD API Integration**: No client to query store status or evict leaders
2. ❌ **Revision Tracking**: No mechanism to track which instances are on new vs old version
3. ❌ **Rolling Update Logic**: TiKVGroup controller doesn't handle version changes
4. ❌ **Leader Eviction**: No automatic leader eviction before Pod deletion
5. ❌ **Upgrade Coordination**: No coordination between instance controller and group controller

## Manual Rolling Upgrade Process

### Prerequisites

1. **PD API Access**: You need access to PD API endpoint
   ```bash
   # Get PD service endpoint
   kubectl get svc -n <namespace> | grep pd
   
   # PD API endpoint: http://<pd-service>:2379
   ```

2. **Tools**: `curl` or `pd-ctl` for PD API calls
   ```bash
   # Install pd-ctl
   # Or use kubectl port-forward
   kubectl port-forward -n <namespace> svc/<cluster-name>-pd 2379:2379
   ```

### Step-by-Step Manual Process

#### Step 1: Identify Current Version and Stores

```bash
# 1. Check current TiKVGroup version
kubectl get tikvgroup <tikvgroup-name> -n <namespace> -o yaml | grep version

# 2. List all TiKV instances
kubectl get tikv -n <namespace> -l tikv.org/cluster=<cluster-name>

# 3. Get store information from PD
curl http://<pd-service>:2379/pd/api/v1/stores
# Or with pd-ctl:
pd-ctl -u http://<pd-service>:2379 store
```

**Example Output:**
```json
{
  "stores": [
    {
      "store": {
        "id": 1,
        "address": "tikv-0.tikv:20160",
        "state_name": "Up"
      },
      "status": {
        "leader_count": 150,
        "region_count": 300
      }
    },
    {
      "store": {
        "id": 2,
        "address": "tikv-1.tikv:20160",
        "state_name": "Up"
      },
      "status": {
        "leader_count": 120,
        "region_count": 250
      }
    }
  ]
}
```

#### Step 2: Update TiKVGroup Version

```bash
# Update the version in TiKVGroup spec
kubectl patch tikvgroup <tikvgroup-name> -n <namespace> --type merge -p '{"spec":{"template":{"spec":{"version":"v7.5.0"}}}}'

# Note: This will NOT trigger upgrades automatically in current implementation
```

**Current Behavior**: The TiKVGroup controller will detect the version change, but it won't trigger upgrades. You need to manually upgrade each instance.

#### Step 3: Upgrade Each Instance (One at a Time)

For each TiKV instance, follow these steps:

##### 3.1 Identify Store ID for the Instance

```bash
# Get Pod name
POD_NAME="<tikv-instance-name>"

# Find store ID from PD API (match by Pod address)
# Store address format: <pod-name>.<headless-service>:<port>
curl http://<pd-service>:2379/pd/api/v1/stores | jq '.stores[] | select(.store.address | contains("'${POD_NAME}'"))'
```

##### 3.2 Evict Leaders from the Store

```bash
STORE_ID=1  # From step 3.1

# Begin evicting leaders (PD will start moving leaders away)
curl -X POST "http://<pd-service>:2379/pd/api/v1/store/${STORE_ID}/leader/evict"

# Verify eviction started
curl "http://<pd-service>:2379/pd/api/v1/schedulers" | grep evict
# Should see: "evict-leader-scheduler-1"

# Wait for leaders to be evicted (check periodically)
while true; do
  LEADER_COUNT=$(curl -s "http://<pd-service>:2379/pd/api/v1/store/${STORE_ID}" | jq '.status.leader_count')
  echo "Leader count: ${LEADER_COUNT}"
  if [ "$LEADER_COUNT" -eq 0 ]; then
    echo "All leaders evicted!"
    break
  fi
  sleep 5
done

# OR wait for a timeout (e.g., 5 minutes max)
# After timeout, proceed even if leaders remain (not recommended for production)
```

**Important Notes:**
- Leader eviction can take time depending on cluster size
- Recommended timeout: 5-10 minutes
- You may proceed after timeout, but risk service disruption

##### 3.3 Update the TiKV Instance Version

```bash
# Update individual TiKV instance version
kubectl patch tikv <tikv-instance-name> -n <namespace> --type merge -p '{"spec":{"version":"v7.5.0"}}'

# Or update the entire spec if needed
kubectl edit tikv <tikv-instance-name> -n <namespace>
```

##### 3.4 Monitor Pod Recreation

The TiKV controller will detect the version change and recreate the Pod:

```bash
# Watch the Pod
kubectl get pod <tikv-instance-name> -n <namespace> -w

# Check Pod events
kubectl describe pod <tikv-instance-name> -n <namespace>
```

**What Happens:**
1. Controller detects spec change (version)
2. Pod is deleted (because Pod spec changed)
3. New Pod is created with new image
4. Pod starts with new version

##### 3.5 Wait for New Pod to be Ready

```bash
# Wait for Pod to be Running and Ready
kubectl wait --for=condition=Ready pod/<tikv-instance-name> -n <namespace> --timeout=300s

# Verify Pod is using new version
kubectl exec -n <namespace> <tikv-instance-name> -- /tikv-server --version
```

##### 3.6 Verify Store is Back Online

```bash
# Check store status in PD
curl "http://<pd-service>:2379/pd/api/v1/store/${STORE_ID}"

# Verify store state is "Up"
# Verify region count is restored
```

##### 3.7 End Leader Eviction

```bash
# Remove evict-leader scheduler
curl -X DELETE "http://<pd-service>:2379/pd/api/v1/scheduler/evict-leader-scheduler-${STORE_ID}"

# Verify scheduler removed
curl "http://<pd-service>:2379/pd/api/v1/schedulers" | grep evict
# Should NOT see: "evict-leader-scheduler-1"
```

##### 3.8 Wait for Stability

```bash
# Monitor leader distribution
curl "http://<pd-service>:2379/pd/api/v1/store/${STORE_ID}"

# Wait a few minutes to ensure cluster is stable
sleep 300
```

##### 3.9 Repeat for Next Instance

Go back to Step 3.1 for the next TiKV instance.

**Upgrade Order Recommendation:**
- Upgrade instances one at a time
- Wait for each instance to be fully ready before proceeding
- Consider upgrading instances with fewer leaders first
- Maintain at least N-1 instances running (where N is total replicas)

## Complete Manual Script Example

Here's a bash script to automate the manual process:

```bash
#!/bin/bash
set -e

NAMESPACE="default"
CLUSTER_NAME="my-cluster"
TIKVGROUP_NAME="my-tikvgroup"
NEW_VERSION="v7.5.0"
PD_SERVICE="${CLUSTER_NAME}-pd"
PD_API="http://${PD_SERVICE}:2379"
EVICT_TIMEOUT=600  # 10 minutes

# Step 1: Get all TiKV instances
echo "Fetching TiKV instances..."
TIKV_INSTANCES=$(kubectl get tikv -n ${NAMESPACE} -l tikv.org/cluster=${CLUSTER_NAME} -o jsonpath='{.items[*].metadata.name}')

# Step 2: Update TiKVGroup version
echo "Updating TiKVGroup version to ${NEW_VERSION}..."
kubectl patch tikvgroup ${TIKVGROUP_NAME} -n ${NAMESPACE} --type merge -p "{\"spec\":{\"template\":{\"spec\":{\"version\":\"${NEW_VERSION}\"}}}}"

# Step 3: Upgrade each instance
for TIKV_NAME in ${TIKV_INSTANCES}; do
  echo "Processing ${TIKV_NAME}..."
  
  # Get store ID
  POD_IP=$(kubectl get pod ${TIKV_NAME} -n ${NAMESPACE} -o jsonpath='{.status.podIP}')
  STORE_INFO=$(curl -s "${PD_API}/pd/api/v1/stores" | jq ".stores[] | select(.store.address | contains(\"${TIKV_NAME}\"))")
  STORE_ID=$(echo ${STORE_INFO} | jq -r '.store.id')
  
  if [ -z "${STORE_ID}" ] || [ "${STORE_ID}" == "null" ]; then
    echo "Warning: Could not find store ID for ${TIKV_NAME}, skipping..."
    continue
  fi
  
  echo "  Store ID: ${STORE_ID}"
  
  # Evict leaders
  echo "  Evicting leaders..."
  curl -X POST "${PD_API}/pd/api/v1/store/${STORE_ID}/leader/evict"
  
  # Wait for leaders to be evicted (with timeout)
  echo "  Waiting for leaders to be evicted (max ${EVICT_TIMEOUT}s)..."
  START_TIME=$(date +%s)
  while true; do
    LEADER_COUNT=$(curl -s "${PD_API}/pd/api/v1/store/${STORE_ID}" | jq -r '.status.leader_count // 0')
    ELAPSED=$(($(date +%s) - ${START_TIME}))
    
    echo "    Leaders remaining: ${LEADER_COUNT} (elapsed: ${ELAPSED}s)"
    
    if [ "${LEADER_COUNT}" -eq 0 ]; then
      echo "  All leaders evicted!"
      break
    fi
    
    if [ ${ELAPSED} -ge ${EVICT_TIMEOUT} ]; then
      echo "  WARNING: Timeout reached, proceeding anyway..."
      break
    fi
    
    sleep 5
  done
  
  # Update instance version
  echo "  Updating TiKV instance version..."
  kubectl patch tikv ${TIKV_NAME} -n ${NAMESPACE} --type merge -p "{\"spec\":{\"version\":\"${NEW_VERSION}\"}}"
  
  # Wait for Pod to be ready
  echo "  Waiting for Pod to be ready..."
  kubectl wait --for=condition=Ready pod/${TIKV_NAME} -n ${NAMESPACE} --timeout=600s || {
    echo "  ERROR: Pod did not become ready!"
    exit 1
  }
  
  # Verify store is online
  echo "  Verifying store is online..."
  sleep 10
  STORE_STATE=$(curl -s "${PD_API}/pd/api/v1/store/${STORE_ID}" | jq -r '.store.state_name')
  if [ "${STORE_STATE}" != "Up" ]; then
    echo "  WARNING: Store state is ${STORE_STATE}, not Up!"
  fi
  
  # End eviction
  echo "  Ending leader eviction..."
  curl -X DELETE "${PD_API}/pd/api/v1/scheduler/evict-leader-scheduler-${STORE_ID}"
  
  # Wait for stability
  echo "  Waiting for stability..."
  sleep 60
  
  echo "  ${TIKV_NAME} upgrade completed!"
done

echo "All upgrades completed!"
```

## What Needs to be Implemented for Automation

### 1. PD Client Integration

**Location**: `pkg/pd/client/`

```go
type PDClient interface {
    // Store operations
    GetStore(ctx context.Context, storeID uint64) (*Store, error)
    ListStores(ctx context.Context) ([]*Store, error)
    
    // Leader eviction
    BeginEvictLeader(ctx context.Context, storeID uint64) error
    EndEvictLeader(ctx context.Context, storeID uint64) error
    IsEvictingLeaders(ctx context.Context, storeID uint64) (bool, error)
    
    // Store state
    GetStoreLeaderCount(ctx context.Context, storeID uint64) (int, error)
}
```

### 2. Revision Tracking in TiKVGroup

**Location**: `pkg/controllers/tikvgroup/`

- Calculate revision hash based on template spec
- Track `updateRevision` and `currentRevision`
- Identify instances on old vs new revision

### 3. Rolling Update Logic in TiKVGroup Controller

**Location**: `pkg/controllers/tikvgroup/controller.go`

```go
func (r *TiKVGroupReconciler) handleUpgrade(ctx context.Context, tikvGroup *v1alpha1.TiKVGroup) error {
    // 1. Calculate new revision
    updateRevision := calculateRevision(tikvGroup.Spec.Template)
    
    // 2. Identify instances on old revision
    oldInstances := filterByRevision(tikvInstances, currentRevision)
    
    // 3. Upgrade one instance at a time (maxUnavailable=1)
    for _, instance := range oldInstances {
        // Wait for instance to be ready
        if !isInstanceReady(instance) {
            return requeue
        }
        
        // Trigger upgrade (via instance controller)
        if err := r.triggerInstanceUpgrade(ctx, instance, updateRevision); err != nil {
            return err
        }
        
        // Wait for upgrade completion
        if err := r.waitForInstanceUpgrade(ctx, instance, updateRevision); err != nil {
            return err
        }
    }
    
    return nil
}
```

### 4. Leader Eviction in TiKV Instance Controller

**Location**: `pkg/controllers/tikv/controller.go`

```go
func (r *TiKVReconciler) Reconcile(ctx context.Context, req ctrl.Request) error {
    // ... existing code ...
    
    // Check if version changed (revision mismatch)
    if needsUpgrade(tikv) {
        // Check if Pod is being upgraded
        if isPodTerminating(pod) {
            // Wait for Pod deletion
            return requeue
        }
        
        // Evict leaders before deleting Pod
        if hasLeaders(tikv) && !isEvicting(tikv) {
            if err := r.beginEvictLeader(ctx, tikv); err != nil {
                return err
            }
            return requeue // Wait for eviction
        }
        
        if isEvicting(tikv) && !leadersEvicted(tikv) {
            return requeue // Still evicting
        }
        
        // Leaders evicted, delete Pod to trigger recreation
        if err := r.deletePod(ctx, pod); err != nil {
            return err
        }
    }
    
    // ... rest of reconciliation ...
}

func (r *TiKVReconciler) beginEvictLeader(ctx context.Context, tikv *v1alpha1.TiKV) error {
    storeID := tikv.Status.ID
    if storeID == "" {
        return fmt.Errorf("store ID not available")
    }
    
    pdClient := r.PDClientManager.GetClient(tikv.Spec.Cluster.Name)
    if err := pdClient.BeginEvictLeader(ctx, storeID); err != nil {
        return err
    }
    
    // Record eviction start time
    tikv.Annotations["evict-leader-begin-time"] = time.Now().Format(time.RFC3339)
    return r.Update(ctx, tikv)
}
```

### 5. Status Updates

**TiKV Status** should include:
- `Revision`: Current revision hash
- `LeaderCount`: Number of leaders on this store
- `EvictingLeaders`: Whether leaders are being evicted

**TiKVGroup Status** should include:
- `UpdateRevision`: Target revision for upgrade
- `CurrentRevision`: Revision all instances are on
- `UpdatedReplicas`: Number of instances on update revision

## Automated Flow (Target State)

Once implemented, the flow will be:

1. **User updates TiKVGroup version**
   ```yaml
   spec:
     template:
       spec:
         version: "v7.5.0"
   ```

2. **TiKVGroup Controller**:
   - Calculates new revision
   - Identifies instances to upgrade
   - Selects one instance (maxUnavailable=1)

3. **TiKV Instance Controller**:
   - Detects revision mismatch
   - Queries PD for store ID and leader count
   - Begins leader eviction if leaders exist
   - Waits for eviction completion (or timeout)
   - Deletes Pod to trigger recreation

4. **Pod Recreation**:
   - New Pod created with new image
   - TiKV starts with new version
   - Store reconnects to PD

5. **Completion**:
   - Verify store is Up
   - End leader eviction scheduler
   - Update instance revision
   - Mark instance as upgraded

6. **Repeat** for next instance

## Summary

**Current State**: Fully manual process requiring:
- Manual PD API calls for leader eviction
- Manual instance-by-instance version updates
- Manual monitoring and coordination

**Required for Automation**:
1. PD client integration
2. Revision tracking
3. Rolling update coordination in TiKVGroup
4. Leader eviction logic in TiKV controller
5. Status tracking for upgrade progress

**Recommendation**: Implement PD client integration first, then add upgrade logic. This will significantly reduce operational burden.

