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

type PKICA struct {
	Name string `json:"name,omitempty"`
}

type PKIServiceAccounts struct {
	Name string `json:"name,omitempty"`
}

type PKIAdmin struct {
	Name string `json:"name,omitempty"`
}

type PKIKubeAPIServer struct {
	Name        string   `json:"name,omitempty"`
	IPAddresses []string `json:"IPAddresses,omitempty"`
	DNSNames    []string `json:"DNSNames,omitempty"`
}

type PKIKubeControllerManager struct {
	Name string `json:"name,omitempty"`
}

type PKIKubeScheduler struct {
	Name string `json:"name,omitempty"`
}

type PKIKonnectivity struct {
	Name string `json:"name,omitempty"`
}

// PkiSpec defines the desired state of Pki
type PkiSpec struct {
	Name                  string                   `json:"name,omitempty"`
	ControlPlaneIP        string                   `json:"controlplane-ips,omitempty"`
	CA                    PKICA                    `json:"ca,omitempty"`
	ServiceAccounts       PKIServiceAccounts       `json:"service-accounts,omitempty"`
	Admin                 PKIAdmin                 `json:"admin,omitempty"`
	KubeAPIServer         PKIKubeAPIServer         `json:"kube-apiserver,omitempty"`
	KubeControllerManager PKIKubeControllerManager `json:"kube-controller-manager,omitempty"`
	KubeScheduler         PKIKubeScheduler         `json:"kube-scheduler,omitempty"`
	Konnectivity          PKIKonnectivity          `json:"konnectivity,omitempty"`
}

// PkiStatus defines the observed state of Pki
type PkiStatus struct {
	Ready bool `json:"ready,omitempty"`
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status

// Pki is the Schema for the pkis API
type Pki struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   PkiSpec   `json:"spec,omitempty"`
	Status PkiStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// PkiList contains a list of Pki
type PkiList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Pki `json:"items"`
}

func init() {
	SchemeBuilder.Register(&Pki{}, &PkiList{})
}
