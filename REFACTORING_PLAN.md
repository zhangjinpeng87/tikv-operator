# TiKV Operator Refactoring Plan

## Goal

Refactor tikv-operator to follow tidb-operator v2's 3-layer architecture:
- **Cluster**: Top-level abstraction for the TiKV cluster
- **Group**: Component groups (PDGroup, TiKVGroup) for replica management
- **Instance**: Individual instances (PD, TiKV) managing Pods directly

## Key Changes

### 1. Remove StatefulSet Dependency
- Manage Pods directly instead of using StatefulSets
- Full control over Pod lifecycle (create, update, delete)
- Support selective Pod scaling by index
- Native pre/post hooks for scaling operations

### 2. 3-Layer CRD Architecture

#### Layer 1: Cluster
- Contains common configurations and feature gates
- Acts as a "namespace" for all components
- Displays overview status

#### Layer 2: Group (PDGroup, TiKVGroup)
- Manages replicas for a component
- Controls instance lifecycle
- Handles scaling and update policies
- Each component can have multiple groups

#### Layer 3: Instance (PD, TiKV)
- Represents a single Pod instance
- Manages Pod, ConfigMap, and PVCs
- Immutable for most fields (managed by Group)

### 3. Controller-Runtime Framework
- Migrate from informer-based to controller-runtime
- Component-level controllers for fine-grained control
- Better testability and maintainability

## Implementation Steps

### Phase 1: API Definitions

1. **Create new API structure**
   - `pkg/apis/core/v1alpha1/` (new API group)
   - Define Cluster, PDGroup, PD, TiKVGroup, TiKV types
   - Define common types (ClusterReference, Overlay, Volume, etc.)

2. **Key Types to Create**
   - `cluster_types.go`: Cluster CRD
   - `pd_types.go`: PDGroup and PD CRDs
   - `tikv_types.go`: TiKVGroup and TiKV CRDs
   - `common_types.go`: Shared types

### Phase 2: Controllers

1. **Cluster Controller** (`pkg/controllers/cluster/`)
   - Watches Cluster CR
   - Watches all Group CRs (PDGroup, TiKVGroup)
   - Aggregates status from groups
   - Manages cluster-level features

2. **Group Controllers** (`pkg/controllers/pdgroup/`, `pkg/controllers/tikvgroup/`)
   - Watches Group CR
   - Owns Instance CRs
   - Manages replica count
   - Creates/deletes instances based on replicas
   - Handles scaling policies

3. **Instance Controllers** (`pkg/controllers/pd/`, `pkg/controllers/tikv/`)
   - Watches Instance CR
   - Owns Pod, ConfigMap, PVC resources
   - Manages Pod lifecycle directly
   - Syncs status from Pod/PD API to Instance status

### Phase 3: Pod Management

1. **Remove StatefulSet Logic**
   - Delete StatefulSet control interfaces
   - Implement direct Pod creation/update/deletion
   - Handle Pod identity and naming
   - Manage PVC lifecycle

2. **Volume Management**
   - Direct PVC creation per instance
   - Label management (Pod → PVC → PV)
   - Volume resizing support

### Phase 4: Migration of Existing Logic

1. **Scaling Logic**
   - Move from StatefulSet scaling to instance creation/deletion
   - Implement selective Pod scaling
   - Two-step deletion for TiKV (offline then delete)

2. **Upgrade Logic**
   - Rolling updates by updating instance specs
   - Revision tracking per group
   - Graceful upgrades

3. **Failover Logic**
   - Instance-level failover
   - Pod replacement logic
   - Health checking

### Phase 5: Integration

1. **Update Main Controller Manager**
   - Migrate to controller-runtime manager
   - Setup all controllers
   - Configure leader election
   - Health checks

2. **Update Dependencies**
   - Upgrade to modern controller-runtime (v0.20.x)
   - Update Kubernetes dependencies
   - Update Go version

3. **Examples and Documentation**
   - Update example YAMLs
   - Update architecture documentation
   - Migration guide

## Directory Structure

```
tikv-operator/
├── pkg/
│   ├── apis/
│   │   ├── core/
│   │   │   └── v1alpha1/
│   │   │       ├── cluster_types.go
│   │   │       ├── pd_types.go
│   │   │       ├── tikv_types.go
│   │   │       ├── common_types.go
│   │   │       ├── register.go
│   │   │       └── zz_generated.deepcopy.go
│   │   └── tikv/          # Legacy API (deprecated)
│   │       └── v1alpha1/
│   ├── controllers/
│   │   ├── cluster/
│   │   │   └── controller.go
│   │   ├── pdgroup/
│   │   │   └── controller.go
│   │   ├── pd/
│   │   │   └── controller.go
│   │   ├── tikvgroup/
│   │   │   └── controller.go
│   │   └── tikv/
│   │       └── controller.go
│   ├── runtime/           # Runtime utilities
│   ├── volumes/           # Volume management
│   └── client/            # Client utilities
├── cmd/
│   └── tikv-controller-manager/
│       └── main.go        # Controller-runtime manager
└── examples/
    └── basic/
        ├── cluster.yaml
        ├── pd-group.yaml
        └── tikv-group.yaml
```

## Migration Strategy

### Backward Compatibility

The old `TikvCluster` API can be kept initially for backward compatibility:
1. Keep old API definitions
2. Create a conversion layer if needed
3. Deprecate old API gradually
4. Provide migration guide

### Gradual Migration

1. Add new API alongside old API
2. Run both controllers in parallel (feature flag)
3. Migrate users gradually
4. Remove old API after migration period

## Benefits

1. **Flexibility**: No StatefulSet limitations
2. **Extensibility**: Easy to add new components
3. **Control**: Fine-grained Pod management
4. **Scalability**: Better performance for large clusters
5. **Maintainability**: Clear separation of concerns
6. **Modern**: Uses latest controller-runtime patterns

## Risks and Mitigations

### Risk: Large refactoring scope
**Mitigation**: Implement incrementally, test thoroughly at each phase

### Risk: Breaking changes
**Mitigation**: Keep old API initially, provide migration path

### Risk: Pod management complexity
**Mitigation**: Follow proven patterns from tidb-operator v2

### Risk: StatefulSet-specific features
**Mitigation**: Implement equivalent features directly (stable network IDs, ordered creation, etc.)

