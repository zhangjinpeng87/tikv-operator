# Getting started

This document explains how to create a simple Kubernetes cluster and use it to do a basic test deployment of TiKV Cluster using TiKV Operator.

<!-- toc -->
- [Step 1: Create a Kubernetes cluster](#step-1-create-a-kubernetes-cluster)
- [Step 2: Deploy TiKV Operator](#step-2-deploy-tikv-operator)
- [Step 3: Deploy TiKV Cluster](#step-3-deploy-tikv-cluster)
- [Step 4: Access the PD endpoint](#step-4-access-the-pd-endpoint)
<!-- /toc -->

## Step 1: Create a Kubernetes cluster

If you have already created a Kubernetes cluster, skip to [Step 2: Deploy TiKV Operator](#step-2-deploy-tikv-operator).

This section covers 2 different ways to create a simple Kubernetes cluster that
can be used to test TiKV Cluster locally. Choose whichever best matches your
environment or experience level.

- Using [kind](https://kind.sigs.k8s.io/docs/user/quick-start/) (Kubernetes in Docker)
- Using [minikube](https://kubernetes.io/docs/tasks/tools/install-minikube/) (Kubernetes running locally in a VM)

You can refer to their official documents to prepare a Kubernetes cluster.

The following shows a simple way to create a Kubernetes cluster using kind. Make sure Docker is up and running before proceeding.

On macOS / Linux:

```shell
curl -Lo ./kind https://kind.sigs.k8s.io/dl/v0.8.1/kind-$(uname)-amd64
chmod +x ./kind
./kind create cluster
```

On Windows:

```shell
curl.exe -Lo kind-windows-amd64.exe https://kind.sigs.k8s.io/dl/v0.8.1/kind-windows-amd64
.\kind-windows-amd64.exe create cluster
```

## Step 2: Deploy TiKV Operator

Before deployment, make sure the following requirements are satisfied:

- A running Kubernetes Cluster that kubectl can connect to
- Helm 3

1. Install helm

    ```shell
    curl https://raw.githubusercontent.com/helm/helm/master/scripts/get-helm-3 | bash
    ```

    Refer to [Helm Documentation](https://helm.sh/docs/intro/install/) for more installation alternatives.

2. Install CRD

    ```shell
    kubectl apply -f manifests/crd/
    ```

3. Install tikv-operator (using the local chart from this repository)

    If you already cloned this repository, you can install directly from the included Helm chart.
    If not, clone it first:

    ```shell
    git clone https://github.com/zhangjinpeng87/tikv-operator.git
    cd tikv-operator
    ```

    1. Create a namespace for TiKV Operator:

        ```shell
        kubectl create ns tikv-operator-system
        ```
        
        This creates a dedicated Kubernetes namespace called `tikv-operator-system` where the operator will be deployed.
        Namespaces provide isolation and organization for Kubernetes resources.
        
        **Note**: You can use any namespace name you prefer. We use `tikv-operator-system` here to clearly 
        distinguish it from other parameters in the installation command.

    2. Install TiKV Operator using the local Helm chart:

        ```shell
        helm install --namespace tikv-operator-system tikv-operator ./charts/tikv-operator
        ```
        
        This command installs the TiKV Operator. Breaking it down:
        - `helm install`: Helm command to install a chart
        - `--namespace tikv-operator-system`: Specifies the target namespace (where resources will be created)
        - `tikv-operator`: The release name (how you'll reference this Helm installation)
        - `./charts/tikv-operator`: The local chart path in this repository

    3. Confirm that the TiKV Operator components are running:

        ```shell
        kubectl --namespace tikv-operator-system get pods
        ```
        
        This command lists all pods in the `tikv-operator-system` namespace. You should see the 
        `tikv-controller-manager` pod running. Wait until it shows `STATUS: Running` and 
        `READY: 1/1` before proceeding to the next step.

## Step 3: Deploy TiKV Cluster

1. Deploy the TiKV Cluster using the v2 API:

    First, create the Cluster:
    ```shell
    curl -LO https://raw.githubusercontent.com/zhangjinpeng87/tikv-operator/master/examples/v2/basic/cluster.yaml
    kubectl apply -f cluster.yaml
    ```

    Then, create the PDGroup:
    ```shell
    curl -LO https://raw.githubusercontent.com/zhangjinpeng87/tikv-operator/master/examples/v2/basic/pd-group.yaml
    kubectl apply -f pd-group.yaml
    ```

    Finally, create the TiKVGroup:
    ```shell
    curl -LO https://raw.githubusercontent.com/zhangjinpeng87/tikv-operator/master/examples/v2/basic/tikv-group.yaml
    kubectl apply -f tikv-group.yaml
    ```

    Expected output:

    ```
    cluster.core.tikv.org/basic created
    pdgroup.core.tikv.org/pd created
    tikvgroup.core.tikv.org/tikv created
    ```

    **Alternative**: You can also clone the repository and apply all files at once:

    ```shell
    git clone https://github.com/zhangjinpeng87/tikv-operator.git
    cd tikv-operator/examples/v2/basic
    kubectl apply -f .
    ```

2. Wait for the cluster to be ready:

    ```shell
    kubectl wait --for=condition=Available --timeout 10m cluster/basic
    ```

    It may take several minutes as it needs to pull images from Docker Hub.

3. Check the progress with the following commands:

    ```shell
    # Check cluster status
    kubectl get cluster basic
    
    # Check PDGroup and PD instances
    kubectl get pdgroup pd
    kubectl get pd
    
    # Check TiKVGroup and TiKV instances
    kubectl get tikvgroup tikv
    kubectl get tikv
    
    # Check all pods
    kubectl get pods -o wide
    ```

## Step 4: Access the PD endpoint

Open a new terminal tab and run this command:

```shell
kubectl port-forward svc/basic-pd 2379:2379
```

This will forward local port `2379` to PD service `basic-pd`.

Now, you can access the PD endpoint with `pd-ctl` or any other PD client:

```shell
$ pd-ctl cluster
{
  "id": 6841476120821315702,
  "max_peer_count": 3
}
```
