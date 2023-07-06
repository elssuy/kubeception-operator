#!/bin/bash

usage() {
  echo "USAGE: ${0} [Control plane IP] [TOKEN] [CA String]"
  exit 1
}

CONTROLPLANE_IP=$1
TOKEN=$2
CA=$3

if [[ -z ${CONTROLPLANE_IP} ]]; then
  echo "Control plane ip is not set"
  usage
fi

if [[ -z ${TOKEN} ]]; then
  echo "Token is not set"
  usage
fi

if [[ -z ${CA} ]]; then
  echo "CA is not set"
  usage
fi


###################
# Network setup
###################

cat <<EOF | sudo tee /etc/modules-load.d/k8s.conf
overlay
br_netfilter
EOF

sudo modprobe overlay
sudo modprobe br_netfilter

# sysctl params required by setup, params persist across reboots
cat <<EOF | sudo tee /etc/sysctl.d/k8s.conf
net.bridge.bridge-nf-call-iptables  = 1
net.bridge.bridge-nf-call-ip6tables = 1
net.ipv4.ip_forward                 = 1
EOF

# Apply sysctl params without reboot
sudo sysctl --system

###################
# CRI-O Setup
###################

sudo apt update
sudo apt install apt-transport-https ca-certificates curl gnupg2 software-properties-common -y

export OS=xUbuntu_22.04
export CRIO_VERSION=1.24

echo "deb https://download.opensuse.org/repositories/devel:/kubic:/libcontainers:/stable/$OS/ /"| sudo tee /etc/apt/sources.list.d/devel:kubic:libcontainers:stable.list
echo "deb http://download.opensuse.org/repositories/devel:/kubic:/libcontainers:/stable:/cri-o:/$CRIO_VERSION/$OS/ /"|sudo tee /etc/apt/sources.list.d/devel:kubic:libcontainers:stable:cri-o:$CRIO_VERSION.list

curl -L https://download.opensuse.org/repositories/devel:kubic:libcontainers:stable:cri-o:$CRIO_VERSION/$OS/Release.key | sudo apt-key add -
curl -L https://download.opensuse.org/repositories/devel:/kubic:/libcontainers:/stable/$OS/Release.key | sudo apt-key add -

sudo apt update
sudo apt install cri-o cri-o-runc -y

sudo systemctl start crio
sudo systemctl enable crio

# Install Kubelet

sudo apt-get update && sudo apt-get install -y apt-transport-https curl
curl -s https://packages.cloud.google.com/apt/doc/apt-key.gpg | sudo apt-key add -
cat <<EOF | sudo tee /etc/apt/sources.list.d/kubernetes.list
deb https://apt.kubernetes.io/ kubernetes-xenial main
EOF
sudo apt-get update
sudo apt-get install -y kubelet
sudo apt-mark hold kubelet

###################
# Kubelet setup
###################

mkdir -p /var/lib/kubelet
mkdir -p /var/lib/kubelet/pki
mkdir -p /var/lib/kubelet/manifests
mkdir -p /etc/kubernetes

cat<<EOF >> /var/lib/kubelet/kubelet-config.yaml
apiVersion: kubelet.config.k8s.io/v1beta1
kind: KubeletConfiguration
containerRuntimeEndpoint: /run/crio/crio.sock
cgroupDriver: systemd
authentication:
  anonymous:
    enabled: false
  webhook:
    cacheTTL: 0s
    enabled: true
  x509:
    clientCAFile: /var/lib/kubelet/pki/ca.crt
authorization:
  mode: Webhook
  webhook:
    cacheAuthorizedTTL: 0s
    cacheUnauthorizedTTL: 0s
clusterDNS:
  - 10.32.0.2
clusterDomain: cluster.local
rotateCertificates: true
serverTLSBootstrap: false
EOF

cat <<EOF >> /var/lib/kubelet/bootstrap-kubeconfig
apiVersion: v1
clusters:
- cluster:
    certificate-authority: /var/lib/kubelet/pki/ca.crt
    server: https://$CONTROLPLANE_IP:6443
  name: bootstrap
contexts:
- context:
    cluster: bootstrap
    user: kubelet-bootstrap
  name: bootstrap
current-context: bootstrap
kind: Config
preferences: {}
users:
- name: kubelet-bootstrap
  user:
    token: $TOKEN
EOF


cat<<EOF >> /etc/systemd/system/kubelet.service
[Unit]
Description=Kubelet Service
Requires=cri-o.service
After=cri-o.service

[Service]
Restart=always

ExecStart=/usr/bin/kubelet \\
  --config=/var/lib/kubelet/kubelet-config.yaml \\
  --bootstrap-kubeconfig=/var/lib/kubelet/bootstrap-kubeconfig \\
  --kubeconfig=/var/lib/kubelet/kubeconfig


[Install]
WantedBy=multi-user.target
EOF

systemctl daemon-reload

echo "$CA" | base64 -d > /var/lib/kubelet/pki/ca.crt
