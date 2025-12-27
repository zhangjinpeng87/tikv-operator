# Tasks Pattern in TiDB Operator V2

## Overview

The `tasks` package in tidb-operator v2 implements a **task-based architecture** for controller reconciliation. Instead of having all logic in a single large `Reconcile` function, the reconciliation is broken down into smaller, composable tasks that are executed sequentially.

## Architecture

### Task Runner Pattern

The controller uses a `TaskRunner` that executes a series of tasks in sequence. Each task:
- Has a name for logging and debugging
- Returns a `Result` indicating success, failure, or need to retry/wait
- Can be conditionally executed based on state
- Can break the execution chain or continue to the next task

### Task Results

Tasks can return different results:
- **Complete**: Task finished successfully, continue to next task
- **Failed**: Error occurred, stop execution and return error
- **Retry**: Task needs to wait and retry after a duration
- **Wait**: Task needs to wait for external event (e.g., Pod deletion)

## Directory Structure

### `tikv/tasks/` - Instance Controller Tasks

These tasks handle operations for individual TiKV instances:

#### Core Files

- **`state.go`**: Defines the `State` interface and state management
  - Holds TiKV instance, Cluster, Pod, and Store state
  - Provides getters/setters for all stateful data

- **`ctx.go`**: Defines `ReconcileContext`
  - Extends State with PD client and store information
  - Holds reconciliation context (store info, leader counts, etc.)

- **`pod.go`**: Pod management task
  - Creates/updates/deletes Pods
  - Handles Pod recreation when needed
  - Manages graceful deletion with grace period

- **`cm.go`**: ConfigMap management task
  - Creates/updates ConfigMaps with TiKV configuration
  - Handles config hot-reload logic

- **`pvc.go`**: PersistentVolumeClaim management task
  - Creates/updates PVCs for data volumes
  - Handles volume resizing

- **`offline.go`**: Store offline management
  - Handles TiKV store offline operations
  - Manages leader eviction before offline
  - Coordinates with PD API for graceful shutdown

- **`evict_leader.go`**: Leader eviction logic
  - Evicts leaders from TiKV store before deletion
  - Manages eviction timeout

- **`store_labels.go`**: Store label management
  - Syncs labels from Pod to PVC/PV
  - Maintains store ID labels

- **`status.go`**: Status synchronization
  - Updates TiKV instance status from Pod and Store state
  - Syncs store ID, state, and conditions

- **`finalizer.go`**: Finalizer management
  - Adds/removes finalizers for proper cleanup
  - Ensures resources are cleaned up before deletion

### `tikvgroup/tasks/` - Group Controller Tasks

These tasks handle operations for TiKVGroup (managing multiple instances):

#### Core Files

- **`state.go`**: Group state management
  - Holds TiKVGroup, Cluster, and list of TiKV instances

- **`ctx.go`**: Group reconciliation context
  - Provides access to group and instance list

- **`updater.go`**: Instance lifecycle management
  - **Scaling**: Creates/deletes instances based on replicas
  - **Upgrades**: Rolling updates of instances
  - **Topology**: Handles topology-aware scheduling
  - Uses the `updater` package for instance management

- **`svc.go`**: Service management
  - Creates headless services for TiKV group
  - Service shared across all instances in group

## Task Execution Flow

### TiKV Instance Controller (`tikv/tasks/`)

```go
runner := task.NewTaskRunner(reporter,
    // 1. Get TiKV instance
    common.TaskContextObject[scope.TiKV](state, r.Client),
    
    // 2. Check if deleted
    task.IfBreak(common.CondObjectHasBeenDeleted[scope.TiKV](state)),
    
    // 3. Get cluster info
    common.TaskContextCluster[scope.TiKV](state, r.Client),
    
    // 4. Check if paused
    task.IfBreak(common.CondClusterIsPaused(state)),
    
    // 5. Get Pod
    common.TaskContextPod[scope.TiKV](state, r.Client),
    
    // 6. Get info from PD
    tasks.TaskContextInfoFromPD(state, r.PDClientManager),
    
    // 7. Handle offline store
    tasks.TaskOfflineStore(state),
    
    // 8. Manage resources
    tasks.TaskConfigMap(state, r.Client),
    common.TaskPVC[scope.TiKV](state, r.Client, ...),
    tasks.TaskPod(state, r.Client),
    
    // 9. Update status
    tasks.TaskStatus(state, r.Client),
)
```

