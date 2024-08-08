# kuberbac
Kubernetes operator to make RBAC in Kubernetes great again



## Description
Kuberbac is a Kubernetes operator that makes it easy to manage detailed RBAC policies. 
It lets you handle dynamic roles and role-bindings



## Motivation
Kubernetes has RBAC implemented on its core. It can natively manage roles, 
subjects (groups, users or service-accounts) and even a way to link both them, the role-bindings.

This mechanism is easy to use and works well in general, but it has several issues at scale:

1. **The rules on its roles are just additive**, so there is not way to express something like 
   _"include permissions for all the resource types, but not for this one, or this other"_

   In this kind of cases, the solution is simply creating a huge policy including everything you need, 
   without those little parts you don't need. 
  
   The problem with this approach is that Kubernetes is dynamic, so new resource types can be included over the time. 
   This causes potential drifts between desired and actual state.


2. **There is no easy way to give a subject permissions on namespaces (or the cluster) dynamically**: a cluster 
   operator can grant a subject permissions either the whole cluster or just into a single namespace with ease. 
  
   Yes, it can grant permissions on several namespaces, but only by creating several bindings manually, one for each of them.


It would be natural to think that this issues can be managed with some scripts into a pipeline, may be a bit of Helm, etc.
But there are a lot of situations where this is not enough, or it gets super complicated.

Simpler is always better, so we created this operator to make our life easier, and now yours too.



## RBAC

**At Prosimcorp we always design our tools with a least privileges policy in mind.** This means that ours tools 
have only the minimal permissions they need. As a core policy, we commonly delegate extra permissions to the user, 
and explain the path to do it in the documentation.

Kuberbac is solving a core issue in Kubernetes and needs some extra permissions from the beginning.
As a transparency action, we document them all here.

Kuberbac is composed by two controllers, one for managing ClusterRoles and other to manage RoleBindings.
Permissions needed by both them are explained as follows:

* DynamicClusterRole controller is able to:
  * Perform any action over _ClusterRole_ and _DynamicClusterRole_ resources.

  * Get / List all the resources in the cluster.

    This is required as we calculate an additive policy for Kubernetes based on the difference 
    between allow/deny rules expressed by the user.

* DynamicRoleBinding controller is able to:

  * Perform any action over _RoleBinding_ and _DynamicRoleBinding_ resources.

  * Get / List _Namespace_ and _ServiceAccount_ resources in the cluster.

    This is required as we select those resource types based on the labels or regular-expressions given by the user


## Deployment

We have designed the deployment of this project to allow remote deployment using Kustomize. This way it is possible
to use it with a GitOps approach, using tools such as ArgoCD or FluxCD. Just make a Kustomization manifest referencing
the tag of the version you want to deploy as follows:

```yaml
apiVersion: kustomize.config.k8s.io/v1beta1
kind: Kustomization
resources:
- https://github.com/prosimcorp/kuberbac/releases/download/v0.1.0/bundle.yaml
```

