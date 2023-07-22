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
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/intstr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/manager"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	clusterv1alpha1 "github.com/elssuy/kubeception/api/v1alpha1"
)

// LoadbalancerReconciler reconciles a Loadbalancer object
type LoadbalancerReconciler struct {
	client.Client
	Scheme *runtime.Scheme
	log    logr.Logger
}

func NewLoadbalancerReconciler(mgr manager.Manager) *LoadbalancerReconciler {
	return &LoadbalancerReconciler{
		Client: mgr.GetClient(),
		Scheme: mgr.GetScheme(),
		log:    log.Log.WithName("loadbalancer-reconciler"),
	}
}

//+kubebuilder:rbac:groups=cluster.kubeception.ulfo.fr,resources=loadbalancers,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=cluster.kubeception.ulfo.fr,resources=loadbalancers/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=cluster.kubeception.ulfo.fr,resources=loadbalancers/finalizers,verbs=update
//+kubebuilder:rbac:groups=core,resources=services,verbs=get;list;watch;create;update;patch;delete

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the Loadbalancer object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.14.4/pkg/reconcile
func (r *LoadbalancerReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {

	lb := &clusterv1alpha1.Loadbalancer{}
	if err := r.Get(ctx, req.NamespacedName, lb); err != nil {
		// Resource has been deleted
		if apierrors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		r.log.Error(err, "failed to get Loadbalancer resource", "name", req.Name, "namespace", req.Namespace)
		return ctrl.Result{}, err
	}

	service := &corev1.Service{ObjectMeta: metav1.ObjectMeta{Name: lb.Spec.Name, Namespace: req.Namespace}}
	err := r.CreateOrPatch(ctx, service, lb, func() error {
		labels := map[string]string{
			"app.kubernetes.io/name": "kube-apiserver",
		}

		for k, v := range lb.Spec.Selectors {
			labels[k] = v
		}

		service.Spec = corev1.ServiceSpec{
			Type: corev1.ServiceTypeLoadBalancer,
			Ports: []corev1.ServicePort{
				{Name: "https", Port: lb.Spec.Port, Protocol: corev1.ProtocolTCP, TargetPort: intstr.FromInt(6443)},
				{Name: "konnectivity", Port: 8091, Protocol: corev1.ProtocolTCP, TargetPort: intstr.FromInt(8091)},
			},
			Selector: labels,
		}
		return nil
	})
	if err != nil {
		r.log.Error(err, "failed to create APIServer service", "name", lb.Spec.Name, "namespace", req.Namespace)
		return ctrl.Result{}, err
	}

	// Update Status
	if len(service.Status.LoadBalancer.Ingress) > 0 {
		lb.Status.IP = service.Status.LoadBalancer.Ingress[0].IP
		if err := r.Status().Update(ctx, lb); err != nil {
			r.log.Error(err, "failed to update Loadbalancer IP Status, requing for 3 seconds", "name", req.Name, "namespace", req.Namespace)
			return ctrl.Result{}, nil
		}
	}

	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *LoadbalancerReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&clusterv1alpha1.Loadbalancer{}).
		Owns(&corev1.Service{}).
		Complete(r)
}

func (r *LoadbalancerReconciler) CreateOrPatch(ctx context.Context, obj client.Object, owner metav1.Object, f controllerutil.MutateFn) error {
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
