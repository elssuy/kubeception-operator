/*
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
*/

package controller

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/component-base/config/v1alpha1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/manager"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	clusterv1alpha1 "github.com/elssuy/kubeception-operator/api/v1alpha1"

	kubescheduler "k8s.io/kube-scheduler/config/v1"
)

// KubeSchedulerReconciler reconciles a KubeScheduler object
type KubeSchedulerReconciler struct {
	client.Client
	Scheme *runtime.Scheme
	log    logr.Logger
}

func NewKubeSchedulerReconciler(mgr manager.Manager) *KubeSchedulerReconciler {
	return &KubeSchedulerReconciler{
		Client: mgr.GetClient(),
		Scheme: mgr.GetScheme(),
		log:    log.Log.WithName("kube-scheduler-reconciler"),
	}
}

//+kubebuilder:rbac:groups=cluster.kubeception.ulfo.fr,resources=kubeschedulers,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=cluster.kubeception.ulfo.fr,resources=kubeschedulers/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=cluster.kubeception.ulfo.fr,resources=kubeschedulers/finalizers,verbs=update
//+kubebuilder:rbac:groups=core,resources=secrets,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=apps,resources=deployments,verbs=get;list;watch;create;update;patch;delete

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the KubeScheduler object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.14.4/pkg/reconcile
func (r *KubeSchedulerReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {

	ks := &clusterv1alpha1.KubeScheduler{}
	if err := r.Get(ctx, req.NamespacedName, ks); err != nil {
		if apierrors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}

		r.log.Error(err, "failed to get KubeScheduler resource", "name", req.Name, "namespace", req.Namespace)
		return ctrl.Result{}, err
	}

	//////////
	// Checks
	//////////
	kubeSchedulerTls := &corev1.Secret{}
	if err := r.Get(ctx, types.NamespacedName{Name: ks.Spec.KubeSchedulerTls, Namespace: req.Namespace}, kubeSchedulerTls); err != nil {
		r.log.Info("failed to get tls secret for KubeScheduler, requeing", "name", ks.Spec.KubeSchedulerTls, "namespace", req.Namespace)
		return ctrl.Result{RequeueAfter: 3 * time.Second}, nil
	}

	////////////
	// Kube Scheduler kubeconfig
	////////////
	kubeSchedulerConfig := &corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: "kube-scheduler-config", Namespace: req.Namespace}}
	err := r.CreateOrPatch(ctx, kubeSchedulerConfig, ks, func() error {

		config := &kubescheduler.KubeSchedulerConfiguration{
			TypeMeta: metav1.TypeMeta{
				APIVersion: "kubescheduler.config.k8s.io/v1",
				Kind:       "KubeSchedulerConfiguration",
			},
			ClientConnection: v1alpha1.ClientConnectionConfiguration{
				Kubeconfig: "/var/lib/kubernetes/auth/kubeconfig.yml",
			},
		}
		configyaml, err := json.Marshal(config)
		if err != nil {
			r.log.Error(err, "failed to marshal kube-scheduler config", "name", "kube-scheduler-config", "namespace", req.Namespace)
			return err
		}

		// kubeSchedulerConfig := &kubescheduler.KubeSchedulerConfiguration{}
		k, err := GenerateKubeconfigFromSecret(*kubeSchedulerTls, fmt.Sprintf("https://%s:%d", ks.Spec.KubeAPIServerService.Name, ks.Spec.KubeAPIServerService.Port))
		if err != nil {
			return err
		}

		kubeSchedulerConfig.Data = map[string][]byte{
			"kubeconfig.yml": k,
			"config.json":    configyaml,
		}

		return nil
	})
	if err != nil {
		return ctrl.Result{}, err
	}

	////////////
	// Deployment
	////////////

	deployment := &appsv1.Deployment{ObjectMeta: metav1.ObjectMeta{Name: "kube-scheduler", Namespace: req.Namespace}}
	err = r.CreateOrPatch(ctx, deployment, ks, func() error {
		var autoMountSA bool = false

		deployment.Labels = labels("kube-scheduler", ks.Name, map[string]string{})
		deployment.Spec = appsv1.DeploymentSpec{
			Replicas: &ks.Spec.Deployment.Replicas,
			Selector: &metav1.LabelSelector{
				MatchLabels: labels("kube-scheduler", ks.Name, map[string]string{}),
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: labels("kube-scheduler", ks.Name, map[string]string{}),
				},
				Spec: corev1.PodSpec{
					Volumes: []corev1.Volume{
						{Name: "kubeconfig", VolumeSource: corev1.VolumeSource{Secret: &corev1.SecretVolumeSource{SecretName: "kube-scheduler-config"}}},
						{Name: "kube-scheduler", VolumeSource: corev1.VolumeSource{Secret: &corev1.SecretVolumeSource{SecretName: ks.Spec.KubeSchedulerTls}}},
					},
					Containers: []corev1.Container{
						{
							Name:  "kube-scheduler",
							Image: fmt.Sprintf("registry.k8s.io/kube-scheduler:%s", ks.Spec.Version),
							Command: []string{
								"/usr/local/bin/kube-scheduler",
								"--config",
								"/var/lib/kubernetes/auth/config.json",

								"--authentication-skip-lookup",
								"--authentication-kubeconfig",
								"/var/lib/kubernetes/auth/kubeconfig.yml",
								"--authorization-kubeconfig",
								"/var/lib/kubernetes/auth/kubeconfig.yml",

								"--tls-cert-file",
								"/var/lib/kubernetes/tls/ks/tls.crt",
								"--tls-private-key-file",
								"/var/lib/kubernetes/tls/ks/tls.key",

								"--client-ca-file",
								"/var/lib/kubernetes/tls/ks/ca.crt",
							},
							Ports: []corev1.ContainerPort{
								{Name: "https", ContainerPort: 10259},
							},
							VolumeMounts: []corev1.VolumeMount{
								{Name: "kube-scheduler", MountPath: "/etc/ssl/certs"},
								{Name: "kube-scheduler", MountPath: "/var/lib/kubernetes/tls/ks"},
								{Name: "kubeconfig", MountPath: "/var/lib/kubernetes/auth"},
							},
							LivenessProbe: &corev1.Probe{
								InitialDelaySeconds: 10,
								TimeoutSeconds:      15,
								FailureThreshold:    8,
								PeriodSeconds:       10,
								SuccessThreshold:    1,
								ProbeHandler: corev1.ProbeHandler{
									HTTPGet: &corev1.HTTPGetAction{
										Port:   intstr.FromInt(10259),
										Path:   "/healthz",
										Scheme: corev1.URISchemeHTTPS,
									},
								},
							},
						},
					},
					AutomountServiceAccountToken: &autoMountSA,
				},
			},
		}
		return nil
	})
	if err != nil {
		return ctrl.Result{}, err
	}
	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *KubeSchedulerReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&clusterv1alpha1.KubeScheduler{}).
		Owns(&corev1.Secret{}).
		Owns(&appsv1.Deployment{}).
		Complete(r)
}

func (r *KubeSchedulerReconciler) CreateOrPatch(ctx context.Context, obj client.Object, owner metav1.Object, f controllerutil.MutateFn) error {
	if err := ctrl.SetControllerReference(owner, obj, r.Scheme); err != nil {
		r.log.Error(err, fmt.Sprintf("failed to set controller reference on %s/%s", obj.GetObjectKind().GroupVersionKind().Kind, obj.GetName()), "name", obj.GetName(), "namespace", obj.GetNamespace())
		return err
	}

	result, err := controllerutil.CreateOrPatch(ctx, r.Client, obj, f)
	if err != nil {
		r.log.Error(err, fmt.Sprintf("failed to create or patch %s", obj.GetObjectKind().GroupVersionKind().Kind), "name", obj.GetName(), "namespace", obj.GetNamespace())
		return err
	}
	r.log.Info(fmt.Sprintf("%s/%s cert was %s", obj.GetObjectKind().GroupVersionKind().Kind, obj.GetName(), result))
	return nil
}
