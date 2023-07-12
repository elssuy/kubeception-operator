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
	"fmt"
	"time"

	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/manager"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	clusterv1alpha1 "kubeception.ulfo.fr/api/v1alpha1"
)

// KubeControllerManagerReconciler reconciles a KubeControllerManager object
type KubeControllerManagerReconciler struct {
	client.Client
	Scheme *runtime.Scheme
	log    logr.Logger
}

func NewKubeControllerManagerReconciler(mgr manager.Manager) *KubeControllerManagerReconciler {
	return &KubeControllerManagerReconciler{
		Client: mgr.GetClient(),
		Scheme: mgr.GetScheme(),
		log:    log.Log.WithName("kube-controller-manager-controller"),
	}
}

//+kubebuilder:rbac:groups=cluster.kubeception.ulfo.fr,resources=kubecontrollermanagers,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=cluster.kubeception.ulfo.fr,resources=kubecontrollermanagers/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=cluster.kubeception.ulfo.fr,resources=kubecontrollermanagers/finalizers,verbs=update
//+kubebuilder:rbac:groups=core,resources=secrets,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=apps,resources=deployments,verbs=get;list;watch;create;update;patch;delete

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the KubeControllerManager object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.14.4/pkg/reconcile
func (r *KubeControllerManagerReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {

	kcm := &clusterv1alpha1.KubeControllerManager{}
	if err := r.Get(ctx, req.NamespacedName, kcm); err != nil {
		if apierrors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}

		r.log.Error(err, "failed to get KubeControllerManager resource", "name", req.Name, "namespace", req.Namespace)
		return ctrl.Result{}, err
	}

	////////////////////////
	// Checks
	////////////////////////
	kubeControllerManagerSecret := &corev1.Secret{}
	if err := r.Get(ctx, types.NamespacedName{Name: kcm.Spec.TLS.KubeControllerManager, Namespace: req.Namespace}, kubeControllerManagerSecret); err != nil {
		r.log.Info("failed to get tls secret for KubeControllerManager, requeing", "name", kcm.Spec.TLS.KubeControllerManager, "namespace", req.Namespace)
		return ctrl.Result{RequeueAfter: 3 * time.Second}, nil
	}

	if err := r.Get(ctx, types.NamespacedName{Name: kcm.Spec.TLS.ServiceAccountsTLS, Namespace: req.Namespace}, &corev1.Secret{}); err != nil {
		r.log.Info("failed to get tls secret for Service Accounts, requeing", "name", kcm.Spec.TLS.ServiceAccountsTLS, "namespace", req.Namespace)
		return ctrl.Result{RequeueAfter: 3 * time.Second}, nil
	}

	if err := r.Get(ctx, types.NamespacedName{Name: kcm.Spec.TLS.CA, Namespace: req.Namespace}, &corev1.Secret{}); err != nil {
		r.log.Info("failed to get tls secret for CA, requeing", "name", kcm.Spec.TLS.CA, "namespace", req.Namespace)
		return ctrl.Result{RequeueAfter: 3 * time.Second}, nil
	}

	////////////
	// Controller manager kubeconfig
	////////////
	kubeControllerManagerKubeconfig := &corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: "kube-controller-manager-kubeconfig", Namespace: req.Namespace}}
	err := r.CreateOrPatch(ctx, kubeControllerManagerKubeconfig, kcm, func() error {
		k, err := GenerateKubeconfigFromSecret(*kubeControllerManagerSecret, fmt.Sprintf("https://%s:%d", kcm.Spec.KubeAPIServerService.Name, kcm.Spec.KubeAPIServerService.Port))
		if err != nil {
			return err
		}
		kubeControllerManagerKubeconfig.Data = make(map[string][]byte)
		kubeControllerManagerKubeconfig.Data["kubeconfig.yml"] = k

		return nil
	})
	if err != nil {
		return ctrl.Result{}, err
	}

	////////////
	// Deployment
	////////////

	deployment := &appsv1.Deployment{ObjectMeta: metav1.ObjectMeta{Name: "kube-controller-manager", Namespace: req.Namespace}}
	err = r.CreateOrPatch(ctx, deployment, kcm, func() error {
		var autoMountSA bool = false

		deployment.Labels = labels("kube-controller-manager", kcm.Name, kcm.Spec.Deployment.Labels)
		deployment.Spec = appsv1.DeploymentSpec{
			Replicas: &kcm.Spec.Deployment.Replicas,
			Selector: &metav1.LabelSelector{
				MatchLabels: labels("kube-controller-manager", kcm.Name, kcm.Spec.Deployment.Labels),
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: labels("kube-controller-manager", kcm.Name, kcm.Spec.Deployment.Labels),
				},
				Spec: corev1.PodSpec{
					Volumes: []corev1.Volume{
						{Name: "kubeconfig", VolumeSource: corev1.VolumeSource{Secret: &corev1.SecretVolumeSource{SecretName: "kube-controller-manager-kubeconfig"}}},
						{Name: "kube-controller-manager", VolumeSource: corev1.VolumeSource{Secret: &corev1.SecretVolumeSource{SecretName: "kube-controller-manager"}}},
						{Name: "service-accounts", VolumeSource: corev1.VolumeSource{Secret: &corev1.SecretVolumeSource{SecretName: "service-accounts"}}},
						{Name: "ca", VolumeSource: corev1.VolumeSource{Secret: &corev1.SecretVolumeSource{SecretName: kcm.Spec.TLS.CA}}},
					},
					Containers: []corev1.Container{
						{
							Name:  "kube-controller-manager",
							Image: fmt.Sprintf("registry.k8s.io/kube-controller-manager:%s", kcm.Spec.Version),
							Command: []string{
								"/usr/local/bin/kube-controller-manager",
								"--authentication-skip-lookup",

								"--cluster-cidr",
								"10.200.0.0/16",

								"--service-cluster-ip-range",
								"10.32.0.0/24",

								"--tls-cert-file",
								"/var/lib/kubernetes/tls/kcm/tls.crt",
								"--tls-private-key-file",
								"/var/lib/kubernetes/tls/kcm/tls.key",

								"--kubeconfig",
								"/var/lib/kubernetes/auth/kubeconfig.yml",
								"--authentication-kubeconfig",
								"/var/lib/kubernetes/auth/kubeconfig.yml",
								"--authorization-kubeconfig",
								"/var/lib/kubernetes/auth/kubeconfig.yml",

								"--use-service-account-credentials",

								"--client-ca-file",
								"/var/lib/kubernetes/tls/kcm/ca.crt",

								"--root-ca-file",
								"/var/lib/kubernetes/tls/kcm/ca.crt",

								"--service-account-private-key-file",
								"/var/lib/kubernetes/tls/sa/tls.key",

								"--cloud-provider", "external",

								"--cluster-signing-cert-file",
								"/var/lib/kubernetes/tls/ca/tls.crt",

								"--cluster-signing-key-file",
								"/var/lib/kubernetes/tls/ca/tls.key",

								"--controllers",
								"*,bootstrapsigner,tokencleaner",
							},
							Ports: []corev1.ContainerPort{
								{Name: "https", ContainerPort: 10257},
							},
							VolumeMounts: []corev1.VolumeMount{
								{Name: "kube-controller-manager", MountPath: "/etc/ssl/certs"},
								{Name: "kube-controller-manager", MountPath: "/var/lib/kubernetes/tls/kcm"},
								{Name: "ca", MountPath: "/var/lib/kubernetes/tls/ca"},
								{Name: "kubeconfig", MountPath: "/var/lib/kubernetes/auth"},
								{Name: "service-accounts", MountPath: "/var/lib/kubernetes/tls/sa"},
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
func (r *KubeControllerManagerReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&clusterv1alpha1.KubeControllerManager{}).
		Owns(&appsv1.Deployment{}).
		Owns(&corev1.Secret{}).
		Complete(r)
}

func (r *KubeControllerManagerReconciler) CreateOrPatch(ctx context.Context, obj client.Object, owner metav1.Object, f controllerutil.MutateFn) error {
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
