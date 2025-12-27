# TiKV Operator V2 Refactoring Status

## Overview

This document tracks the progress of refactoring tikv-operator to follow tidb-operator v2's 3-layer architecture.

## Completed ✅

### 1. API Definitions (`pkg/apis/core/v1alpha1/`)
- ✅ `Cluster` CRD: Top-level cluster abstraction
- ✅ `PDGroup` and `PD` CRDs: PD component management
- ✅ `TiKVGroup` and `TiKV` CRDs: TiKV component management
- ✅ Common types: `ClusterReference`, `Overlay`, `Volume`, `GroupStatus`, `CommonStatus`, etc.

### 2. Controllers (controller-runtime based)
- ✅ **Cluster Controller** (`pkg/controllers/cluster/`)
  - Watches Cluster and all Group CRs
  - Aggregates status from groups
  - Updates cluster-level status

- ✅ **PDGroup Controller** (`pkg/controllers/pdgroup/`)
  - Manages PD replicas
  - Creates/deletes PD instances based on replicas
  - Updates group status

- ✅ **PD Controller** (`pkg/controllers/pd/`)
  - Manages Pods directly (no StatefulSet)
  - Creates/updates Services (headless)
  - Creates/updates ConfigMaps
  - Creates/updates PVCs
  - Syncs status from Pod

- ✅ **TiKVGroup Controller** (`pkg/controllers/tikvgroup/`)
  - Manages TiKV replicas
  - Creates/deletes TiKV instances
  - Handles offline marking for scale-in
  - Updates group status

- ✅ **TiKV Controller** (`pkg/controllers/tikv/`)
  - Manages Pods directly (no StatefulSet)
  - Creates/updates Services (headless)
  - Creates/updates ConfigMaps
  - Creates/updates PVCs
  - Syncs status from Pod

### 3. Controller Manager
- ✅ Main entry point (`cmd/tikv-controller-manager/main_v2.go`)
  - Uses controller-runtime Manager
  - Sets up all controllers
  - Field indexers for efficient queries
  - Health and readiness probes
  - Leader election support

### 4. Examples and Documentation
- ✅ Basic example YAMLs (`examples/v2/basic/`)
  - Cluster YAML
  - PDGroup YAML
  - TiKVGroup YAML
  - README with usage instructions

## In Progress 🔄

### 5. Migration of Existing Logic
- ⏳ **Upgrade Logic**: Rolling updates with revision tracking
- ⏳ **Advanced Scaling**: Selective Pod scaling, graceful offline
- ⏳ **PD API Integration**: Query PD API for member IDs, leader status
- ⏳ **TiKV Store Status**: Query PD API for store IDs and states
- ⏳ **Failover Logic**: Automatic failover for failed pods

## Pending ❌

### 6. Code Generation
- ❌ Deepcopy generation
- ❌ CRD manifests generation
- ❌ Client code generation

### 7. Testing
- ❌ Unit tests
- ❌ Integration tests
- ❌ E2E tests

### 8. Additional Features
- ❌ Finalizers for proper cleanup
- ❌ Conditions and events
- ❌ Webhook validation (if needed)
- ❌ Metrics and observability

## Key Features Implemented

### ✅ No StatefulSet Dependency
- Direct Pod creation/update/deletion
- Full control over Pod lifecycle
- Per-instance PVC management

### ✅ 3-Layer Architecture
- **Cluster**: Top-level namespace
- **Group**: Component replica management
- **Instance**: Individual Pod management

### ✅ Component-Level Controllers
- Separate controllers for each component type
- Clear separation of concerns
- Easier to extend

### ✅ Direct Pod Management
- Pods created directly (not via StatefulSet)
- ConfigMaps and PVCs per instance
- Services shared across instances

## Next Steps

1. **Add PD API Integration**
   - Query PD API to get member IDs
   - Update PD status with actual member info
   - Check leader status

2. **Add TiKV Store Status**
   - Query PD API to get store IDs
   - Update TiKV status with store states
   - Handle offline/online states

3. **Implement Upgrade Logic**
   - Revision tracking per group
   - Rolling updates
   - Version management

4. **Add Finalizers**
   - Proper cleanup on deletion
   - Graceful shutdown

5. **Generate Code**
   - Deepcopy methods
   - CRD manifests
   - Client code

6. **Add Tests**
   - Unit tests for controllers
   - Integration tests
   - E2E tests

## Architecture Comparison

### Old (v1)
```
TikvCluster (monolithic CRD)
  └── StatefulSets
      └── Pods
```

### New (v2)
```
Cluster
  ├── PDGroup
  │   ├── PD (instance 1) ──> Pod
  │   ├── PD (instance 2) ──> Pod
  │   └── PD (instance 3) ──> Pod
  └── TiKVGroup
      ├── TiKV (instance 1) ──> Pod
      ├── TiKV (instance 2) ──> Pod
      └── TiKV (instance 3) ──> Pod
```

## Files Structure

```
tikv-operator/
├── pkg/
│   ├── apis/
│   │   └── core/v1alpha1/
│   │       ├── cluster_types.go
│   │       ├── pd_types.go
│   │       ├── tikv_types.go
│   │       ├── common_types.go
│   │       └── register.go
│   └── controllers/
│       ├── cluster/
│       │   └── controller.go
│       ├── pdgroup/
│       │   └── controller.go
│       ├── pd/
│       │   └── controller.go
│       ├── tikvgroup/
│       │   └── controller.go
│       └── tikv/
│           └── controller.go
├── cmd/tikv-controller-manager/
│   ├── main_v2.go
│   └── app/
│       └── v2_app.go
└── examples/v2/basic/
    ├── cluster.yaml
    ├── pd-group.yaml
    ├── tikv-group.yaml
    └── README.md
```

## Usage

See `examples/v2/basic/README.md` for usage instructions.

