#!/bin/bash

set -euo pipefail

DOCKER_USER=${1:?Usage: $0 <your-dockerhub-username>}
IMAGE_TAG="v0.1.19"
OPERATOR_NAMESPACE="client-yazio-techtest-system"
SAMPLE_NAMESPACE="default"
SAMPLE_NAME="redis-sample"

IMG="docker.io/${DOCKER_USER}/redis-operator:${IMAGE_TAG}"
SECRET_NAME="${SAMPLE_NAME}-password"

info() {
    echo "[INFO] $1"
}

info "Starting full deployment of the Redis Operator..."

info "Step 1: Installing CRDs and base RBAC in the cluster..."
make install

info "Step 2: Building and pushing the Operator image: ${IMG}"
make docker-build docker-push IMG="${IMG}"

info "Step 3: Deploying the Operator controller in the '${OPERATOR_NAMESPACE}' namespace..."
make deploy IMG="${IMG}"

info "Step 4: Deploying monitoring resources (Service, ServiceMonitors)..."
kubectl apply -k config/prometheus/

info "Step 5: Creating a sample Redis instance ('${SAMPLE_NAME}') in the '${SAMPLE_NAMESPACE}' namespace..."
kubectl apply -f config/samples/cache_v1_redis.yaml -n "${SAMPLE_NAMESPACE}"

info "Step 6: Waiting for StatefulSet '${SAMPLE_NAME}' to become available..."
if ! kubectl wait sts "${SAMPLE_NAME}" --for=condition=Available --timeout=120s -n "${SAMPLE_NAMESPACE}"; then
    echo "[ERROR] Redis Deployment did not become available in time. Checking logs..."
    kubectl logs "sts/${SAMPLE_NAME}" -n "${SAMPLE_NAMESPACE}" --all-containers
    exit 1
fi

info "Step 7: Verifying StatefulSet and Secret status..."
kubectl get sts "${SAMPLE_NAME}" -n "${SAMPLE_NAMESPACE}"
kubectl get secret "${SECRET_NAME}" -n "${SAMPLE_NAMESPACE}"

info "Step 8: Retrieving the generated Redis password:"
PASSWORD=$(kubectl get secret "${SECRET_NAME}" -n "${SAMPLE_NAMESPACE}" -o jsonpath="{.data.password}" | base64 --decode)
echo "Redis Password: ${PASSWORD}"

info "Deployment completed successfully."