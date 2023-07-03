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

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	clusterv1alpha1 "kubeception.ulfo.fr/api/v1alpha1"
)

var (
	clientNamespace = "client-a"
)

var _ = Describe("controlplane controller", Ordered, func() {
	ctx := context.Background()

	BeforeAll(func() {
		By("Creating client namespace")
		ns := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: clientNamespace}}
		Expect(k8sClient.Create(ctx, ns)).Should(Succeed())
	})

	It("Create the control plane CRD", func() {
		crd := &clusterv1alpha1.ControlPlane{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "client-a",
				Namespace: clientNamespace,
			},
			Spec: clusterv1alpha1.ControlPlaneSpec{
				Version: "v1.26.1",
				PKI: clusterv1alpha1.PkiSpec{
					Name: "pki",
					CA: clusterv1alpha1.PKICA{
						Name: "ca",
					},
					Admin: clusterv1alpha1.PKIAdmin{
						Name: "admin",
					},
					ServiceAccounts: clusterv1alpha1.PKIServiceAccounts{
						Name: "service-accounst",
					},
					Konnectivity: clusterv1alpha1.PKIKonnectivity{
						Name: "konnectivity",
					},
					KubeAPIServer: clusterv1alpha1.PKIKubeAPIServer{
						Name: "kube-apiserver",
						IPAddresses: []string{
							"127.0.0.1",
							"10.0.0.1",
							"10.32.0.1",
						},
						DNSNames: []string{
							"localhost",
							"kubernetes",
							"kubernetes.default",
							"kubernetes.default.svc",
							"kubernetes.default.cluster.local",
							"kube-apiserver",
						},
					},
					KubeControllerManager: clusterv1alpha1.PKIKubeControllerManager{
						Name: "kube-controller-manager",
					},
					KubeScheduler: clusterv1alpha1.PKIKubeScheduler{
						Name: "kube-scheduler",
					},
				},
				KubeApiServer: clusterv1alpha1.KubeAPIServerSpec{
					ETCDservers: "etcd:2379",
					Deployment: clusterv1alpha1.Deployment{
						Name:     "kube-apiserver",
						Replicas: 3,
						Labels: map[string]string{
							"client": "a",
						},
					},
				},
				KubeControllerManager: clusterv1alpha1.KubeControllerManagerSpec{
					Deployment: clusterv1alpha1.Deployment{
						Name:     "kube-controller-manager",
						Replicas: 3,
					},
					TLS: clusterv1alpha1.KubeControllerManagerTLS{
						CA:                    "ca",
						KubeControllerManager: "kube-controller-manager",
						ServiceAccountsTLS:    "service-accounts",
					},
					KubeAPIServerService: clusterv1alpha1.Service{
						Name: "kube-apiserver",
						Port: 6443,
					},
				},
				KubeScheduler: clusterv1alpha1.KubeSchedulerSpec{
					KubeAPIServerService: clusterv1alpha1.Service{
						Name: "kube-apiserver",
						Port: 6443,
					},
					KubeSchedulerTls: "kube-scheduler",
					Deployment: clusterv1alpha1.Deployment{
						Name:     "kube-scheduler",
						Replicas: 3,
						Labels: map[string]string{
							"client.cluster": "foo",
						},
					},
				},
			},
		}
		Expect(k8sClient.Create(ctx, crd)).Should(Succeed())

	})

})
