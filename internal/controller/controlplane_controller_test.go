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
	"k8s.io/apimachinery/pkg/types"

	clusterv1alpha1 "github.com/elssuy/kubeception/api/v1alpha1"
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

	It("Keep version in sync", func() {
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

		kas := &clusterv1alpha1.KubeAPIServer{}
		kcm := &clusterv1alpha1.KubeControllerManager{}
		ks := &clusterv1alpha1.KubeScheduler{}

		Eventually(func() bool {
			err := k8sClient.Get(ctx, types.NamespacedName{Name: crd.Name, Namespace: crd.Namespace}, kas)
			return err == nil && kas.Spec.Version == "v1.26.1"
		}, timeout, interval).Should(BeTrue())

		Eventually(func() bool {
			err := k8sClient.Get(ctx, types.NamespacedName{Name: crd.Name, Namespace: crd.Namespace}, kcm)
			return err == nil && kcm.Spec.Version == "v1.26.1"
		}, timeout, interval).Should(BeTrue())

		Eventually(func() bool {
			err := k8sClient.Get(ctx, types.NamespacedName{Name: crd.Name, Namespace: crd.Namespace}, ks)
			return err == nil && ks.Spec.Version == "v1.26.1"
		}, timeout, interval).Should(BeTrue())

		By("Changing main version it keep in sync all components")
		crd.Spec.Version = "v1.27.1"
		Expect(k8sClient.Update(ctx, crd)).Should(Succeed())

		Eventually(func() bool {
			err := k8sClient.Get(ctx, types.NamespacedName{Name: crd.Name, Namespace: crd.Namespace}, kas)
			return err == nil && kas.Spec.Version == "v1.27.1"
		}, timeout, interval).Should(BeTrue())

		Eventually(func() bool {
			err := k8sClient.Get(ctx, types.NamespacedName{Name: crd.Name, Namespace: crd.Namespace}, kcm)
			return err == nil && kcm.Spec.Version == "v1.27.1"
		}, timeout, interval).Should(BeTrue())

		Eventually(func() bool {
			err := k8sClient.Get(ctx, types.NamespacedName{Name: crd.Name, Namespace: crd.Namespace}, ks)
			return err == nil && ks.Spec.Version == "v1.27.1"
		}, timeout, interval).Should(BeTrue())

		By("Updateing one component version it updates it")
		crd.Spec.KubeApiServer.Version = "v1.25.1"
		Expect(k8sClient.Update(ctx, crd)).Should(Succeed())

		Eventually(func() bool {
			err := k8sClient.Get(ctx, types.NamespacedName{Name: crd.Name, Namespace: crd.Namespace}, kas)
			return err == nil && kas.Spec.Version == "v1.25.1"
		}, timeout, interval).Should(BeTrue())

		Eventually(func() bool {
			err := k8sClient.Get(ctx, types.NamespacedName{Name: crd.Name, Namespace: crd.Namespace}, kcm)
			return err == nil && kcm.Spec.Version == "v1.27.1"
		}, timeout, interval).Should(BeTrue())

		Consistently(func() bool {
			err := k8sClient.Get(ctx, types.NamespacedName{Name: crd.Name, Namespace: crd.Namespace}, ks)
			return err == nil && ks.Spec.Version == "v1.27.1"
		}, timeout, interval).Should(BeTrue())

	})

})
