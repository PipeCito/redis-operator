#!/bin/bash

set -e

OPERATOR_NAMESPACE="client-yazio-techtest-system"
SAMPLE_NAMESPACE="default"
SAMPLE_NAME="redis-sample"

info() {
    echo "[INFO] $1"
}

info "Starting full cleanup of the Redis Operator..."

info "Step 1: Deleting the sample Redis instance for clean removal..."
kubectl delete -f config/samples/cache_v1_redis.yaml -n "${SAMPLE_NAMESPACE}" --ignore-not-found=true

info "Step 2: Deleting monitoring resources..."
kubectl delete -k config/prometheus/ --ignore-not-found=true

info "Step 3: Uninstalling the Operator and its CRDs from the cluster..."

if ! make undeploy; then
    echo "[WARNING] 'make undeploy' failed. Resources might have already been deleted."
fi

info "Step 4: Deleting PVCs..."
kubectl delete pvc -l app.kubernetes.io/instance=redis-sample

info "Cleanup completed."