> 🧚🏼 **Hey, listen! If you prefer to deploy using Helm, go to the [Helm registry](https://github.com/prosimcorp/helm-charts)**



## Examples

After deploying this operator, you will have two new custom resources available: `DynamicClusterRole` and `DynamicRoleBinding`.
Both them will be explained in the following sections.

### How to create kubernetes dynamic roles

To create a dynamic role in your cluster, you will need to create a `DynamicClusterRole` resource.
You may prefer to learn directly from an example, so let's explain it creating a DynamicClusterRole:

> You can find the spec samples for all the versions of the resource in the [examples directory](./config/samples)

```yaml
apiVersion: kuberbac.prosimcorp.com/v1alpha1
kind: DynamicClusterRole
metadata:
  name: example-policy
spec:
  # Synchronization parameters
  synchronization:
    time: "30s"

  # Desired name for produced ClusterRole
  target:
    name: example-policy
    annotations: {}
    labels: {}

    # This flag create two separated ClusterRoles: 
    # one for cluster-wide resources and another for namespace-scoped resources
    separateScopes: false

  # This is where the allowed policies are expressed
  # Ref: https://kubernetes.io/docs/reference/access-authn-authz/rbac/
  allow:
    # Allow everything to remove permissions or resources later
    # Of course, you can be much more specific. This is just an example
    - apiGroups: [ "*" ]
      resources: [ "*" ]
      verbs: [ "*" ]

  # This is where the denied policies are expressed
  # Ref: https://kubernetes.io/docs/reference/access-authn-authz/rbac/
  deny:
    # Deny access to resources related to this wonderful RBAC
    # You can use typical wildcards. They will be expanded by Kuberbac
    - apiGroups:
        - "*"
      resources:
        - "dynamicclusterroles"
        - "dynamicrolebindings"
      verbs:
        - "*"

    # Deny access to resources related to core RBAC
    - apiGroups:
        - "*"
      resources:
        - "clusterroles"
        - "clusterrolebindings"
        - "serviceaccounts"
      verbs:
        - "*"

    # Deny access to secrets
    - apiGroups: [ "*" ]
      resources: [ "secrets" ]
      verbs: [ "*" ]

    # Deny access to the cluster's CA
    - apiGroups: [ "*" ]
      resources: [ "configmaps" ]
      verbs: [ "*" ]
      resourceNames: [ "kube-root-ca.crt"]

    # Avoid users deleting some resources
    # You can specify verbs, or even names!
    - apiGroups: [ "*" ]
      resources: [ "configmaps" ]
      verbs: [ "delete" ]
      resourceNames: 
      - "kubeadm-config"
      - "kube-proxy"
      - "kubelet-config"
      - "coredns"
      - "cluster-info"

```

### How to create kubernetes dynamic role-binding

Now that you created a role, you can:

* Attach some members to the role 
* Grant those permissions inside specific namespaces

You can do both things inside a `DynamicRoleBinding` as follows:


```yaml
apiVersion: kuberbac.prosimcorp.com/v1alpha1
kind: DynamicRoleBinding
metadata:
  name: example-role-binding
spec:

  synchronization:
      time: "10s"

  # This is the section to enrol members to your existing role
  source:
    clusterRole: example-policy

    subject:
      # Members can be of type User. These members only exists outside your cluster
      # so they can be ONLY matched by exact names

      # apiGroup: rbac.authorization.k8s.io
      # kind: User
      # nameSelector:
      #   matchList:
      #     - Famara
      #     - Yeray
      #     - Chaxiraxi
      #     - Beneharo
      #     - Itahisa


      # Members can be of type Group. This case is exact same as User members.
      # This is typically used on cloud providers, when their external IAM members are granted on Kubernetes.
      # They can be ONLY matched by exact names

      #apiGroup: rbac.authorization.k8s.io
      #kind: Group
      #nameSelector:
      #  matchList:
      #    - managers@company.com
      #    - upper-managers@company.com


      # ServiceAccount resources actually exists inside Kubernetes, so the operator can look for them.
      # Kuberbac will look for them by name and namespace, both at once, so you need to fill both selectors. 
      apiGroup: ""
      kind: ServiceAccount

      # (Optional)
      # ServiceAccounts can be selected by some metadata
      # This field is mutually exclusive with 'nameSelector'
      # Attention: Only one can be performed.
      metaSelector:
        
        # Select by matching labels
        matchLabels:
          managed-by: custom-operator

        # Select by matching annotations
        # matchAnnotations:
        #   managed-by: custom-operator

      # (Optional)
      # ServiceAccount names can be matched by exact name, or a Golang regular expression. 
      # This field is mutually exclusive with 'metaSelector'
      # Attention: Only one can be performed.
      nameSelector:

        # Select by matching exact names
        matchList:
          - default

        # Select by matching names using a regular expression.
        # As Golang does not support negative lookahead on regex, there is a special parameter 
        # called 'negative' to select the opossite names than expressed by the expression

        # matchRegex: 
        #   negative: false
        #   expression: "^(.*)$"

      # (Optional)
      # To look for a ServiceAccount, namespaces can be matched by exact name, 
      # by their labels, or a Golang regular expression. 
      # Attention: Only one can be performed.
      namespaceSelector:

        # Select namespaces by matching exact names
        matchList:
          - kube-system
          - kube-public
          - default

        # Select those ServiceAccounts in namespaces containing some labels
        # matchLabels:
        #   managed-by: hashicorp-vault

        # Select those ServiceAccounts in namespaces different from: kube-system, kube-public or default
        # matchRegex:
        #   negative: true
        #   expression: "^(default|kube-system|kube-public)$"


  # This is the section to define the target namespaces where the role-bindings will be created
  # For those members selected in the previous section
  targets:

    # (Required) 
    # Name of the RoleBinding objects to be created
    name: example-policy

    # Add some metadata to the RoleBinding objects
    annotations: {}
    labels: {}

    # This flag create a ClusterRoleBinding object instead of RoleBindings 
    clusterScoped: true

    # (Optional)
    # Target namespaces can be matched by exact name, 
    # by their labels, or a Golang regular expression. 
    # Attention: Only one can be performed.
    namespaceSelector:

      # Select namespaces by matching exact names
      matchList:
        - kube-system
        - kube-public
        - default

      # Select those ServiceAccounts in namespaces containing some labels
      # matchLabels:
      #   managed-by: hashicorp-vault

      # Select those ServiceAccounts in namespaces different from: kube-system, kube-public or default
      # matchRegex:
      #   negative: true
      #   expression: "^(default|kube-system|kube-public)$"
  
```


## How to develop

### Prerequisites
- Kubebuilder v4.0.0+
- Go version v1.22.0+
- Docker version 17.03+.
- kubectl version v1.11.3+.
- Access to a Kubernetes v1.11.3+ cluster.

### The process

> We recommend you to use a development tool like [Kind](https://kind.sigs.k8s.io/) or [Minikube](https://minikube.sigs.k8s.io/docs/start/)
> to launch a lightweight Kubernetes on your local machine for development purposes

For learning purposes, we will suppose you are going to use Kind. So the first step is to create a Kubernetes cluster
on your local machine executing the following command:

```console
kind create cluster
```

Once you have launched a safe play place, execute the following command. It will install the custom resource definitions
(CRDs) in the cluster configured in your ~/.kube/config file and run Kuberbac locally against the cluster:

```console
make install run
```

If you would like to test the operator against some resources, our examples can be applied to see the result in
your Kind cluster

```sh
kubectl apply -k config/samples/
```

> Remember that your `kubectl` is pointing to your Kind cluster. However, you should always review the context your
> kubectl CLI is pointing to



## How releases are created

Each release of this operator is done following several steps carefully in order not to break the things for anyone.
Reliability is important to us, so we automated all the process of launching a release. For a better understanding of
the process, the steps are described in the following recipe:

1. Test the changes on the code:

    ```console
    make test
    ```

   > A release is not done if this stage fails


2. Define the package information

    ```console
    export VERSION="0.0.1"
    export IMG="ghcr.io/prosimcorp/kuberbac:v$VERSION"
    ```

3. Generate and push the Docker image (published on Docker Hub).

    ```console
    make docker-build docker-push
    ```

4. Generate the manifests for deployments using Kustomize

   ```console
    make build-installer
    ```



## How to collaborate

This project is done on top of [Kubebuilder](https://github.com/kubernetes-sigs/kubebuilder), so read about that project 
before collaborating. Of course, we are open to external collaborations for this project. For doing it you must fork the 
repository, make your changes to the code and open a PR. The code will be reviewed and tested (always)

> We are developers and hate bad code. For that reason we ask you the highest quality on each line of code to improve
> this project on each iteration.



## License

Copyright 2022.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
