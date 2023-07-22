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

	clusterv1alpha1 "github.com/elssuy/kubeception/api/v1alpha1"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

var _ = Describe("KubeApiServer controller", Ordered, func() {
	ctx := context.Background()
	nsName := "kube-apiserver"

	BeforeAll(func() {
		By("Creating client namespace")
		ns := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: nsName}}
		Expect(k8sClient.Create(ctx, ns)).Should(Succeed())
	})

	It("Sync deployment version", func() {

		// Deploy requirements
		ca := GenerateSecret("ca", nsName, map[string]string{"ca.crt": "", "tls.crt": "", "tls.key": ""})
		apiserver := GenerateSecret("apiserver", nsName, map[string]string{"ca.crt": "", "tls.crt": "", "tls.key": ""})
		serviceaccounts := GenerateSecret("service-accounts", nsName, map[string]string{"ca.crt": "", "tls.crt": "", "tls.key": ""})
		konnectivity := GenerateSecret("konnectivity", nsName, map[string]string{"ca.crt": "", "tls.crt": "", "tls.key": ""})

		Expect(k8sClient.Create(ctx, ca)).Should(Succeed())
		Expect(k8sClient.Create(ctx, apiserver)).Should(Succeed())
		Expect(k8sClient.Create(ctx, serviceaccounts)).Should(Succeed())
		Expect(k8sClient.Create(ctx, konnectivity)).Should(Succeed())

		crd := &clusterv1alpha1.KubeAPIServer{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "kube-apiserver",
				Namespace: nsName,
			},
			Spec: clusterv1alpha1.KubeAPIServerSpec{
				Version: "v1.26.1",
				Deployment: clusterv1alpha1.Deployment{
					Replicas: 1,
					Name:     "kube-apiserver",
				},
				ETCDservers: "etcd",
				TLS: clusterv1alpha1.KubeAPIServerTLS{
					CASecretName:              "ca",
					KubeApiServerSecretName:   "apiserver",
					ServiceAccountsSecretName: "service-accounts",
					KonnectivitySecretName:    "konnectivity",
				},
				Options: clusterv1alpha1.KubeAPIServerOptions{
					AdvertiseAddress:      "0.0.0.0",
					ServiceClusterIpRange: "10.0.0.0/24",
				},
			},
		}
		Expect(k8sClient.Create(ctx, crd)).Should(Succeed())

		By("Checking initial deployment image version")
		deployment := &appsv1.Deployment{}
		Eventually(func() bool {
			err := k8sClient.Get(ctx, types.NamespacedName{Name: "kube-apiserver", Namespace: nsName}, deployment)
			if err != nil {
				return false
			}

			for _, v := range deployment.Spec.Template.Spec.Containers {
				if v.Name == "kube-apiserver" &&
					v.Image == "registry.k8s.io/kube-apiserver:v1.26.1" {
					return true
				}
			}
			return false
		}, timeout, interval).Should(BeTrue())

		crd.Spec.Version = "v1.27.1"
		Expect(k8sClient.Update(ctx, crd)).Should(Succeed())

		Eventually(func() bool {
			err := k8sClient.Get(ctx, types.NamespacedName{Name: "kube-apiserver", Namespace: nsName}, deployment)
			if err != nil {
				return false
			}

			for _, v := range deployment.Spec.Template.Spec.Containers {
				if v.Name == "kube-apiserver" &&
					v.Image == "registry.k8s.io/kube-apiserver:v1.27.1" {
					return true
				}
			}
			return false
		}, timeout, interval).Should(BeTrue())
	})

})
