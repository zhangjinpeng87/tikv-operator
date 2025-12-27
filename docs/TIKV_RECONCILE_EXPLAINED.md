# TiKVReconciler::Reconcile Method - Line by Line Explanation

## Method Signature

```go
func (r *TiKVReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error)
```

**Purpose**: This is the main reconciliation function called by controller-runtime whenever a TiKV CR is created, updated, or deleted.

**Parameters**:
- `ctx`: Context for cancellation and timeout handling
- `req`: Request containing the NamespacedName (namespace + name) of the TiKV instance to reconcile

**Returns**:
- `ctrl.Result`: Indicates whether to requeue and after how long
- `error`: Any error that occurred during reconciliation

---

## Line-by-Line Breakdown

### Line 61: Create Logger Context
```go
log := r.Log.WithValues("tikv", req.NamespacedName)
```
**What it does**: Creates a logger with context values (tikv instance name) for structured logging.
**Why**: Makes logs easier to filter and debug by including which TiKV instance is being reconciled.

---

### Lines 63-66: Fetch TiKV Instance
```go
tikv := &v1alpha1.TiKV{}
if err := r.Get(ctx, req.NamespacedName, tikv); err != nil {
    return ctrl.Result{}, client.IgnoreNotFound(err)
}
```
**What it does**:
1. Creates an empty TiKV object
2. Attempts to fetch the TiKV CR from Kubernetes API using the NamespacedName from the request
3. If the TiKV doesn't exist (NotFound error), ignores it and returns (this is normal when deleting)
4. If any other error occurs, returns it (will trigger retry with backoff)

**Why**: We need the TiKV spec to know what to create/manage. If it's been deleted, we can exit early.

---

