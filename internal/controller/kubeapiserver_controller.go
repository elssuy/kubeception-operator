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
	"strconv"
	"time"

	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/manager"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	clusterv1alpha1 "github.com/elssuy/kubeception-operator/api/v1alpha1"
)

// KubeAPIServerReconciler reconciles a KubeAPIServer object
type KubeAPIServerReconciler struct {
	client.Client
	Scheme *runtime.Scheme
	log    logr.Logger
}

func NewKubeAPIServerReconciler(mgr manager.Manager) *KubeAPIServerReconciler {
	return &KubeAPIServerReconciler{
		Client: mgr.GetClient(),
		Scheme: mgr.GetScheme(),
		log:    log.Log.WithName("kube-apiserver-controller"),
	}
}

//+kubebuilder:rbac:groups=cluster.kubeception.ulfo.fr,resources=kubeapiservers,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=cluster.kubeception.ulfo.fr,resources=kubeapiservers/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=cluster.kubeception.ulfo.fr,resources=kubeapiservers/finalizers,verbs=update
//+kubebuilder:rbac:groups=core,resources=configmaps,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=core,resources=secrets,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=apps,resources=deployments,verbs=get;list;watch;create;update;patch;delete

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the KubeAPIServer object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.14.4/pkg/reconcile
func (r *KubeAPIServerReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {

	kas := &clusterv1alpha1.KubeAPIServer{}
	if err := r.Get(ctx, req.NamespacedName, kas); err != nil {
		if apierrors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}

		r.log.Error(err, "failed to get ApiServer resource", "name", req.Name, "namespace", req.Namespace)
		return ctrl.Result{}, err
	}

	konnectivityCertSecret := &corev1.Secret{}
	if err := r.Get(ctx, types.NamespacedName{Namespace: req.Namespace, Name: kas.Spec.TLS.KonnectivitySecretName}, konnectivityCertSecret); err != nil {
		r.log.Info("Konnectivity certificate secret not found retrying later", "name", kas.Spec.TLS.KonnectivitySecretName, "namespace", req.Namespace)
		return ctrl.Result{RequeueAfter: 3 * time.Second}, nil
	}

	konnectivityKubeconfigName := fmt.Sprintf("%s-kubeconfig", kas.Spec.TLS.KonnectivitySecretName)
	konnectivityKubeconfig := &corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: konnectivityKubeconfigName, Namespace: req.Namespace}}
	err := r.CreateOrPatch(ctx, konnectivityKubeconfig, kas, func() error {
		k, err := GenerateKubeconfigFromSecret(
			*konnectivityCertSecret,
			"https://kube-apiserver:6443",
		)
		if err != nil {
			r.log.Error(err, "failed to marshal konnectivity kubeconfig", "name", konnectivityKubeconfigName, "namespace", req.Namespace)
			return err
		}
		konnectivityKubeconfig.Data = make(map[string][]byte)
		konnectivityKubeconfig.Data["kubeconfig.yml"] = k
		return nil
	})
	if err != nil {
		r.log.Error(err, "failed to create or patch konnectivity Kubeconfig", "name", konnectivityKubeconfig.Name, "namespace", konnectivityKubeconfig.Namespace)
		return ctrl.Result{}, err
	}

	// Setup Konnectivity
	konnectivityEgressConfig := &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: "konnectivity-egress", Namespace: req.Namespace}}
	err = r.CreateOrPatch(ctx, konnectivityEgressConfig, kas, func() error {
		konnectivityEgressConfig.Data = map[string]string{
			"egress.yaml": `apiVersion: apiserver.k8s.io/v1beta1
kind: EgressSelectorConfiguration
egressSelections:
# Since we want to control the egress traffic to the cluster, we use the
# "cluster" as the name. Other supported values are "etcd", and "controlplane".
- name: cluster
  connection:
    # This controls the protocol between the API Server and the Konnectivity
    # server. Supported values are "GRPC" and "HTTPConnect". There is no
    # end user visible difference between the two modes. You need to set the
    # Konnectivity server to work in the same mode.
    proxyProtocol: GRPC
    transport:
      # This controls what transport the API Server uses to communicate with the
      # Konnectivity server. UDS is recommended if the Konnectivity server
      # locates on the same machine as the API Server. You need to configure the
      # Konnectivity server to listen on the same UDS socket.
      # The other supported transport is "tcp". You will need to set up TLS
      # config to secure the TCP transport.
      uds:
        udsName: /etc/kubernetes/konnectivity-socket/konnectivity-server.socket
`,
		}
		return nil
	})
	if err != nil {
		r.log.Error(err, "failed to create or patch konnectivity EgressConfig", "name", konnectivityEgressConfig.Name, "namespace", konnectivityEgressConfig.Namespace)
		return ctrl.Result{}, err
	}

	// Check CA
	if err := r.Get(ctx, types.NamespacedName{Name: kas.Spec.TLS.CASecretName, Namespace: req.Namespace}, &corev1.Secret{}); err != nil {
		r.log.Info("failed to get secret for CA, requeing", "name", kas.Spec.TLS.CASecretName, "namespace", req.Namespace)
		return ctrl.Result{RequeueAfter: 3 * time.Second}, nil
	}

	// Check APIServer
	kubeapiserverCertSecret := &corev1.Secret{}
	if err := r.Get(ctx, types.NamespacedName{Name: kas.Spec.TLS.KubeApiServerSecretName, Namespace: req.Namespace}, kubeapiserverCertSecret); err != nil {
		r.log.Info("failed to get secret for APIServer TLS Cert, requeing", "name", kas.Spec.TLS.KubeApiServerSecretName, "namespace", req.Namespace)
		return ctrl.Result{RequeueAfter: 3 * time.Second}, nil
	}

	// Check ServiceAccounts
	if err := r.Get(ctx, types.NamespacedName{Name: kas.Spec.TLS.ServiceAccountsSecretName, Namespace: req.Namespace}, &corev1.Secret{}); err != nil {
		r.log.Info("failed to get secret for Service Account TLS Cert, requeing", "name", kas.Spec.TLS.ServiceAccountsSecretName, "namespace", req.Namespace)
		return ctrl.Result{RequeueAfter: 3 * time.Second}, nil
	}

	// Deployment APIServer & Konnectivity
	foundDeployment := &appsv1.Deployment{ObjectMeta: metav1.ObjectMeta{Name: kas.Spec.Deployment.Name, Namespace: req.Namespace}}
	err = r.CreateOrPatch(ctx, foundDeployment, kas, func() error {

		deployment := r.GenerateDeployment(*kas)
		foundDeployment.Labels = deployment.Labels
		foundDeployment.Spec = deployment.Spec

		return nil
	})
	if err != nil {
		r.log.Error(err, "failed to create or patch KubeAPIServer deployment", "name", foundDeployment.Name, "namespace", foundDeployment.Namespace)
		return ctrl.Result{}, err
	}

	// TODO: create a specific controller for this ! with admin access
	// Deploy APIServer RBAC to remote control plane
	// kubeconfig, err := utils.GenerateKubeconfigFromSecret("kube-apiserver", "kube-system", *kubeapiserverCertSecret, fmt.Sprintf("https://%s:6443", "51.159.205.41"))
	// if err != nil {
	// 	r.log.Error(err, "failed to create kubeconfig from kube-apiserver cert secret")
	// 	return ctrl.Result{}, err
	// }
	// rconfig, err := clientcmd.RESTConfigFromKubeConfig(kubeconfig.Data["kubeconfig.yml"])
	// if err != nil {
	// 	r.log.Error(err, "failed to create kube config for rbac deployment")
	// 	return ctrl.Result{}, err
	// }

	// rclient, err := kubernetes.NewForConfig(rconfig)
	// if err != nil {
	// 	r.log.Error(err, "failed to create kube client for rbac deployment")
	// 	return ctrl.Result{}, err
	// }

	// apigroup := "rbac.authorization.k8s.io"
	// kindrole := "ClusterRole"
	// kindsubject := "User"
	// namerole := "system:kubelet-api-admin"
	// namesubject := "kube-apiserver"

	// clusterrolebinding := v1.ClusterRoleBinding("system:kube-apiserver").
	// 	WithRoleRef(&v1.RoleRefApplyConfiguration{
	// 		APIGroup: &apigroup,
	// 		Kind:     &kindrole,
	// 		Name:     &namerole,
	// 	}).WithSubjects(
	// 	v1.Subject().WithAPIGroup(apigroup).WithKind(kindsubject).WithName(namesubject),
	// )
	// result, err := rclient.RbacV1().ClusterRoleBindings().Apply(ctx, clusterrolebinding, metav1.ApplyOptions{})
	// if err != nil {
	// 	r.log.Error(err, "failed to apply rbac for apiserver")
	// 	return ctrl.Result{RequeueAfter: 3 * time.Second}, nil
	// }
	// r.log.Info("Rabc was %s", result)

	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *KubeAPIServerReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&clusterv1alpha1.KubeAPIServer{}).
		Owns(&appsv1.Deployment{}).
		Owns(&corev1.ConfigMap{}).
		Complete(r)
}

