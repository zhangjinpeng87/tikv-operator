# TiKV Operator Refactoring Summary

## Overview

This document summarizes the refactoring of tikv-operator to follow tidb-operator v2's 3-layer architecture pattern.

## Current Architecture

- **Single CRD**: `TikvCluster` containing all components (PD, TiKV)
- **StatefulSet-based**: Uses StatefulSet for Pod management
- **Informer-based**: Uses Kubernetes informers and workqueues
- **Monolithic controller**: Single controller managing everything

## Target Architecture

### 3-Layer CRD Structure

```
Cluster (top-level namespace)
├── PDGroup (manages PD replicas)
│   ├── PD (instance 1)
│   ├── PD (instance 2)
│   └── PD (instance 3)
└── TiKVGroup (manages TiKV replicas)
    ├── TiKV (instance 1)
    ├── TiKV (instance 2)
    └── TiKV (instance 3)
```

### Key Components

1. **Cluster CRD** (`pkg/apis/core/v1alpha1/cluster_types.go`)
   - Common configurations
   - Feature gates
   - Cluster-level status

2. **Group CRDs** (`pkg/apis/core/v1alpha1/pd_types.go`, `tikv_types.go`)
   - PDGroup: Manages PD instances
   - TiKVGroup: Manages TiKV instances
   - Replica management
   - Scaling policies

3. **Instance CRDs** (`pkg/apis/core/v1alpha1/pd_types.go`, `tikv_types.go`)
   - PD: Individual PD instance (manages Pod directly)
   - TiKV: Individual TiKV instance (manages Pod directly)
   - Pod lifecycle management
   - Status sync

### Controllers

1. **Cluster Controller** (`pkg/controllers/cluster/`)
   - Watches Cluster CR
   - Watches all Group CRs
   - Aggregates status

2. **Group Controllers** (`pkg/controllers/pdgroup/`, `tikvgroup/`)
   - Watches Group CR
   - Owns Instance CRs
   - Manages replicas (creates/deletes instances)

3. **Instance Controllers** (`pkg/controllers/pd/`, `tikv/`)
   - Watches Instance CR
   - Owns Pod, ConfigMap, PVC
   - Direct Pod management (no StatefulSet)

### Main Changes

#### 1. Remove StatefulSet Dependency

**Before:**
```go
// Create StatefulSet
statefulSet := &appsv1.StatefulSet{...}
setControl.CreateStatefulSet(statefulSet)
```

**After:**
```go
// Create Pod directly
pod := &corev1.Pod{...}
client.Create(ctx, pod)
```

#### 2. Direct Pod Management

- Create Pods with stable names based on instance name
- Manage PVCs per Pod (not per StatefulSet)
- Handle Pod updates directly
- Implement own ordering logic if needed

#### 3. Controller-Runtime Framework

**Before:**
```go
// Informer-based
tcInformer.Informer().AddEventHandler(...)
wait.Forever(func() { controller.Run(workers, stopCh) }, waitDuration)
```

**After:**
```go
// Controller-runtime
ctrl.NewControllerManagedBy(mgr).For(&v1alpha1.PD{}).
    Owns(&corev1.Pod{}).
    Complete(r)
```

#### 4. Group Manages Instances

**Group Controller Logic:**
```go
func (r *PDGroupReconciler) Reconcile(ctx context.Context, req ctrl.Request) {
    // Get PDGroup
    pdGroup := &v1alpha1.PDGroup{}
    
    // List existing PD instances
    pdList := &v1alpha1.PDList{}
    client.List(ctx, pdList, ...)
    
    // Scale: Create/Delete PD instances based on replicas
    desired := *pdGroup.Spec.Replicas
    current := len(pdList.Items)
    
    if desired > current {
        // Create new PD instances
        for i := current; i < desired; i++ {
            pd := &v1alpha1.PD{...}
            client.Create(ctx, pd)
        }
    } else if desired < current {
        // Delete PD instances
        // Handle graceful deletion (offline then delete)
    }
}
```

#### 5. Instance Manages Pod

**Instance Controller Logic:**
```go
func (r *PDReconciler) Reconcile(ctx context.Context, req ctrl.Request) {
    // Get PD instance
    pd := &v1alpha1.PD{}
    
    // Create/Update Pod
    pod := buildPod(pd)
    client.CreateOrUpdate(ctx, pod)
    
    // Create/Update ConfigMap
    cm := buildConfigMap(pd)
    client.CreateOrUpdate(ctx, cm)
    
    // Create/Update PVCs
    for _, vol := range pd.Spec.Volumes {
        pvc := buildPVC(pd, vol)
        client.CreateOrUpdate(ctx, pvc)
    }
    
    // Sync status from Pod/PD API
    syncStatus(pd, pod)
}
```

## Migration Path

### Step 1: Add New API (Non-Breaking)

- Add new API types alongside old API
- Both can coexist
- Old API marked as deprecated

### Step 2: Implement New Controllers

- Implement controllers for new API
- Run in parallel with old controllers (feature flag)
- Test thoroughly

### Step 3: Migration Tool

- Create conversion/migration tool
- Convert old `TikvCluster` to new structure
- Validate conversion

### Step 4: Deprecate Old API

- Mark old API as deprecated
- Provide migration guide
- Set deprecation timeline

### Step 5: Remove Old API

- Remove old API after migration period
- Remove old controllers
- Clean up code

## Benefits

1. **No StatefulSet Limitations**
   - Selective Pod scaling by index
   - Pre/post hooks for scaling
   - Volume template updates

2. **Better Extensibility**
   - Easy to add new components
   - Component-level controllers
   - Clear separation of concerns

3. **Fine-Grained Control**
   - Direct Pod management
   - Instance-level operations
   - Better observability

4. **Modern Framework**
   - Controller-runtime best practices
   - Better testability
   - Improved maintainability

## Implementation Status

See `REFACTORING_PLAN.md` for detailed implementation plan.

Current status: **Planning/Design Phase**

Next steps:
1. Create API definitions
2. Implement basic controllers
3. Migrate Pod management logic
4. Test and validate
5. Update documentation

