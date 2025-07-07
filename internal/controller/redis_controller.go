/*
Copyright 2025.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package controller

import (
	"context"
	"crypto/rand"
	"fmt"
	"math/big"
	"strconv"
	"strings"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	cachev1 "github.com/yazio-com-co/client-yazio-techtest/api/v1"
)

type RedisReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=cache.com.yazio,resources=redis,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=cache.com.yazio,resources=redis/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=cache.com.yazio,resources=redis/finalizers,verbs=update
// +kubebuilder:rbac:groups=apps,resources=statefulsets,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=core,resources=secrets,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=core,resources=configmaps,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=core,resources=services,verbs=get;list;watch;create;update;patch;delete

func (r *RedisReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := logf.FromContext(ctx)

	redis := &cachev1.Redis{}
	err := r.Get(ctx, req.NamespacedName, redis)
	if err != nil {
		if errors.IsNotFound(err) {
			logger.Info("Redis resource not found. Ignoring as it was likely deleted.")
			return ctrl.Result{}, nil
		}
		logger.Error(err, "Failed to get Redis resource")
		return ctrl.Result{}, err
	}

	secretName := fmt.Sprintf("%s-password", redis.Name)
	foundSecret := &corev1.Secret{}
	err = r.Get(ctx, types.NamespacedName{Name: secretName, Namespace: redis.Namespace}, foundSecret)
	if err != nil && errors.IsNotFound(err) {
		logger.Info("Creating a new Secret", "Secret.Namespace", redis.Namespace, "Secret.Name", secretName)
		password, err := generateRandomPassword(16)
		if err != nil {
			logger.Error(err, "Failed to generate random password for Secret")
			return ctrl.Result{}, err
		}
		secret := r.secretForRedis(redis, secretName, password)
		if err := r.Create(ctx, secret); err != nil {
			logger.Error(err, "Failed to create new Secret", "Secret.Namespace", secret.Namespace, "Secret.Name", secret.Name)
			return ctrl.Result{}, err
		}
	} else if err != nil {
		logger.Error(err, "Failed to get Secret")
		return ctrl.Result{}, err
	}

	configMapName := fmt.Sprintf("%s-config", redis.Name)
	foundConfigMap := &corev1.ConfigMap{}
	err = r.Get(ctx, types.NamespacedName{Name: configMapName, Namespace: redis.Namespace}, foundConfigMap)
	if err != nil && errors.IsNotFound(err) {
		logger.Info("Creating a new ConfigMap", "ConfigMap.Namespace", redis.Namespace, "ConfigMap.Name", configMapName)
		cm := r.configMapForRedis(redis, configMapName)
		if err := r.Create(ctx, cm); err != nil {
			logger.Error(err, "Failed to create new ConfigMap", "ConfigMap.Namespace", cm.Namespace, "ConfigMap.Name", cm.Name)
			return ctrl.Result{}, err
		}
	} else if err != nil {
		logger.Error(err, "Failed to get ConfigMap")
		return ctrl.Result{}, err
	}

	headlessSvcName := fmt.Sprintf("%s-headless", redis.Name)
	foundHeadlessSvc := &corev1.Service{}
	err = r.Get(ctx, types.NamespacedName{Name: headlessSvcName, Namespace: redis.Namespace}, foundHeadlessSvc)
	if err != nil && errors.IsNotFound(err) {
		logger.Info("Creating a new Headless Service", "Service.Namespace", redis.Namespace, "Service.Name", headlessSvcName)
		headlessSvc := r.headlessServiceForRedis(redis, headlessSvcName)
		if err := r.Create(ctx, headlessSvc); err != nil {
			logger.Error(err, "Failed to create new Headless Service", "Service.Namespace", headlessSvc.Namespace, "Service.Name", headlessSvc.Name)
			return ctrl.Result{}, err
		}
	} else if err != nil {
		logger.Error(err, "Failed to get Headless Service")
		return ctrl.Result{}, err
	}

	foundStatefulSet := &appsv1.StatefulSet{}
	err = r.Get(ctx, types.NamespacedName{Name: redis.Name, Namespace: redis.Namespace}, foundStatefulSet)
	if err != nil && errors.IsNotFound(err) {
		logger.Info("Creating a new StatefulSet", "StatefulSet.Namespace", redis.Namespace, "StatefulSet.Name", redis.Name)
		sts := r.statefulSetForRedis(redis, secretName, configMapName, headlessSvcName)
		if err := r.Create(ctx, sts); err != nil {
			logger.Error(err, "Failed to create new StatefulSet", "StatefulSet.Namespace", sts.Namespace, "StatefulSet.Name", sts.Name)
			return ctrl.Result{}, err
		}
		return ctrl.Result{Requeue: true}, nil
	} else if err != nil {
		logger.Error(err, "Failed to get StatefulSet")
		return ctrl.Result{}, err
	}

	size := redis.Spec.Size
	if *foundStatefulSet.Spec.Replicas != size {
		logger.Info("Updating StatefulSet size", "Current", *foundStatefulSet.Spec.Replicas, "Desired", size)
		foundStatefulSet.Spec.Replicas = &size
		if err := r.Update(ctx, foundStatefulSet); err != nil {
			logger.Error(err, "Failed to update StatefulSet", "StatefulSet.Namespace", foundStatefulSet.Namespace, "StatefulSet.Name", foundStatefulSet.Name)
			return ctrl.Result{}, err
		}
		return ctrl.Result{Requeue: true}, nil
	}

	clientSvc := &corev1.Service{}
	err = r.Get(ctx, types.NamespacedName{Name: redis.Name, Namespace: redis.Namespace}, clientSvc)
	if err != nil && errors.IsNotFound(err) {
		logger.Info("Creating a new Client Service", "Service.Namespace", redis.Namespace, "Service.Name", redis.Name)
		svc := r.clientServiceForRedis(redis)
		if err := r.Create(ctx, svc); err != nil {
			logger.Error(err, "Failed to create new Client Service")
			return ctrl.Result{}, err
		}
	} else if err != nil {
		logger.Error(err, "Failed to get Client Service")
		return ctrl.Result{}, err
	}

	logger.Info("Reconciliation completed successfully.")
	return ctrl.Result{}, nil
}

func generateRandomPassword(length int) (string, error) {
	const charset = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	b := make([]byte, length)
	for i := range b {
		num, err := rand.Int(rand.Reader, big.NewInt(int64(len(charset))))
		if err != nil {
			return "", err
		}
		b[i] = charset[num.Int64()]
	}
	return string(b), nil
}

func (r *RedisReconciler) secretForRedis(m *cachev1.Redis, name string, password string) *corev1.Secret {
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: m.Namespace,
		},
		Type: corev1.SecretTypeOpaque,
		StringData: map[string]string{
			"password": password,
		},
	}
	ctrl.SetControllerReference(m, secret, r.Scheme)
	return secret
}

func (r *RedisReconciler) configMapForRedis(m *cachev1.Redis, name string) *corev1.ConfigMap {
	redisConf := `port 6379
cluster-enabled yes
cluster-config-file /data/nodes.conf
cluster-node-timeout 5000
dir /data
appendonly yes
`
	if m.Spec.MaxMemory != "" {
		redisConf += fmt.Sprintf("maxmemory %s\n", m.Spec.MaxMemory)
		redisConf += "maxmemory-policy allkeys-lru\n"
	}

	startupScript := `#!/bin/sh
redis-server /conf/redis.conf --requirepass "$REDIS_PASSWORD" --masterauth "$REDIS_PASSWORD" &
REDIS_PID=$!

until redis-cli -a "$REDIS_PASSWORD" -h localhost PING; do
  sleep 1
done

if [ "$(hostname)" != "{{.RedisName}}-0" ]; then
  wait $REDIS_PID
  exit $?
fi

nodes=""
for i in $(seq 0 $((${REDIS_REPLICAS}-1))); do
  pod_host="{{.RedisName}}-${i}.{{.HeadlessSvcName}}.{{.Namespace}}.svc.cluster.local"
  until redis-cli -a "$REDIS_PASSWORD" -h $pod_host PING; do
    sleep 1
  done
  nodes="$nodes $pod_host:6379"
done

if redis-cli -a "$REDIS_PASSWORD" -h localhost CLUSTER INFO | grep -q "cluster_state:ok"; then
  :
else
  echo "yes" | redis-cli -a "$REDIS_PASSWORD" --cluster create $nodes --cluster-replicas 1
fi

wait $REDIS_PID
`

	startupScript = strings.NewReplacer(
		"{{.RedisName}}", m.Name,
		"{{.HeadlessSvcName}}", fmt.Sprintf("%s-headless", m.Name),
		"{{.Namespace}}", m.Namespace,
	).Replace(startupScript)

	startupScript = strings.ReplaceAll(startupScript, "\r\n", "\n")

	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: m.Namespace,
		},
		Data: map[string]string{
			"redis.conf": redisConf,
			"startup.sh": startupScript,
		},
	}
	ctrl.SetControllerReference(m, cm, r.Scheme)
	return cm
}

func (r *RedisReconciler) statefulSetForRedis(m *cachev1.Redis, secretName, configMapName, headlessSvcName string) *appsv1.StatefulSet {
	replicas := m.Spec.Size
	labels := map[string]string{"app.kubernetes.io/name": "redis-cluster", "app.kubernetes.io/instance": m.Name}
	executableMode := int32(0755)
	fsGroup := int64(1001)

	redisImage := "bitnami/redis:latest"
	if m.Spec.Image.Repository != "" && m.Spec.Image.Tag != "" {
		redisImage = fmt.Sprintf("%s:%s", m.Spec.Image.Repository, m.Spec.Image.Tag)
	}

	storageSize := "1Gi"
	if m.Spec.Storage.Size != "" {
		storageSize = m.Spec.Storage.Size
	}

	var storageClassName *string
	if m.Spec.Storage.StorageClassName != "" {
		storageClassName = &m.Spec.Storage.StorageClassName
	}

	sts := &appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      m.Name,
			Namespace: m.Namespace,
		},
		Spec: appsv1.StatefulSetSpec{
			ServiceName: headlessSvcName,
			Replicas:    &replicas,
			Selector: &metav1.LabelSelector{
				MatchLabels: labels,
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: labels,
					Annotations: map[string]string{
						"prometheus.io/scrape": "true",
						"prometheus.io/port":   "9121",
					},
				},
				Spec: corev1.PodSpec{
					SecurityContext: &corev1.PodSecurityContext{
						FSGroup: &fsGroup,
					},
					Affinity:    m.Spec.Affinity,
					Tolerations: m.Spec.Tolerations,
					Containers: []corev1.Container{
						{
							Name:    "redis",
							Image:   redisImage,
							Command: []string{"/scripts/startup.sh"},
							Ports: []corev1.ContainerPort{
								{ContainerPort: 6379, Name: "redis"},
								{ContainerPort: 16379, Name: "cluster-bus"},
							},
							Env: []corev1.EnvVar{
								{Name: "REDIS_REPLICAS", Value: strconv.Itoa(int(m.Spec.Size))},
								{
									Name: "REDIS_PASSWORD",
									ValueFrom: &corev1.EnvVarSource{
										SecretKeyRef: &corev1.SecretKeySelector{
											LocalObjectReference: corev1.LocalObjectReference{Name: secretName},
											Key:                  "password",
										},
									},
								},
							},
							Resources: m.Spec.Resources,
							LivenessProbe: &corev1.Probe{
								ProbeHandler: corev1.ProbeHandler{
									Exec: &corev1.ExecAction{
										Command: []string{"redis-cli", "-a", "$(REDIS_PASSWORD)", "PING"},
									},
								},
								InitialDelaySeconds: 15,
								TimeoutSeconds:      5,
								PeriodSeconds:       10,
								FailureThreshold:    3,
							},
							ReadinessProbe: &corev1.Probe{
								ProbeHandler: corev1.ProbeHandler{
									Exec: &corev1.ExecAction{
										Command: []string{"redis-cli", "-a", "$(REDIS_PASSWORD)", "PING"},
									},
								},
								InitialDelaySeconds: 5,
								TimeoutSeconds:      5,
								PeriodSeconds:       10,
								FailureThreshold:    3,
							},
							VolumeMounts: []corev1.VolumeMount{
								{Name: "data", MountPath: "/data"},
								{Name: "config", MountPath: "/conf"},
								{Name: "scripts", MountPath: "/scripts"},
							},
						},
						{
							Name:  "redis-exporter",
							Image: "oliver006/redis_exporter:v1.72.0",
							Ports: []corev1.ContainerPort{{ContainerPort: 9121, Name: "metrics"}},
							Env: []corev1.EnvVar{{
								Name: "REDIS_PASSWORD",
								ValueFrom: &corev1.EnvVarSource{
									SecretKeyRef: &corev1.SecretKeySelector{
										LocalObjectReference: corev1.LocalObjectReference{Name: secretName},
										Key:                  "password",
									},
								},
							}},
							Args: []string{
								"--redis.addr=redis://localhost:6379",
								"--is-cluster",
							},
						},
					},
					Volumes: []corev1.Volume{
						{
							Name: "config",
							VolumeSource: corev1.VolumeSource{
								ConfigMap: &corev1.ConfigMapVolumeSource{
									LocalObjectReference: corev1.LocalObjectReference{Name: configMapName},
									Items:                []corev1.KeyToPath{{Key: "redis.conf", Path: "redis.conf"}},
								},
							},
						},
						{
							Name: "scripts",
							VolumeSource: corev1.VolumeSource{
								ConfigMap: &corev1.ConfigMapVolumeSource{
									LocalObjectReference: corev1.LocalObjectReference{Name: configMapName},
									Items:                []corev1.KeyToPath{{Key: "startup.sh", Path: "startup.sh"}},
									DefaultMode:          &executableMode,
								},
							},
						},
					},
				},
			},
			VolumeClaimTemplates: []corev1.PersistentVolumeClaim{
				{
					ObjectMeta: metav1.ObjectMeta{Name: "data"},
					Spec: corev1.PersistentVolumeClaimSpec{
						AccessModes: []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce},
						Resources: corev1.VolumeResourceRequirements{
							Requests: corev1.ResourceList{
								corev1.ResourceStorage: resource.MustParse(storageSize),
							},
						},
						StorageClassName: storageClassName,
					},
				},
			},
		},
	}
	ctrl.SetControllerReference(m, sts, r.Scheme)
	return sts
}

func (r *RedisReconciler) headlessServiceForRedis(m *cachev1.Redis, name string) *corev1.Service {
	labels := map[string]string{
		"app.kubernetes.io/name":     "redis-cluster",
		"app.kubernetes.io/instance": m.Name,
	}
	svc := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: m.Namespace,
			Labels:    labels,
		},
		Spec: corev1.ServiceSpec{
			ClusterIP: "None",
			Selector:  labels,
			Ports: []corev1.ServicePort{
				{Name: "redis", Port: 6379, TargetPort: intstr.FromString("redis")},
				{Name: "cluster-bus", Port: 16379, TargetPort: intstr.FromString("cluster-bus")},
			},
		},
	}
	ctrl.SetControllerReference(m, svc, r.Scheme)
	return svc
}

func (r *RedisReconciler) clientServiceForRedis(m *cachev1.Redis) *corev1.Service {
	labels := map[string]string{
		"app.kubernetes.io/name":     "redis-cluster",
		"app.kubernetes.io/instance": m.Name,
	}
	svc := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      m.Name,
			Namespace: m.Namespace,
			Labels:    labels,
		},
		Spec: corev1.ServiceSpec{
			Selector: labels,
			Ports: []corev1.ServicePort{
				{Name: "redis", Port: 6379, TargetPort: intstr.FromString("redis")},
				{Name: "metrics", Port: 9121, TargetPort: intstr.FromString("metrics")},
			},
		},
	}

	ctrl.SetControllerReference(m, svc, r.Scheme)
	return svc
}

func (r *RedisReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&cachev1.Redis{}).
		Owns(&appsv1.StatefulSet{}).
		Owns(&corev1.Secret{}).
		Owns(&corev1.ConfigMap{}).
		Owns(&corev1.Service{}).
		Named("redis").
		Complete(r)
}