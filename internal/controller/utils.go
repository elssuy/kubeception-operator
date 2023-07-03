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
	"bytes"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/tools/clientcmd"

	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"
)

func GenerateKubeconfigFromSecret(tls corev1.Secret, host string) ([]byte, error) {
	clientCert := [][]byte{
		tls.Data["tls.crt"],
		tls.Data["ca.crt"],
	}

	config, err := clientcmd.Write(clientcmdapi.Config{
		Clusters: map[string]*clientcmdapi.Cluster{
			"default": {
				Server:                   host,
				CertificateAuthorityData: tls.Data["ca.crt"],
			},
		},
		AuthInfos: map[string]*clientcmdapi.AuthInfo{
			"default-user": {
				ClientCertificateData: bytes.Join(clientCert, []byte("")),
				ClientKeyData:         tls.Data["tls.key"],
			},
		},
		Contexts: map[string]*clientcmdapi.Context{
			"default": {
				Cluster:  "default",
				AuthInfo: "default-user",
			},
		},
		CurrentContext: "default",
	})
	if err != nil {
		return nil, err
	}

	return config, nil

}

func labels(component, instance string, more map[string]string) map[string]string {

	l := map[string]string{
		"app.kubernetes.io/name":     component,
		"app.kubernetes.io/instance": instance,
	}

	for k, v := range more {
		l[k] = v
	}

	return l
}
