# kubeception-operator

Kubeception is a Kubernetes Control Plane Operator.
It deploye a Kubernetes Control Plane into a Kubernetes cluster.
Allowing to leverage the Kubernetes Operator pattern to manage Kubernetes Control Plane components.

It is perfect for users that want to develop or implement a Kubernetes As A Service product in house !

The project is in it's early stages. It currently handle kube-apiserver, kube-controller-manager, kube-scheduler the pki. But doesn't handle ETCD cluster.
No roadmap is clear at the moment.

## Requirements

You should have a functionnal kubernetes cluster that provision Loadbalancer service type to be able to run this operator.
You'll need Certmanager installed as well. You can install it with the `./hack/install-cert-manager.sh` script. It will use your kubeconfig file to deploy.

You can follow the Getting started (local) to run the operator on a local cluster. But you will face difficulties registering workers on managed Control Planes.


## Getting started (already existing cluster)

**We asume you have a publicly available cluster and is able to provisione **Loadbalancer** service type.**

Install Cert Manager:
```sh
./hack/install-cert-manager.sh
```

Create demo namespace
```sh
kubectl create ns demo
```

Deploy ETCD cluster into demo namespace
```sh
kubectl apply -n demo -f ./hack/etcd
```

Install operator CRDs
```sh
make manifests install
```

Run operator
```sh
make run
```

Deploy demo Control plane
```sh
kubectl apply -f ./config/samples/cluster_v1alpha1_controlplane.yaml
```

### Export Admin Kubeconfig

To export kubernetes admin kubeconfig run:

```sh
$ kubectl get -n demo secret admin-kubeconfig -o json | jq '.data["kubeconfig.yml"]' -r | base64 -d > .kubeconfig-cluster

$ export KUBECONFIG=.kubeconfig-cluster

$ kubectl get no
No resources found
```

### Uninstall CRDs
To delete the CRDs from the cluster:

```sh
make uninstall
```

### Undeploy controller
UnDeploy the controller from the cluster:

```sh
make undeploy
```

### Worker deployment

Some installation script was created to simplify worker deployment.

1. Generate and deploy bootstrap token

To be able to provision worker you'll need a boostrap token. It can be generated and deployed using `./hack/deploy-token.sh` script
**Warning**: Be sure to run this script using the kubeconfig of the managed control plane.

This script deploye the bootstrap token and all RBAC to automaticaly approve CSR from Kubelet Workers

```sh
$ ./hack/deploy-plugins.sh
TOKEN is : hptl02.lb6wyaq5pkwmiza7
secret/bootstrap-token-hptl02 created
clusterrolebinding.rbac.authorization.k8s.io/create-csrs-for-bootstrapping created
clusterrolebinding.rbac.authorization.k8s.io/auto-approve-csrs-for-group created
clusterrolebinding.rbac.authorization.k8s.io/auto-approve-renewals-for-nodes created
```