### TiKVGroup Controller (`tikvgroup/tasks/`)

```go
runner := task.NewTaskRunner(reporter,
    // 1. Get TiKVGroup
    common.TaskContextObject[scope.TiKVGroup](state, r.Client),
    
    // 2. Get cluster
    common.TaskContextCluster[scope.TiKVGroup](state, r.Client),
    
    // 3. Get all TiKV instances
    common.TaskContextSlice[scope.TiKVGroup](state, r.Client),
    
    // 4. Manage services
    tasks.TaskService(state, r.Client),
    
    // 5. Manage instances (scaling, upgrades)
    tasks.TaskUpdater(state, r.Client, r.AllocateFactory),
    
    // 6. Update status
    common.TaskStatusRevisionAndReplicas[scope.TiKVGroup](state),
)
```

## Key Benefits

### 1. **Modularity**
- Each task has a single responsibility
- Easy to test individual tasks
- Clear separation of concerns

### 2. **Composability**
- Tasks can be combined in different orders
- Conditional execution with `IfBreak`, `If`
- Easy to add/remove tasks

### 3. **Observability**
- Each task reports its status
- Task runner provides execution summary
- Easy to debug which task failed

### 4. **Error Handling**
- Tasks can return specific error types
- Retry logic per task
- Graceful error propagation

### 5. **State Management**
- Centralized state object
- Tasks update state, then persist
- Clear state flow through tasks

## Example Task Implementation

```go
func TaskPod(state *ReconcileContext, c client.Client) task.Task {
    return task.NameTaskFunc("Pod", func(ctx context.Context) task.Result {
        expected := newPod(state.Cluster(), state.TiKV(), state.Store)
        pod := state.Pod()
        
        if pod == nil {
            // Create new pod
            if err := c.Apply(ctx, expected); err != nil {
                return task.Fail().With("can't apply pod: %w", err)
            }
            state.SetPod(expected)
            return task.Complete().With("pod is created")
        }
        
        if !pod.GetDeletionTimestamp().IsZero() {
            return task.Wait().With("pod is deleting")
        }
        
        // Update existing pod
        if err := c.Apply(ctx, expected); err != nil {
            return task.Fail().With("can't apply pod: %w", err)
        }
        
        state.SetPod(expected)
        return task.Complete().With("pod is synced")
    })
}
```

## Comparison with Simple Controllers

### Without Tasks Pattern
```go
func (r *Reconciler) Reconcile(ctx context.Context, req ctrl.Request) error {
    // All logic in one function
    tikv := &v1alpha1.TiKV{}
    r.Get(ctx, req.NamespacedName, tikv)
    
    // Check conditions
    if tikv.DeletionTimestamp != nil {
        // handle deletion
    }
    
    // Get cluster
    cluster := &v1alpha1.Cluster{}
    // ...
    
    // Get pod
    pod := &corev1.Pod{}
    // ...
    
    // Create/update resources
    r.reconcileConfigMap(...)
    r.reconcilePVC(...)
    r.reconcilePod(...)
    
    // Update status
    r.updateStatus(...)
    
    return nil
}
```

### With Tasks Pattern
```go
func (r *Reconciler) Reconcile(ctx context.Context, req ctrl.Request) error {
    state := tasks.NewState(req.NamespacedName)
    runner := r.NewRunner(state, reporter)
    return runner.Run(ctx) // Tasks handle everything
}
```

## Task Orchestration

Tasks can use control structures:

- **`IfBreak(condition, ...tasks)`**: Break execution if condition is true
- **`If(condition, ...tasks)`: Execute tasks only if condition is true
- **Sequential execution**: Tasks run in order
- **Early exit**: Tasks can stop execution chain

## Summary

The tasks pattern provides:
1. **Better organization**: Logic split into focused tasks
2. **Easier testing**: Each task can be tested independently
3. **Better debugging**: Clear execution flow and status reporting
4. **Flexibility**: Easy to modify or extend reconciliation logic
5. **Reusability**: Common tasks can be shared across controllers

This pattern makes the controllers more maintainable and testable, which is especially important for complex operations like TiKV cluster management.

