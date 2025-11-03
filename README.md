## HPA Stats Poller

This project provides a simple CLI for periodically printing Horizontal Pod Autoscaler (HPA) status
information directly from the Kubernetes API. It supports both in-cluster execution and external
use with a provided kubeconfig.

### Prerequisites

- Go 1.24+
- Access to a Kubernetes cluster and appropriate RBAC permissions for reading HPAs and the
  referenced metrics.

### Installation

```bash
go install ./cmd/hpastats
```

### Usage

```bash
hpastats \
  -kubeconfig=$HOME/.kube/config \
  -namespace=default \
  -hpa=new-stress-server-hpa \
  -interval=1m
```

Flags:

- `-kubeconfig`: Path to kubeconfig. Leave empty inside a cluster.
- `-namespace`: Limit to a specific namespace (default: all namespaces).
- `-hpa`: Focus on a single HPA (requires `-namespace`).
- `-interval`: Polling interval (default: 30s).

### Running in a Cluster

When running inside a Kubernetes cluster, leave `-kubeconfig` empty and ensure the service account
has permissions for `get`, `list`, and `watch` on HPAs plus any metrics the HPAs depend on.

### Future Improvements

1. Emit structured metrics (e.g., Prometheus) in addition to console output.
2. Add filtering options for specific HPA names or label selectors.
3. Provide JSON output mode for integration with other systems.

## HPA Replica Alert Controller

The `hpalogger` manager watches `HPAReplicaAlert` custom resources and logs whenever the referenced
HPA's `status.currentReplicas` value changes.

### CRD Installation

```bash
kubectl apply -f config/crd/hpareplicaalerts.yaml
```

### Running Locally

```bash
go run ./cmd/hpalogger
```

### Deploying to the Cluster

1. Build and push an image containing the binary (example shown with ko):
   ```bash
   ko build ./cmd/hpalogger
   ```
2. Create a Deployment using that image and grant RBAC permissions to:
   - Read HPAs (`autoscaling` API group) in target namespaces.
   - Manage `hpareplicaalerts.ops.zvikanaparstek.dev` CRDs.

### Example Alert

```yaml
apiVersion: ops.zvikanaparstek.dev/v1alpha1
kind: HPAReplicaAlert
metadata:
  name: new-stress-server-alert
  namespace: default
spec:
  targetRef:
    apiVersion: autoscaling/v2
    kind: HorizontalPodAutoscaler
    name: new-stress-server-hpa
```

The controller updates `.status.lastObservedReplicas` and logs replica transitions for the target.

### Annotation-Based Alerts

The controller can create and retire alerts automatically based on annotations placed on HPAs:

- `ops.zvikanaparstek.dev/replica-alert: "enabled"` — enable alerts for the annotated HPA.
- `ops.zvikanaparstek.dev/managed-by: annotation` and `ops.zvikanaparstek.dev/target-hpa` are added automatically to generated `HPAReplicaAlert` objects.

Workflows:

1. Annotate an existing HPA:
   ```bash
   kubectl annotate hpa new-stress-server-hpa ops.zvikanaparstek.dev/replica-alert=enabled
   ```
2. The controller creates an `HPAReplicaAlert` named after the HPA.
3. Removing the annotation deletes the managed alert automatically.

### Helm Chart

Deploy the controller with Helm:

```bash
helm upgrade --install hpalogger deploy/charts/hpalogger \
  --namespace hpa-alert --create-namespace \
  --set image.repository=<your repo>/hpalogger \
  --set image.tag=<your tag>
```

Key values (`values.yaml`) include metrics/health bind addresses, leader election, and image settings. The chart also installs the CRD from `config/crd/hpareplicaalerts.yaml` automatically.

### Building & Publishing the Image

The repository includes a root `Dockerfile` for building the controller image:

```bash
# authenticate if needed (example for GHCR)
echo "$CR_PAT" | docker login ghcr.io -u <user> --password-stdin

# build the image (Buildx recommended for multi-arch)
docker build -t ghcr.io/<user>/hpalogger:v0.1.0 .

# push to your registry
docker push ghcr.io/<user>/hpalogger:v0.1.0
```

Use the resulting tag when upgrading the Helm release.
