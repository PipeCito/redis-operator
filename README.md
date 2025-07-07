# Redis Cluster Operator

This project is a Kubernetes Operator to deploy and manage highly available, stateful Redis clusters. The Operator is built using the Operator SDK and manages Redis instances through a Custom Resource Definition (CRD).

It automates the creation of a true Redis cluster, complete with sharding for scalability and master-slave replication for high availability and fault tolerance.

## Features

- **Automated Redis Cluster Deployment**: Creates a multi-node Redis cluster, not just standalone instances.
- **High Availability**: Automatically configures master-slave replication based on the specified size. For a cluster of size N, it creates N/2 masters and N/2 replicas.
- **Data Sharding**: Distributes the key space across master nodes for write scalability.
- **Stateful Persistence**: Uses StatefulSets and PersistentVolumeClaims to ensure stable network identities and persistent storage for each Redis node.
- **Automated Cluster Initialization**: Handles the redis-cli --cluster create process automatically.
- **Secure by Default**: Generates a random, strong password for the cluster and stores it in a Kubernetes Secret.
- **Monitoring Ready**: Pods are annotated for Prometheus scraping, and a redis-exporter sidecar is included in each pod, making it fully compatible with Prometheus and Grafana.

## Prerequisites

- A running Kubernetes cluster (v1.25+). [Kind](https://kind.sigs.k8s.io/) is recommended for local development.
- kubectl command-line tool.
- make command-line tool.
- A container registry (e.g., Docker Hub, GCR, Quay.io) to store the Operator image.
- [Helm](https://helm.sh/docs/intro/install/).

## Deploy monitoring stack (Grafana + Prometheus)

If you don't have a monitoring stack, you can install one using Helm:

```bash
# Add the prometheus-community Helm repository
helm repo add prometheus-community https://prometheus-community.github.io/helm-charts
helm repo update

# Install the kube-prometheus-stack chart into a 'monitoring' namespace
helm install prometheus prometheus-community/kube-prometheus-stack --namespace monitoring --create-namespace
```

This will deploy Prometheus, Grafana, Alertmanager, and various exporters.

## Deploying with Helm (Recommended)

You can deploy the entire stack (Operator, CRD, Redis cluster, ServiceMonitors, and all required RBAC) using the provided Helm chart. This is the recommended and simplest way to get started.

1. **Go to the Helm chart directory:**
   
   ```bash
   cd charts/redis-operator
   ```

2. **Edit values.yaml if you want to customize the Redis cluster size, resources, storage, or image.**

   The Helm chart provides two deployment modes:
   
   - **Operator + Redis Cluster**: When `redis.enabled` is set to `true` in values.yaml, the Helm chart will deploy both the operator and a sample Redis cluster automatically.
   - **Operator Only (Default)**: When `redis.enabled` is set to `false`, only the operator will be deployed. You'll need to manually create Redis clusters using custom resource manifests.

3. **Install everything with Helm:**
   
   ```bash
   helm install redis-operator .
   ```

   **If `redis.enabled: true`**, this will:
   - Create the client-yazio-techtest-system namespace for the operator and all its resources.
   - Deploy the Redis Operator, CRD, RBAC, and ServiceAccount.
   - Deploy a sample Redis cluster (customizable via values.yaml).
   - Deploy ServiceMonitors for both the operator and Redis, so Prometheus can scrape metrics automatically.

   **If `redis.enabled: false` (default)**, this will:
   - Create the client-yazio-techtest-system namespace for the operator.
   - Deploy only the Redis Operator, CRD, RBAC, and ServiceAccount.
   - You'll need to manually create Redis clusters by applying custom resource manifests (see the "Creating a Redis Cluster Manually" section below).

4. **Verify the installation:**
   - Check the operator pod:
     
     ```bash
     kubectl get pods -n client-yazio-techtest-system
     ```

   - If Redis was enabled, check the Redis cluster:
     
     ```bash
     kubectl get redis
     kubectl get pods -l app.kubernetes.io/name=redis-cluster
     ```

   - Check ServiceMonitors:
     
     ```bash
     kubectl get servicemonitor -A
     ```

5. **Get the Redis cluster password (if Redis was enabled):**
   
   ```bash
   kubectl get secret redis-sample-password -o jsonpath="{.data.password}" | base64 --decode && echo
   ```

> **Note:**
> - The ServiceMonitor for Redis is deployed in the monitoring namespace (where Prometheus expects it).
> - You must have the Prometheus Operator (kube-prometheus-stack) installed for monitoring to work out of the box.

### Creating a Redis Cluster Manually (When redis.enabled: false)

If you deployed the operator with `redis.enabled: false`, or if you want to create additional Redis clusters, you can do so by applying custom resource manifests:

1. **Create a Redis cluster manifest** (e.g., my-redis-cluster.yaml):

   ```yaml
   apiVersion: cache.com.yazio/v1
   kind: Redis
   metadata:
     name: redis-sample
     namespace: default
   spec:
     size: 6
     
     maxMemory: "512mb"

     image:
       repository: "bitnami/redis"
       tag: "latest"

     resources:
       requests:
         cpu: "250m"
         memory: "512Mi"
       limits:
         cpu: "1000m"
         memory: "1Gi"

     storage:
       size: "1Gi"
       storageClassName: "standard"

     affinity:
       podAntiAffinity:
         preferredDuringSchedulingIgnoredDuringExecution:
         - weight: 100
           podAffinityTerm:
             labelSelector:
               matchExpressions:
               - key: app.kubernetes.io/instance
                 operator: In
                 values:
                 - redis-sample
             topologyKey: "kubernetes.io/hostname"
     tolerations:
     - key: "workload"
       operator: "Equal"
       value: "database"
       effect: "NoSchedule"
   ```

2. **Apply the manifest:**
   
   ```bash
   kubectl apply -f my-redis-cluster.yaml
   ```

3. **Monitor the deployment:**
   
   ```bash
   watch kubectl get pods -l app.kubernetes.io/instance=redis-sample
   ```

This approach gives you full control over when and how Redis clusters are created, allowing you to deploy multiple clusters with different configurations as needed.

## Deploy with bash Script

The deployment process involves building the Operator image, pushing it to a registry, and applying the necessary Kubernetes manifests. A convenience script is provided to streamline this.

The deploy_sample.sh script automates the build, push, and deployment steps.

```bash
# Usage: ./deploy_sample.sh <YOUR_CONTAINER_REGISTRY_USERNAME>
# Example for Docker Hub:
./deploy_sample.sh your-dockerhub-username
```

This script will:
- Build the Operator container image.
- Push the image to docker.io/<your-username>/redis-operator:latest.
- Deploy the Operator's CRD, RBAC rules, and Controller Deployment to the cluster.

## Deploy manually

If you prefer to deploy manually, follow these steps:

1. **Set Image Variable:**
   ```bash
   # Replace with your registry and image name
   export IMG="docker.io/your-username/redis-operator:v0.0.1"
   ```

2. **Build and Push the Image:**
   ```bash
   make docker-build docker-push IMG=$IMG
   ```

3. **Deploy to Kubernetes:**
   This command applies all necessary CRDs, RBAC configurations, and the controller manager deployment.
   ```bash
   make deploy IMG=$IMG
   ```

After deployment, verify that the Operator pod is running in the client-yazio-techtest-system namespace:
```bash
watch kubectl get pods -n client-yazio-techtest-system
```

Wait until it is 1/1 ready.

### Creating a Redis Cluster

With the Operator running, you can now create a Redis cluster by defining a Redis custom resource.

**Define the Custom Resource:**

Create a YAML file (e.g., my-redis-cluster.yaml) with the following content. For a highly available cluster, a size of 6 is recommended, which will create 3 masters and 3 replicas.

```yaml
apiVersion: cache.com.yazio/v1
kind: Redis
metadata:
  name: redis-sample
  namespace: default
spec:
  size: 6
  
  maxMemory: "512mb"

  image:
    repository: "bitnami/redis"
    tag: "latest"

  resources:
    requests:
      cpu: "250m"
      memory: "512Mi"
    limits:
      cpu: "1000m"
      memory: "1Gi"

  storage:
    size: "1Gi"
    storageClassName: "standard"

  affinity:
    podAntiAffinity:
      preferredDuringSchedulingIgnoredDuringExecution:
      - weight: 100
        podAffinityTerm:
          labelSelector:
            matchExpressions:
            - key: app.kubernetes.io/instance
              operator: In
              values:
              - redis-sample
          topologyKey: "kubernetes.io/hostname"
  tolerations:
  - key: "workload"
    operator: "Equal"
    value: "database"
    effect: "NoSchedule"
```

**Apply the Manifest:**
```bash
kubectl apply -f my-redis-cluster.yaml
```

## Verifying the Redis Cluster

The Operator will now create all the necessary Kubernetes resources.

### Check the StatefulSet and Pods:

A StatefulSet ensures that all pods are created sequentially and have stable identities.

```bash
# Check the StatefulSet status
kubectl get statefulset redis-sample

# Check the individual pods (all should be 2/2 and Running)
watch kubectl get pods -l app.kubernetes.io/instance=redis-sample
```

Wait until the 6 (or the specified size) pods are up.

### Check the Services:

Two services are created: one headless for internal pod discovery and one standard ClusterIP service for client access.

```bash
kubectl get svc -l app.kubernetes.io/instance=redis-sample
```

### Apply service monitor

This deploys serviceMonitor resources so the redis cluster can be monitored:

```bash
kubectl apply -k config/prometheus/
```

### Retrieve the Cluster Password:

The password is automatically generated and stored in a Secret.

```bash
kubectl get secret redis-sample-password -o jsonpath="{.data.password}" | base64 --decode && echo
```

## Interacting with the Cluster

To verify the cluster topology and test its functionality, you can connect using redis-cli from within one of the pods.

### Get a Shell Inside a Pod:
```bash
kubectl exec -it redis-sample-0 -- /bin/bash
```

### Check Cluster Nodes:

From inside the pod, use the CLUSTER NODES command. The REDIS_PASSWORD environment variable is already available inside the container.

```bash
# Inside the pod
redis-cli -a "$REDIS_PASSWORD" CLUSTER NODES
```

This will output a list of all nodes, their roles (master/slave), and their connection status.

### Test Sharding:

Connect in cluster mode (-c) to have redis-cli automatically follow redirects.

```bash
# Inside the pod
redis-cli -c -a "$REDIS_PASSWORD"

# Set keys and observe the redirections to different master nodes
127.0.0.1:6379> SET mykey "Hello, Cluster!"
-> Redirected to slot [6257] at <ip-of-another-node>:6379
OK
```

## Monitoring with Prometheus and Grafana

The Redis pods are instrumented with the Redis Exporter and are ready to be scraped by Prometheus. The recommended way to set up monitoring is by using the kube-prometheus-stack Helm chart.

### 1. Access Grafana

**Expose Grafana via Port-Forwarding:**
```bash
kubectl port-forward svc/prometheus-grafana -n monitoring 3000:80
```

**Log In:**
Open your browser to http://localhost:3000. The default credentials are:
- Username: admin
- Password: prom-operator (You can retrieve it with 
```kubectl get secret prometheus-grafana -n monitoring -o jsonpath="{.data.admin-password}" | base64 --decode && echo```
)

### 2. Import the Redis Exporter Dashboard

1. In the Grafana UI, go to Dashboards -> Import.
2. Use the Grafana.com ID for the official Redis Exporter dashboard: 763
3. Click Load.
4. On the next screen, select your Prometheus data source from the dropdown.
5. Click Import.

You should now see a detailed dashboard with metrics from all the Redis nodes in your cluster.

If you want to visualize more information about the redis cluster, the dashboard "Redis Cluster" with code 21914 is good.

## Stress Testing the Cluster

You can perform a benchmark test from your local machine to measure the performance of the cluster.

### 1. Install Redis Tools (Ubuntu/Debian)
```bash
sudo apt-get update
sudo apt-get install redis-tools
```

### 2. Expose the Cluster and Run Benchmark

**Port-Forward the Client Service:**
Open a terminal and run the following command. This will expose the Redis cluster's client entry point to your local machine on port 6379.

```bash
kubectl port-forward svc/redis-sample 6379:6379
```

**Get the Cluster Password:**
Open another terminal to retrieve the password.

```bash
export REDIS_PASSWORD=$(kubectl get secret redis-sample-password -o jsonpath="{.data.password}" | base64 --decode)
```

**Run the Benchmark:**
Use redis-benchmark with the -c flag for cluster mode. This example runs 100,000 SET/GET operations with 50 parallel clients.

```bash
redis-benchmark -h 127.0.0.1 -p 6379 -a "$REDIS_PASSWORD" -c 50 -n 100000
```

The output will show you the requests per second, latency distribution, and other key performance indicators for your cluster.

## Testing High Availability

You can simulate a master node failure to see the automatic failover in action.

### Identify a Master Pod:
Use the output of CLUSTER NODES to find a master (e.g., redis-sample-1).

### Delete the Master Pod:
```bash
kubectl delete pod redis-sample-1
```

### Observe the Failover:
- The StatefulSet will immediately start recreating redis-sample-1.
- In the meantime, the Redis cluster will detect the failure and promote the corresponding replica to become the new master.

### Verify the New Topology:
Once the deleted pod is running again, connect to the cluster and run CLUSTER NODES. You will see that the roles have changed, and the newly created pod has joined as a replica to the newly promoted master. Data will have been preserved.

## Cleanup

### If you used helm

If you used helm to install the operator, you need to uninstall the helm release

```bash
helm uninstall redis-operator
```

Delete PVCs:

```bash
kubectl delete pvc -l app.kubernetes.io/instance=redis-sample
```

### If you used bash script:

If you used the automated deployment script, you can destroy it using the automated destroy script:

```bash
./destroy_sample.sh
```

This script will delete the sample Redis custom resource and then uninstall the Operator components.

### Manual Cleanup Steps:

1. **Delete the Redis Cluster Instance:**
   ```bash
   kubectl delete redis redis-sample
   ```

2. **Delete the Redis Cluster PVCs:**
   ```bash
   kubectl delete pvc -l app.kubernetes.io/instance=redis-sample
   ```

3. **Delete Service Monitor:**
   ```bash
   kubectl delete -k config/prometheus/
   ```

4. **Uninstall the Operator:**
   ```bash
   make undeploy
   ```

5. **Delete CRDs:**
   ```bash
   make uninstall
   ```