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

package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

type Deployment struct {
	Name     string            `json:"name,omitempty"`
	Replicas int32             `json:"replicas,omitempty"`
	Labels   map[string]string `json:"labels,omitempty"`
}

type Service struct {
	Name string `json:"name,omitempty"`
	Port uint   `json:"port,omitempty"`
}

type KubeAPIServerTLS struct {
	CASecretName              string `json:"ca-secret-name,omitempty"`
	KubeApiServerSecretName   string `json:"kube-apiserver-secret-name,omitempty"`
	ServiceAccountsSecretName string `json:"service-accounts-secret-name,omitempty"`
	KonnectivitySecretName    string `json:"konnectivity-secret-name,omitempty"`
}

type KubeAPIServerOptions struct {
	AdvertiseAddress      string `json:"advertise-address,omitempty"`
	ServiceClusterIpRange string `json:"service-cluster-ip-range,omitempty"`
}

// KubeAPIServerSpec defines the desired state of KubeAPIServer
type KubeAPIServerSpec struct {
	// INSERT ADDITIONAL SPEC FIELDS - desired state of cluster
	// Important: Run "make" to regenerate code after modifying this file

	Version string `json:"version,omitempty"`

	ETCDservers string `json:"etcd-servers,omitempty"`

	Deployment Deployment `json:"deployment,omitempty"`

	TLS KubeAPIServerTLS `json:"tls,omitempty"`

	Options KubeAPIServerOptions `json:"options,omitempty"`
}

// KubeAPIServerStatus defines the observed state of KubeAPIServer
type KubeAPIServerStatus struct {
	// INSERT ADDITIONAL STATUS FIELD - define observed state of cluster
	// Important: Run "make" to regenerate code after modifying this file
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status
//+kubebuilder:resource:shortName=kas

// KubeAPIServer is the Schema for the kubeapiservers API
type KubeAPIServer struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   KubeAPIServerSpec   `json:"spec,omitempty"`
	Status KubeAPIServerStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// KubeAPIServerList contains a list of KubeAPIServer
type KubeAPIServerList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []KubeAPIServer `json:"items"`
}

func init() {
	SchemeBuilder.Register(&KubeAPIServer{}, &KubeAPIServerList{})
}
