# Basic TiKV Cluster Example (v2)

This example demonstrates how to create a basic TiKV cluster using the new v2 API.

## Prerequisites

- Kubernetes cluster
- tikv-operator v2 installed

## Deploy the Cluster

1. Create the Cluster:
```bash
kubectl apply -f cluster.yaml
```

2. Create the PDGroup:
```bash
kubectl apply -f pd-group.yaml
```

3. Create the TiKVGroup:
```bash
kubectl apply -f tikv-group.yaml
```

## Check Status

```bash
# Check cluster status
kubectl get cluster basic

# Check PDGroup status
kubectl get pdgroup pd

# Check PD instances
kubectl get pd

# Check TiKVGroup status
kubectl get tikvgroup tikv

# Check TiKV instances
kubectl get tikv
```

## Architecture

This example creates:
- 1 Cluster (basic)
- 1 PDGroup with 3 PD instances
- 1 TiKVGroup with 3 TiKV instances

The operators will automatically:
- Create PD Pods
- Create TiKV Pods
- Create ConfigMaps for configuration
- Create PVCs for persistent storage
- Manage the lifecycle of all resources

