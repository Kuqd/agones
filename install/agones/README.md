# Agones

[Agones](https://agones.dev) is designed as a batteries-included, open-source, dedicated game server hosting and scaling project built on top of Kubernetes, with the flexibility you need to tailor it to the needs of your multiplayer game.

## Introduction

This chart install the Agones application and defines deployment on a [Kubernetes](http://kubernetes.io) cluster using the [Helm](https://helm.sh) package manager.

## Prerequisites

- Kubernetes 1.9+
- Role-based access controls (RBAC) activated
- MutatingAdmissionWebhook admission controller activated, see [recommendation](https://kubernetes.io/docs/admin/admission-controllers/#is-there-a-recommended-set-of-admission-controllers-to-use)

## Installing the Chart

To install the chart with the release name `my-release`:

```bash
$ helm install --name my-release agones
```

The command deploys Agones on the Kubernetes cluster with the default configuration. The [configuration](#configuration) section lists the parameters that can be configured during installation.


> **Tip**: List all releases using `helm list`

## Uninstalling the Chart

To uninstall/delete the `my-release` deployment:

```bash
$ helm delete my-release
```

The command removes all the Kubernetes components associated with the chart and deletes the release.

## Configuration

The following tables lists the configurable parameters of the MySQL chart and their default values.

| Parameter                            | Description                               | Default                                              |
| ------------------------------------ | ----------------------------------------- | ---------------------------------------------------- |
| `namespace`                          | Namespace to use for Agones               | `agones-system`                                      |
| `serviceaccount.controller`          | Service account name for the controller   | `agones-controller`                                  |
| `serviceaccount.sdk`                 | Service account name for the sdk          | `agones-sdk`                                         |
| `image.controller.repository`        | Image repository for the controller       | `gcr.io/agones-images/agones-controller`             |
| `image.controller.tag`               | Image tag for the controller              | `0.1`                                                |
| `image.controller.pullPolicy`        | Image pull policy for the controller      | `IfNotPresent`                                       |
| `image.sdk.repository`               | Image repository for the sdk              | `gcr.io/agones-images/agones-sdk`                    |
| `image.sdk.tag`                      | Image tag for the sdk                     | `0.1`                                                |
| `image.sdk.alwaysPull`               | Tells if the sdk image should always be pulled  | `false`                                        |
| `minPort`                            | Minimum port to use for dynamic port allocation | 7000 |
| `maxPort`                            | Maximum port to use for dynamic port allocation | 8000 |

Specify each parameter using the `--set key=value[,key=value]` argument to `helm install`. For example,

```bash
$ helm install --name my-release \
  --set namespace=mynamespace,minPort=1000,maxPort=5000 agones
```

The above command sets the namespace where Agones is deployed to `mynamespace`. Additionally Agones will use a dynamic port allocation range of 1000-5000.

Alternatively, a YAML file that specifies the values for the parameters can be provided while installing the chart. For example,

```bash
$ helm install --name my-release -f values.yaml agones
```

> **Tip**: You can use the default [values.yaml](values.yaml)
