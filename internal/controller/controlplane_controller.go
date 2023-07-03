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

	"github.com/go-logr/logr"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"

	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/manager"

	// yaml "k8s.io/apimachinery/pkg/util/yaml"
	// kubeletconfigv1beta1 "k8s.io/kubelet/config/v1beta1"
	clusterv1alpha1 "kubeception.ulfo.fr/api/v1alpha1"
)

const (
	pkiName       = "pki"
	ApiServerName = "api-server"
)

// ControlPlaneReconciler reconciles a ControlPlane object
type ControlPlaneReconciler struct {
	client.Client
	Scheme *runtime.Scheme
	log    logr.Logger
}

func NewControlPlaneReconciler(mgr manager.Manager) *ControlPlaneReconciler {
	return &ControlPlaneReconciler{
		Client: mgr.GetClient(),
		Scheme: mgr.GetScheme(),
		log:    log.Log.WithName("controlplane-reconciler"),
	}
}

//+kubebuilder:rbac:groups=cluster.kubeception.ulfo.fr,resources=controlplanes,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=cluster.kubeception.ulfo.fr,resources=controlplanes/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=cluster.kubeception.ulfo.fr,resources=controlplanes/finalizers,verbs=update
//+kubebuilder:rbac:groups=cluster.kubeception.ulfo.fr,resources=loadbalancers,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=cluster.kubeception.ulfo.fr,resources=pkis,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=cluster.kubeception.ulfo.fr,resources=kubeapiservers,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=cluster.kubeception.ulfo.fr,resources=kubecontrollermanagers,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=cluster.kubeception.ulfo.fr,resources=kubeschedulers,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=cluster.kubeception.ulfo.fr,resources=kubeschedulers,verbs=get;list;watch;create;update;patch;delete

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the ControlPlane object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.14.4/pkg/reconcile
func (r *ControlPlaneReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {

	cp := &clusterv1alpha1.ControlPlane{}
	if err := r.Get(ctx, req.NamespacedName, cp); err != nil {
		// Resource has been deleted
		if apierrors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		r.log.Error(err, "Failed to get control plane resource", "name", req.Name, "namespace", req.Namespace)
		return ctrl.Result{}, err
	}

	// Create loadbalancer
	lb := &clusterv1alpha1.Loadbalancer{ObjectMeta: metav1.ObjectMeta{Name: req.Name, Namespace: req.Namespace}}

	if err := ctrl.SetControllerReference(cp, lb, r.Scheme); err != nil {
		r.log.Error(err, "failed to set controller reference on LoadBalancer", "name", req.Name, "namespace", req.Namespace)
		return ctrl.Result{}, err
	}

	result, err := controllerutil.CreateOrPatch(ctx, r.Client, lb, func() error {
		lb.Spec = cp.Spec.Loadbalancer
		return nil
	})
	if err != nil {
		r.log.Error(err, "failed to create or patch Loadbalancer", "name", req.Name, "namespace", req.Namespace)
		return ctrl.Result{}, err
	}

	r.log.Info(fmt.Sprintf("Loadbalancer was: %s", result))

	// Create PKI
	pki := &clusterv1alpha1.Pki{ObjectMeta: metav1.ObjectMeta{Name: req.Name, Namespace: req.Namespace}}
	if err := ctrl.SetControllerReference(cp, pki, r.Scheme); err != nil {
		r.log.Error(err, "failed to set controller reference on pki", "name", req.Name, "namespace", req.Namespace)
		return ctrl.Result{}, err
	}

	result, err = controllerutil.CreateOrPatch(ctx, r.Client, pki, func() error {
		pki.Spec = cp.Spec.PKI
		pki.Spec.ControlPlaneIP = lb.Status.IP
		return nil
	})
	if err != nil {
		r.log.Error(err, "failed to create or patch PKI", "name", req.Name, "namespace", req.Namespace)
		return ctrl.Result{}, err
	}

	r.log.Info(fmt.Sprintf("PKI was: %s", result))

	// Create ApiServer

	kas := &clusterv1alpha1.KubeAPIServer{ObjectMeta: metav1.ObjectMeta{Name: req.Name, Namespace: req.Namespace}}
	if err := ctrl.SetControllerReference(cp, kas, r.Scheme); err != nil {
		r.log.Error(err, "failed to set controller reference on ApiServer", "name", req.Name, "namespace", req.Namespace)
		return ctrl.Result{}, err
	}

	result, err = controllerutil.CreateOrPatch(ctx, r.Client, kas, func() error {
		kas.Spec = cp.Spec.KubeApiServer

		if kas.Spec.Deployment.Labels == nil {
			kas.Spec.Deployment.Labels = make(map[string]string)
		}

		for k, v := range lb.Spec.Selectors {
			kas.Spec.Deployment.Labels[k] = v
		}

		kas.Spec.Options.AdvertiseAddress = lb.Status.IP

		return nil
	})
	if err != nil {
		r.log.Error(err, "failed to create or patch KubeAPIServer", "name", req.Name, "namespace", req.Namespace)
		return ctrl.Result{}, err
	}
	r.log.Info(fmt.Sprintf("KubeAPIServer was: %s", result))

	///
	/// KubeControllerManager
	///

	kcm := &clusterv1alpha1.KubeControllerManager{ObjectMeta: metav1.ObjectMeta{Name: req.Name, Namespace: req.Namespace}}
	if err := ctrl.SetControllerReference(cp, kcm, r.Scheme); err != nil {
		r.log.Error(err, "failed to set controller reference on KubeControllerManager", "name", req.Name, "namespace", req.Namespace)
		return ctrl.Result{}, err
	}

	result, err = controllerutil.CreateOrPatch(ctx, r.Client, kcm, func() error {
		kcm.Spec = cp.Spec.KubeControllerManager
		return nil
	})
	if err != nil {
		r.log.Error(err, "failed to create or patch KubeControllerManager", "name", req.Name, "namespace", req.Namespace)
		return ctrl.Result{}, err
	}
	r.log.Info(fmt.Sprintf("KubeControllerManager was: %s", result))

	///
	/// KubeScheduler
	///

	ks := &clusterv1alpha1.KubeScheduler{ObjectMeta: metav1.ObjectMeta{Name: req.Name, Namespace: req.Namespace}}
	if err := ctrl.SetControllerReference(cp, ks, r.Scheme); err != nil {
		r.log.Error(err, "failed to set controller reference on KubeScheduler", "name", req.Name, "namespace", req.Namespace)
		return ctrl.Result{}, err
	}

	result, err = controllerutil.CreateOrPatch(ctx, r.Client, ks, func() error {
		ks.Spec = cp.Spec.KubeScheduler
		return nil
	})
	if err != nil {
		r.log.Error(err, "failed to create or patch KubeScheduler", "name", req.Name, "namespace", req.Namespace)
		return ctrl.Result{}, err
	}
	r.log.Info(fmt.Sprintf("KubeScheduler was: %s", result))

	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *ControlPlaneReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&clusterv1alpha1.ControlPlane{}).
		Owns(&clusterv1alpha1.Pki{}).
		Owns(&clusterv1alpha1.KubeAPIServer{}).
		Owns(&clusterv1alpha1.KubeControllerManager{}).
		Owns(&clusterv1alpha1.Loadbalancer{}).
		Complete(r)
}
