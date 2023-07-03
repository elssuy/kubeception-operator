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

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"

	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/manager"

	certmanagerv1 "github.com/cert-manager/cert-manager/pkg/apis/certmanager/v1"
	certmanagermetav1 "github.com/cert-manager/cert-manager/pkg/apis/meta/v1"
	"github.com/go-logr/logr"
	clusterv1alpha1 "kubeception.ulfo.fr/api/v1alpha1"
)

// PkiReconciler reconciles a Pki object
type PkiReconciler struct {
	client.Client
	Scheme *runtime.Scheme
	log    logr.Logger
}

func NewPkiReconciler(mgr manager.Manager) *PkiReconciler {
	return &PkiReconciler{
		Client: mgr.GetClient(),
		Scheme: mgr.GetScheme(),
		log:    log.Log.WithName("pki-reconciler"),
	}
}

//+kubebuilder:rbac:groups=cluster.kubeception.ulfo.fr,resources=pkis,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=cluster.kubeception.ulfo.fr,resources=pkis/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=cluster.kubeception.ulfo.fr,resources=pkis/finalizers,verbs=update
//+kubebuilder:rbac:groups=core,resources=secrets,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=cert-manager.io,resources=certificates,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=cert-manager.io,resources=issuers,verbs=get;list;watch;create;update;patch;delete

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the Pki object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.14.4/pkg/reconcile
func (r *PkiReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {

	pki := &clusterv1alpha1.Pki{}
	err := r.Get(ctx, req.NamespacedName, pki)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		r.log.Error(err, "failed to get PKI resource", "name", req.Name, "namespace", req.Namespace)
		return ctrl.Result{}, err
	}

	///////////////
	// Root ISSUER
	///////////////
	rootIssuer := &certmanagerv1.Issuer{ObjectMeta: metav1.ObjectMeta{Name: pki.Spec.Name, Namespace: req.Namespace}}
	r.CreateOrPatch(ctx, rootIssuer, pki, func() error {
		rootIssuer.Spec = certmanagerv1.IssuerSpec{
			IssuerConfig: certmanagerv1.IssuerConfig{
				SelfSigned: &certmanagerv1.SelfSignedIssuer{},
			},
		}
		return nil
	})

	////////////
	// Root CA
	///////////

	rootCa := &certmanagerv1.Certificate{ObjectMeta: metav1.ObjectMeta{Name: pki.Spec.CA.Name, Namespace: req.Namespace}}
	r.CreateOrPatch(ctx, rootCa, pki, func() error {
		rootCa.Spec = certmanagerv1.CertificateSpec{
			IsCA:       true,
			CommonName: "ca",
			SecretName: pki.Spec.CA.Name,
			DNSNames:   []string{"localhost"},
			PrivateKey: &certmanagerv1.CertificatePrivateKey{
				Algorithm: certmanagerv1.RSAKeyAlgorithm,
				Size:      2048,
			},
			IssuerRef: certmanagermetav1.ObjectReference{
				Name: pki.Spec.Name,
				Kind: "Issuer",
			},
		}
		return nil
	})

	////////////
	// CA ISSUER
	////////////
	rootCaIssuer := &certmanagerv1.Issuer{ObjectMeta: metav1.ObjectMeta{Name: pki.Spec.CA.Name, Namespace: req.Namespace}}
	r.CreateOrPatch(ctx, rootCaIssuer, pki, func() error {
		rootCaIssuer.Spec.IssuerConfig = certmanagerv1.IssuerConfig{
			CA: &certmanagerv1.CAIssuer{
				SecretName: pki.Spec.CA.Name,
			},
		}
		return nil
	})

	////////////
	// ADMIN CERT
	////////////
	adminCert := &certmanagerv1.Certificate{ObjectMeta: metav1.ObjectMeta{Name: pki.Spec.Admin.Name, Namespace: req.Namespace}}
	r.CreateOrPatch(ctx, adminCert, pki, func() error {
		adminCert.Spec = certmanagerv1.CertificateSpec{
			CommonName: "cluster-admin",
			Subject: &certmanagerv1.X509Subject{
				Organizations: []string{"system:masters"},
			},
			SecretName: pki.Spec.Admin.Name,
			PrivateKey: &certmanagerv1.CertificatePrivateKey{
				Algorithm: certmanagerv1.RSAKeyAlgorithm,
				Size:      2048,
			},
			IssuerRef: certmanagermetav1.ObjectReference{
				Name: pki.Spec.CA.Name,
				Kind: "Issuer",
			},
		}
		return nil
	})

	////////////
	// ADMIN KUBECONFIG
	////////////

	adminCertSecret := &corev1.Secret{}
	if err = r.Get(ctx, types.NamespacedName{Namespace: req.Namespace, Name: pki.Spec.Admin.Name}, adminCertSecret); err != nil {
		r.log.Info("Admin certificate secret not found retrying later", "name", pki.Spec.Admin.Name, "namespace", req.Namespace)
		return ctrl.Result{RequeueAfter: 3 * time.Second}, nil
	}

	adminKubeconfigName := fmt.Sprintf("%s-kubeconfig", pki.Spec.Admin.Name)
	adminKubeconfig := &corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: adminKubeconfigName, Namespace: req.Namespace}}
	r.CreateOrPatch(ctx, adminKubeconfig, pki, func() error {
		kc, err := GenerateKubeconfigFromSecret(*adminCertSecret, fmt.Sprintf("https://%s:6443", pki.Spec.ControlPlaneIP))
		if err != nil {
			r.log.Error(err, "failed to generate admin kubeconfig secret")
			return err
		}
		adminKubeconfig.Data = map[string][]byte{}
		adminKubeconfig.Data["kubeconfig.yml"] = kc
		return nil
	})

	///

	if pki.Spec.ControlPlaneIP == "" {
		r.log.Info("Control Plane IP is not registered, retrying later", "name", req.Name, "namespace", req.Namespace)
		return ctrl.Result{RequeueAfter: 3 * time.Second}, nil
	}

	////////////
	// APIServer CERT
	////////////

	kubeAPIServerCert := &certmanagerv1.Certificate{ObjectMeta: metav1.ObjectMeta{Name: pki.Spec.KubeAPIServer.Name, Namespace: req.Namespace}}
	r.CreateOrPatch(ctx, kubeAPIServerCert, pki, func() error {
		kubeAPIServerCert.Spec = certmanagerv1.CertificateSpec{
			CommonName: "kube-apiserver",
			Subject: &certmanagerv1.X509Subject{
				Organizations: []string{"kubernetes"},
			},
			IPAddresses: append(pki.Spec.KubeAPIServer.IPAddresses, pki.Spec.ControlPlaneIP),
			DNSNames:    append(pki.Spec.KubeAPIServer.DNSNames, pki.Spec.ControlPlaneIP),
			SecretName:  pki.Spec.KubeAPIServer.Name,
			PrivateKey: &certmanagerv1.CertificatePrivateKey{
				Algorithm: certmanagerv1.RSAKeyAlgorithm,
				Size:      2048,
			},
			IssuerRef: certmanagermetav1.ObjectReference{
				Name: pki.Spec.CA.Name,
				Kind: "Issuer",
			},
		}
		return nil
	})

	////////////
	// Service Account cert
	////////////
	serviceAccountCert := &certmanagerv1.Certificate{ObjectMeta: metav1.ObjectMeta{Name: pki.Spec.ServiceAccounts.Name, Namespace: req.Namespace}}
	r.CreateOrPatch(ctx, serviceAccountCert, pki, func() error {
		serviceAccountCert.Spec = certmanagerv1.CertificateSpec{
			CommonName: "kubernetes",
			Subject: &certmanagerv1.X509Subject{
				Organizations: []string{"Kubernetes"},
			},
			SecretName: pki.Spec.ServiceAccounts.Name,
			PrivateKey: &certmanagerv1.CertificatePrivateKey{
				Algorithm: certmanagerv1.RSAKeyAlgorithm,
				Size:      2048,
			},
			IssuerRef: certmanagermetav1.ObjectReference{
				Name: pki.Spec.CA.Name,
				Kind: "Issuer",
			},
		}
		return nil
	})

	////////////
	// Controller manager cert
	////////////

	kubeControllerManagerCert := &certmanagerv1.Certificate{ObjectMeta: metav1.ObjectMeta{Name: pki.Spec.KubeControllerManager.Name, Namespace: req.Namespace}}
	r.CreateOrPatch(ctx, kubeControllerManagerCert, pki, func() error {
		kubeControllerManagerCert.Spec = certmanagerv1.CertificateSpec{
			CommonName: "system:kube-controller-manager",
			Subject: &certmanagerv1.X509Subject{
				Organizations: []string{"system:kube-controller-manager"},
			},
			SecretName: pki.Spec.KubeControllerManager.Name,
			PrivateKey: &certmanagerv1.CertificatePrivateKey{
				Algorithm: certmanagerv1.RSAKeyAlgorithm,
				Size:      2048,
			},
			IssuerRef: certmanagermetav1.ObjectReference{
				Name: pki.Spec.CA.Name,
				Kind: "Issuer",
			},
		}
		return nil
	})

	////////////
	// Scheduler cert
	////////////
	kubeSchedulerCert := &certmanagerv1.Certificate{ObjectMeta: metav1.ObjectMeta{Name: pki.Spec.KubeScheduler.Name, Namespace: req.Namespace}}
	r.CreateOrPatch(ctx, kubeSchedulerCert, pki, func() error {
		kubeSchedulerCert.Spec = certmanagerv1.CertificateSpec{
			CommonName: "system:kube-scheduler",
			Subject: &certmanagerv1.X509Subject{
				Organizations: []string{"system:kube-scheduler"},
			},
			SecretName: pki.Spec.KubeScheduler.Name,
			PrivateKey: &certmanagerv1.CertificatePrivateKey{
				Algorithm: certmanagerv1.RSAKeyAlgorithm,
				Size:      2048,
			},
			IssuerRef: certmanagermetav1.ObjectReference{
				Name: pki.Spec.CA.Name,
				Kind: "Issuer",
			},
		}
		return nil
	})

	////////////
	// Konnectivity CERT
	////////////
	konnectivityCert := &certmanagerv1.Certificate{ObjectMeta: metav1.ObjectMeta{Name: pki.Spec.Konnectivity.Name, Namespace: req.Namespace}}
	r.CreateOrPatch(ctx, konnectivityCert, pki, func() error {
		konnectivityCert.Spec = certmanagerv1.CertificateSpec{
			CommonName: "system:konnectivity-server",
			SecretName: pki.Spec.Konnectivity.Name,
			PrivateKey: &certmanagerv1.CertificatePrivateKey{
				Algorithm: certmanagerv1.RSAKeyAlgorithm,
				Size:      2048,
			},
			IssuerRef: certmanagermetav1.ObjectReference{
				Name: pki.Spec.CA.Name,
				Kind: "Issuer",
			},
		}
		return nil
	})

	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *PkiReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&clusterv1alpha1.Pki{}).
		Owns(&certmanagerv1.Certificate{}).
		Owns(&certmanagerv1.Issuer{}).
		Owns(&corev1.Secret{}).
		Complete(r)
}

func (r *PkiReconciler) CreateOrPatch(ctx context.Context, obj client.Object, owner metav1.Object, f controllerutil.MutateFn) error {
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
