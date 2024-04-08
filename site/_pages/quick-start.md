---
permalink: /quick-start/
title: "Quick Start Guide"
classes: wide
---

## Installing the latest Release

These quick start instructions assume you have a Kubernetes cluster
available to you and `kubectl` is properly configured.

### Install Kueue

Install a compatible version of Kueue by executing this command:
```sh
kubectl apply --server-side -f https://github.com/kubernetes-sigs/kueue/releases/download/{{ site.kueue_version }}/manifests.yaml
```

Before continuing, ensure Kueue is ready by executing this command:
```sh
kubectl -n kueue-system wait --timeout=300s --for=condition=Available deployments --all
```

Finally, you need to create a default `ClusterQueue` and `LocalQueue`
with a quota of 4 CPUs to enable Kueue to schedule workloads on your cluster.
The yaml shown below accomplishes this:
```yaml
apiVersion: kueue.x-k8s.io/v1beta1
kind: ResourceFlavor
metadata:
  name: "default-flavor"
---
apiVersion: kueue.x-k8s.io/v1beta1
kind: ClusterQueue
metadata:
  name: "cluster-queue"
spec:
  namespaceSelector: {} # match all.
  resourceGroups:
  - coveredResources: ["cpu"]
    flavors:
    - name: "default-flavor"
      resources:
      - name: "cpu"
        nominalQuota: 4
---
apiVersion: kueue.x-k8s.io/v1beta1
kind: LocalQueue
metadata:
  namespace: "default"
  name: "user-queue"
spec:
  clusterQueue: "cluster-queue"
```

You can either copy this yaml to your local file system and do a `kubectl apply -f <name-of-local-file>`
or apply it remotely by doing
```sh
kubectl apply -f kubectl apply -f https://raw.githubusercontent.com/project-codeflare/appwrapper/main/hack/default-queues.yaml
```

### Install AppWrapper

Install the most recent AppWrapper release by doing:
```sh
kubectl apply --server-side -f https://github.com/project-codeflare/appwrapper/releases/download/{{ site.appwrapper_version }}/install.yaml
```

Before continuing, ensure AppWrappers are ready by executing this command:
```sh
kubectl -n appwrapper-system wait --timeout=300s --for=condition=Available deployments --all
```

### Validate the Install

Finally, validate the installation by creating a simple AppWrapper and verifying that it runs
as expected.

Create an AppWrapper by executing
```sh
kubectl apply -f https://raw.githubusercontent.com/project-codeflare/appwrapper/{{ site.appwrapper_version }}/samples/wrapped-pod.yaml
```

You should quickly see an AppWrapper with the `Running` Status.
The sample contains a single Pod with an `init` container that runs for 10 seconds,
followed by a main container that runs for 5 seconds. After the main container completes,
the Status of the AppWrapper will be `Succeeded`. We show some kubectl commands and
their expected outputs below:
```sh
% kubectl get appwrappers
NAME         STATUS
sample-pod   Running

% kubectl get pods
NAME         READY   STATUS     RESTARTS   AGE
sample-pod   0/1     Init:0/1   0          14s

% kubectl get pods
NAME         READY   STATUS    RESTARTS   AGE
sample-pod   1/1     Running   0          18s

% kubectl get pods
NAME         READY   STATUS      RESTARTS   AGE
sample-pod   0/1     Completed   0          30s

% kubectl get appwrappers
NAME         STATUS
sample-pod   Succeeded
```

You can delete the AppWrapper with:
```sh
kubectl delete -f https://raw.githubusercontent.com/project-codeflare/appwrapper/{{ site.appwrapper_version }}/samples/wrapped-pod.yaml
```
