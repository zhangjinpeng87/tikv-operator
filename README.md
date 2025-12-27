# TiKV Operator

TiKV Operator is a Kubernetes operator for managing TiKV clusters. It provides a declarative way to deploy, scale, and manage TiKV clusters on Kubernetes.

## Status

TiKV Operator is currently being refactored to a new v2 architecture that follows a modern 3-layer CRD design pattern. The refactoring introduces:

- **3-Layer Architecture**: Cluster → Group → Instance hierarchy
- **Direct Pod Management**: No StatefulSet dependency for fine-grained control
- **Controller-Runtime Framework**: Modern Kubernetes operator patterns
- **Component-Level Controllers**: Separate controllers for PD and TiKV components

For detailed information about the refactoring, see:
- [Refactoring Plan](./REFACTORING_PLAN.md) - Overview of the refactoring goals and approach
- [Refactoring Status](./REFACTORING_STATUS.md) - Current implementation progress
- [Refactoring Summary](./REFACTORING_SUMMARY.md) - Architecture comparison and migration path

## Quick Start

See the [Getting Started Guide](./docs/getting-started.md) for detailed instructions on deploying TiKV Operator and creating your first cluster.

### Basic Example

Deploy a basic TiKV cluster using the v2 API:

```bash
# Create Cluster
kubectl apply -f examples/v2/basic/cluster.yaml

# Create PDGroup
kubectl apply -f examples/v2/basic/pd-group.yaml

# Create TiKVGroup
kubectl apply -f examples/v2/basic/tikv-group.yaml
```

Check the status:

```bash
kubectl get cluster basic
kubectl get pdgroup pd
kubectl get tikvgroup tikv
kubectl get pd
kubectl get tikv
```

## Architecture

### V2 Architecture (Current)

The v2 architecture uses a 3-layer CRD structure:

```
Cluster (top-level namespace)
├── PDGroup (manages PD replicas)
│   ├── PD (instance 1) ──> Pod
│   ├── PD (instance 2) ──> Pod
│   └── PD (instance 3) ──> Pod
└── TiKVGroup (manages TiKV replicas)
    ├── TiKV (instance 1) ──> Pod
    ├── TiKV (instance 2) ──> Pod
    └── TiKV (instance 3) ──> Pod
```

### Key Features

- **Direct Pod Management**: Pods are created and managed directly without StatefulSet
- **Fine-Grained Control**: Instance-level operations for scaling, upgrades, and maintenance
- **Component Separation**: Separate controllers for each component type
- **Modern Framework**: Built on controller-runtime for better testability and maintainability

## Installation

### Prerequisites

- Kubernetes cluster (1.20+)
- kubectl configured to access your cluster
- Helm 3

### Install CRDs

```bash
kubectl apply -f manifests/crd/
```

### Install Operator

```bash
# Create namespace
kubectl create ns tikv-operator-system

# Install using Helm
helm install --namespace tikv-operator-system tikv-operator ./charts/tikv-operator
```

## Documentation

- [Getting Started](./docs/getting-started.md) - Step-by-step guide to deploy your first cluster
- [Development Guide](./docs/development.md) - How to build and develop TiKV Operator
- [Examples](./examples/v2/basic/) - Example configurations

## License

TiKV Operator is under the Apache 2.0 license. See the [LICENSE](./LICENSE) file for details.