### Lines 68-72: Handle Deletion
```go
// Check if TiKV is being deleted
if !tikv.DeletionTimestamp.IsZero() {
    // Handle finalizers if needed
    return ctrl.Result{}, nil
}
```
**What it does**:
- Checks if the TiKV CR has a deletion timestamp (means it's being deleted)
- If deleting, returns early (TODO: should handle finalizers here for proper cleanup)

**Why**: During deletion, we need to clean up resources. Currently this is a placeholder - in production you'd:
1. Check if finalizers exist
2. Clean up resources (delete Pod, wait for store to be removed from PD)
3. Remove finalizers to allow deletion

---

### Lines 74-77: Reconcile Service
```go
// Ensure Service exists (headless service for TiKV cluster)
if err := r.reconcileService(ctx, tikv); err != nil {
    return ctrl.Result{}, err
}
```
**What it does**: Ensures a headless Kubernetes Service exists for the TiKV cluster.
**Why**: 
- Headless service (ClusterIP=None) provides stable DNS names for each Pod
- Allows TiKV instances to discover each other via DNS
- Shared across all TiKV instances in the cluster (not owned by individual instance)

**Details** (from `reconcileService`):
- Service name: `{cluster-name}-tikv`
- Headless (ClusterIP=None) for stable Pod DNS
- Selector matches all TiKV Pods in the cluster
- Exposes client port (20160) and status port (20180)

---

### Lines 79-82: Reconcile ConfigMap
```go
// Ensure ConfigMap exists
if err := r.reconcileConfigMap(ctx, tikv); err != nil {
    return ctrl.Result{}, err
}
```
**What it does**: Ensures a ConfigMap exists with TiKV configuration.
**Why**: 
- Stores TiKV's configuration file (TOML format)
- Mounted into the Pod at `/etc/tikv/tikv.toml`
- Per-instance (each TiKV has its own ConfigMap)

**Details** (from `reconcileConfigMap`):
- ConfigMap name: `{tikv-instance-name}-config`
- Contains the config file content from `tikv.Spec.Config`
- Owned by the TiKV instance (so it gets deleted when TiKV is deleted)

---

### Lines 84-87: Reconcile PVCs
```go
// Ensure PVCs exist
if err := r.reconcilePVCs(ctx, tikv); err != nil {
    return ctrl.Result{}, err
}
```
**What it does**: Ensures PersistentVolumeClaims exist for each volume defined in TiKV spec.
**Why**:
- TiKV needs persistent storage for data
- Each volume in `tikv.Spec.Volumes` gets its own PVC
- PVCs provide persistent storage that survives Pod restarts

**Details** (from `reconcilePVCs`):
- Creates one PVC per volume: `{tikv-instance-name}-{volume-name}`
- Uses ReadWriteOnce access mode
- Storage size and StorageClass from volume spec
- Owned by TiKV instance

---

### Lines 89-92: Reconcile Pod
```go
// Ensure Pod exists
if err := r.reconcilePod(ctx, tikv); err != nil {
    return ctrl.Result{}, err
}
```
**What it does**: Ensures the TiKV Pod exists and is configured correctly.
**Why**: 
- The Pod is the actual running container
- Contains the TiKV server process
- This is the core resource that runs TiKV

**Details** (from `reconcilePod`):
- Pod name: Same as TiKV instance name
- Container: TiKV server with proper command-line arguments
- Image: From spec or defaults to `pingcap/tikv:{version}`
- Volumes: Mounts ConfigMap and PVCs
- Environment variables: POD_NAME, HEADLESS_SERVICE, PD_SERVICE for discovery
- Resources: CPU and memory from spec
- Restart policy: Always (so it restarts if crashes)

**Key Pod Configuration**:
- `--addr=0.0.0.0:20160`: Listen on all interfaces
- `--advertise-addr=$(POD_NAME).$(HEADLESS_SERVICE):20160`: Advertised address (stable DNS)
- `--pd=$(PD_SERVICE):2379`: PD cluster endpoint
- `--data-dir=/var/lib/tikv`: Data directory (mounted from PVC)
- `--config=/etc/tikv/tikv.toml`: Config file (mounted from ConfigMap)

---

### Lines 94-97: Update Status
```go
// Update status from Pod
if err := r.updateStatus(ctx, tikv); err != nil {
    return ctrl.Result{}, err
}
```
**What it does**: Updates the TiKV CR's status based on the current state of the Pod.
**Why**: 
- Status reflects the actual state of the TiKV instance
- Used by Group controller to track ready replicas
- Provides visibility to users

**Details** (from `updateStatus`):
- Sets `ObservedGeneration` to track which spec generation has been processed
- Checks Pod phase and ready condition
- Updates store ID and state (currently placeholder, should query PD API)
- If Pod is not running, clears status fields

---

### Line 99: Success Return
```go
return ctrl.Result{}, nil
```
**What it does**: Returns success with no requeue.
**Why**: 
- All resources are reconciled successfully
- No immediate requeue needed (will be triggered by events)
- Controller-runtime will continue watching for changes

---

## Execution Flow Diagram

```
Reconcile Triggered
    ↓
Get TiKV CR from API
    ↓
TiKV exists?
    ├─ No → Return (ignore not found)
    └─ Yes → Continue
        ↓
Being deleted?
    ├─ Yes → Handle deletion (TODO: finalizers)
    └─ No → Continue
        ↓
Reconcile Service (headless, shared)
    ├─ Success → Continue
    └─ Error → Return error
        ↓
Reconcile ConfigMap (per instance)
    ├─ Success → Continue
    └─ Error → Return error
        ↓
Reconcile PVCs (per instance, per volume)
    ├─ Success → Continue
    └─ Error → Return error
        ↓
Reconcile Pod (per instance)
    ├─ Success → Continue
    └─ Error → Return error
        ↓
Update Status (reflect Pod state)
    ├─ Success → Continue
    └─ Error → Return error
        ↓
Return Success (no requeue)
```

## Key Design Decisions

### 1. Service Created by Instance Controller
**Why**: While the service is shared, it's created by the first instance controller that runs. The service is idempotent (CreateOrUpdate), so multiple instances trying to create it is safe.

**Alternative**: Could be created by Group controller (tidb-operator v2 does this).

### 2. Sequential Resource Creation
**Why**: 
- Service must exist before Pod (for DNS)
- ConfigMap must exist before Pod (for volume mount)
- PVCs should exist before Pod (to ensure storage is ready)

**Note**: Kubernetes will handle dependency errors (e.g., Pod creation will fail if PVC doesn't exist, and retry later).

### 3. Direct Pod Management (No StatefulSet)
**Why**: 
- Full control over Pod lifecycle
- Can selectively delete specific Pods
- No StatefulSet limitations (index-based scaling, volume template updates)

### 4. Status Update Last
**Why**: 
- Status reflects the actual state after all changes
- Ensures status matches what was just created/updated
- Other controllers (Group) rely on status

## Error Handling

- **API errors**: Returned immediately, controller-runtime will retry with exponential backoff
- **Resource creation errors**: Returned immediately, will be retried
- **Not found errors**: Ignored for TiKV CR (deletion case), but propagated for Pod (unexpected)

## Reconciliation Guarantees

This implementation ensures:
1. **Idempotency**: Running Reconcile multiple times produces the same result
2. **Desired State**: Resources match the TiKV spec
3. **Dependency Order**: Resources created in correct order
4. **Ownership**: Resources owned by TiKV instance (except Service)

## Missing Features (TODOs)

1. **Finalizer Handling**: Proper cleanup during deletion
2. **PD API Integration**: Query PD to get actual store ID and state
3. **Store Offline**: Handle graceful offline before deletion
4. **Leader Eviction**: Evict leaders before store deletion
5. **Health Checks**: Proper readiness/liveness probes
6. **Graceful Shutdown**: Handle Pod termination gracefully