More information here:
- [Authenticating with Bootstrap Tokens](https://kubernetes.io/docs/reference/access-authn-authz/bootstrap-tokens/)
- [TLS bootstrapping](https://kubernetes.io/docs/reference/access-authn-authz/kubelet-tls-bootstrapping/)


2. Install and Configure Worker

This script install and configure worker nodes. It enable required modules `overlay` and `br_netfilter`
then setup kernel parameters for **bridge filtering** and **ip forwarding**.
It Install cri-o and all required package to run kubelet.
Then it finaly setup kubelet service, enables and start it.

This script needs to be copied to the worker and run with `Control Plane IP`, `Token` and `Base64 cluster CA` args:

```sh
./hack/setup-worker.sh xxx.xxx.xxx.xxx hptl02.lb6wyaq5pkwmiza7 base64castring...
```

Once this script succeed you should see the node registred via `kubectl get no`


3. Install required plugins

**Warning**: Be sure to run this script using the kubeconfig of the managed control plane.

Kubernetes needs plugins like CoreDNS or kube-proxy.
This scripts install:
- Konnectivity Agent (for ControlPlane to Node communication)
- CoreDNS
- Cilium (this will replace kube-proxy)
- Metrics Server
- API Server RBAC (to allow log, proxy, exec commands to run)

It needs Controle Plane IP to run.

```sh
./hack/deploy-plugins.sh xxx.xxx.xxx.xxx
```

## Getting started (Local)

Here is the guide for local development. The operator is supposed to run on a cluster that provision **Loadbalancer** service type.
To be able to provision **Loadbalancer** service type, we will be installing MetalLB.
In local mode, worker deployment can be challenging. We recommend you use a publicly available cluster for this purpose.

Start a kind cluster:

```sh
kind create cluster
```

Install CertManager:
```sh
./hack/install-cert-manager.sh
```

Install MetalLB for local service loadbalancer provisionning:
```sh
# Get your kind network subnet
$ docker network inspect kind -f "{{(index .IPAM.Config 0).Subnet}}"
100.64.1.0/24

# Install MetalLB with a portion of kind network subnet
$ ./hack/install-metallb.sh 100.64.1.100-100.64.1.200
```

Create demo namespace
```sh
kubectl create ns demo
```

Deploy ETCD cluster into demo namespace
```sh
kubectl apply -n demo -f ./hack/etcd
```

Install operator CRDs
```sh
make manifests install
```

Run operator
```sh
make run
```

Deploy demo Control plane
```sh
kubectl apply -f ./config/samples/cluster_v1alpha1_controlplane.yaml
```

You should see each components created:
```sh
$ kubectl get all -n demo
NAME                                           READY   STATUS    RESTARTS      AGE
pod/etcd-0                                     1/1     Running   0             54s
pod/etcd-1                                     1/1     Running   1 (21s ago)   52s
pod/etcd-2                                     1/1     Running   0             50s
pod/kube-apiserver-7798959c48-jmvxb            2/2     Running   0             30s
pod/kube-apiserver-7798959c48-xg59z            2/2     Running   0             30s
pod/kube-apiserver-7798959c48-xmjmv            2/2     Running   0             30s
pod/kube-controller-manager-55d9b79557-2l68w   1/1     Running   1 (16s ago)   33s
pod/kube-controller-manager-55d9b79557-bdxqq   1/1     Running   0             33s
pod/kube-controller-manager-55d9b79557-h8xp5   1/1     Running   0             33s
pod/kube-scheduler-57ccd95f78-bpf5r            1/1     Running   0             30s
pod/kube-scheduler-57ccd95f78-nb9vh            1/1     Running   0             30s
pod/kube-scheduler-57ccd95f78-t89xw            1/1     Running   0             30s

NAME                     TYPE           CLUSTER-IP      EXTERNAL-IP    PORT(S)                                                       AGE
service/etcd             ClusterIP      None            <none>         2379/TCP,2380/TCP                                             54s
service/etcd-client      ClusterIP      10.96.183.120   <none>         2379/TCP                                                      54s
service/kube-apiserver   LoadBalancer   10.96.248.50    100.64.1.100   6443:31850/TCP,8132:30777/TCP,8133:31090/TCP,8134:31276/TCP   42s

NAME                                      READY   UP-TO-DATE   AVAILABLE   AGE
deployment.apps/kube-apiserver            3/3     3            3           30s
deployment.apps/kube-controller-manager   3/3     3            3           33s
deployment.apps/kube-scheduler            3/3     3            3           30s

NAME                                                 DESIRED   CURRENT   READY   AGE
replicaset.apps/kube-apiserver-7798959c48            3         3         3       30s
replicaset.apps/kube-controller-manager-55d9b79557   3         3         3       33s
replicaset.apps/kube-scheduler-57ccd95f78            3         3         3       30s

NAME                    READY   AGE
statefulset.apps/etcd   3/3     54s

$ kubectl get secrets -n demo
NAME                                 TYPE                DATA   AGE
admin                                kubernetes.io/tls   3      118s
admin-kubeconfig                     Opaque              1      118s
ca                                   kubernetes.io/tls   3      2m3s
konnectivity                         kubernetes.io/tls   3      114s
konnectivity-kubeconfig              Opaque              1      111s
kube-apiserver                       kubernetes.io/tls   3      116s
kube-controller-manager              kubernetes.io/tls   3      117s
kube-controller-manager-kubeconfig   Opaque              1      114s
kube-scheduler                       kubernetes.io/tls   3      113s
kube-scheduler-config                Opaque              2      111s
service-accounts                     kubernetes.io/tls   3      116s
```


## Contributing
Make a PR, document it and explain why it should be added to the projet.

### How it works
This project aims to follow the Kubernetes [Operator pattern](https://kubernetes.io/docs/concepts/extend-kubernetes/operator/).

It deploys each Control Plane components via CRD and mange it.
It currently doesn't support managing ETCD clusters.


### Modifying the API definitions
If you are editing the API definitions, generate the manifests such as CRs or CRDs using:

```sh
make manifests
```

**NOTE:** Run `make --help` for more information on all potential `make` targets

More information can be found via the [Kubebuilder Documentation](https://book.kubebuilder.io/introduction.html)

## License

Copyright 2023 Ulysse FONTAINE.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