func (r *KubeAPIServerReconciler) GenerateDeployment(kas clusterv1alpha1.KubeAPIServer) appsv1.Deployment {
	konnectivityKubeconfigName := fmt.Sprintf("%s-kubeconfig", kas.Spec.TLS.KonnectivitySecretName)

	return appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      kas.Spec.Deployment.Name,
			Namespace: kas.Namespace,
			Labels:    labels("kube-apiserver", kas.Name, kas.Spec.Deployment.Labels),
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: &kas.Spec.Deployment.Replicas,
			Strategy: appsv1.DeploymentStrategy{
				RollingUpdate: &appsv1.RollingUpdateDeployment{
					MaxUnavailable: &intstr.IntOrString{IntVal: 1},
				},
			},
			Selector: &metav1.LabelSelector{
				MatchLabels: labels("kube-apiserver", kas.Name, kas.Spec.Deployment.Labels),
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: labels("kube-apiserver", kas.Name, kas.Spec.Deployment.Labels),
				},
				Spec: corev1.PodSpec{
					Volumes: []corev1.Volume{
						{Name: "ca", VolumeSource: corev1.VolumeSource{Secret: &corev1.SecretVolumeSource{SecretName: kas.Spec.TLS.CASecretName}}},
						{Name: "service-accounts", VolumeSource: corev1.VolumeSource{Secret: &corev1.SecretVolumeSource{SecretName: kas.Spec.TLS.ServiceAccountsSecretName}}},
						{Name: "kube-apiserver", VolumeSource: corev1.VolumeSource{Secret: &corev1.SecretVolumeSource{SecretName: kas.Spec.TLS.KubeApiServerSecretName}}},
						{Name: "konnectivity-egress", VolumeSource: corev1.VolumeSource{ConfigMap: &corev1.ConfigMapVolumeSource{LocalObjectReference: corev1.LocalObjectReference{Name: "konnectivity-egress"}}}},
						{Name: "konnectivity-kubeconfig", VolumeSource: corev1.VolumeSource{Secret: &corev1.SecretVolumeSource{SecretName: konnectivityKubeconfigName}}},
						{Name: "konnectivity-socket", VolumeSource: corev1.VolumeSource{EmptyDir: &corev1.EmptyDirVolumeSource{}}},
					},
					Containers: []corev1.Container{
						{
							Name:    "konnectivity",
							Image:   "registry.k8s.io/kas-network-proxy/proxy-server:v0.0.37",
							Command: []string{"/proxy-server"},
							Args: []string{
								"--logtostderr=true",
								"--uds-name=/etc/kubernetes/konnectivity-socket/konnectivity-server.socket",
								"--delete-existing-uds-file",
								"--cluster-cert=/etc/kubernetes/tls/tls.crt",
								"--cluster-key=/etc/kubernetes/tls/tls.key",
								"--mode=grpc",
								"--server-port=0",
								"--agent-namespace=kube-system",
								"--agent-service-account=konnectivity-agent",
								"--server-count",
								strconv.FormatInt(int64(kas.Spec.Deployment.Replicas), 10),
								"--kubeconfig=/etc/kubernetes/konnectivity/kubeconfig.yml",
								"--authentication-audience=system:konnectivity-server",
							},
							Ports: []corev1.ContainerPort{
								{Name: "grpc", ContainerPort: 8091},
								{Name: "admin", ContainerPort: 8095},
								{Name: "health", ContainerPort: 8092},
							},
							VolumeMounts: []corev1.VolumeMount{
								{Name: "konnectivity-socket", MountPath: "/etc/kubernetes/konnectivity-socket"},
								{Name: "konnectivity-kubeconfig", MountPath: "/etc/kubernetes/konnectivity"},
								{Name: "kube-apiserver", MountPath: "/etc/kubernetes/tls"},
							},
							LivenessProbe: &corev1.Probe{
								InitialDelaySeconds: 10,
								TimeoutSeconds:      15,
								FailureThreshold:    8,
								PeriodSeconds:       10,
								SuccessThreshold:    1,
								ProbeHandler: corev1.ProbeHandler{
									HTTPGet: &corev1.HTTPGetAction{
										Port:   intstr.FromInt(8092),
										Path:   "/healthz",
										Scheme: corev1.URISchemeHTTP,
									},
								},
							},
						},
						{
							Name:  "kube-apiserver",
							Image: fmt.Sprintf("registry.k8s.io/kube-apiserver:%s", kas.Spec.Version),
							Command: []string{
								"/usr/local/bin/kube-apiserver",
								"--advertise-address",
								kas.Spec.Options.AdvertiseAddress,
								"--allow-privileged",
								"--runtime-config",
								"api/all=true",
								"--service-cluster-ip-range",
								kas.Spec.Options.ServiceClusterIpRange,
								"--authorization-mode",
								"RBAC,Node",
								"--client-ca-file",
								"/var/lib/kubernetes/tls/kube-apiserver/ca.crt",
								"--etcd-servers",
								kas.Spec.ETCDservers,
								"--service-account-issuer",
								"https://kube-apiserver",
								"--service-account-key-file",
								"/var/lib/kubernetes/tls/sa/tls.crt",
								"--service-account-signing-key-file",
								"/var/lib/kubernetes/tls/sa/tls.key",
								"--tls-cert-file",
								"/var/lib/kubernetes/tls/kube-apiserver/tls.crt",
								"--tls-private-key-file",
								"/var/lib/kubernetes/tls/kube-apiserver/tls.key",
								"--egress-selector-config-file",
								"/etc/kubernetes/konnectivity-egress/egress.yaml",
								"--enable-bootstrap-token-auth",
								"--enable-admission-plugins",
								"NamespaceLifecycle,NodeRestriction,LimitRanger,ServiceAccount,DefaultStorageClass,ResourceQuota",
								"--kubelet-preferred-address-types",
								"ExternalIP,InternalIP,Hostname",
								"--kubelet-client-certificate",
								"/var/lib/kubernetes/tls/kube-apiserver/tls.crt",
								"--kubelet-client-key",
								"/var/lib/kubernetes/tls/kube-apiserver/tls.key",
								"--requestheader-client-ca-file=/var/lib/kubernetes/tls/kube-apiserver/ca.crt",
								"--requestheader-allowed-names=front-proxy-client",
								"--requestheader-extra-headers-prefix=X-Remote-Extra-",
								"--requestheader-group-headers=X-Remote-Group",
								"--requestheader-username-headers=X-Remote-User",
								"--proxy-client-cert-file=/var/lib/kubernetes/tls/kube-apiserver/tls.crt",
								"--proxy-client-key-file=/var/lib/kubernetes/tls/kube-apiserver/tls.key",
							},
							Ports: []corev1.ContainerPort{
								{Name: "https", ContainerPort: 6443},
							},
							VolumeMounts: []corev1.VolumeMount{
								{Name: "service-accounts", MountPath: "/var/lib/kubernetes/tls/sa"},
								{Name: "kube-apiserver", MountPath: "/var/lib/kubernetes/tls/kube-apiserver"},
								{Name: "konnectivity-socket", MountPath: "/etc/kubernetes/konnectivity-socket"},
								{Name: "konnectivity-egress", MountPath: "/etc/kubernetes/konnectivity-egress"},
							},
							LivenessProbe: &corev1.Probe{
								InitialDelaySeconds: 10,
								TimeoutSeconds:      15,
								FailureThreshold:    8,
								PeriodSeconds:       10,
								ProbeHandler: corev1.ProbeHandler{
									HTTPGet: &corev1.HTTPGetAction{
										Port:   intstr.FromInt(6443),
										Path:   "/livez",
										Scheme: corev1.URISchemeHTTPS,
									},
								},
							},
							ReadinessProbe: &corev1.Probe{
								InitialDelaySeconds: 10,
								TimeoutSeconds:      15,
								FailureThreshold:    8,
								PeriodSeconds:       10,
								ProbeHandler: corev1.ProbeHandler{
									HTTPGet: &corev1.HTTPGetAction{
										Port:   intstr.FromInt(6443),
										Path:   "/readyz",
										Scheme: corev1.URISchemeHTTPS,
									},
								},
							},
						},
					},
				},
			},
		},
	}
}
