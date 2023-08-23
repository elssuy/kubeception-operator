#!/bin/bash

IP_RANGE=${1}

usage() {
  echo "Usage: ${0} [IP Range]"
  echo "Install MetalLB with L2 Advertisement and an IPAddressPool"
  SUBNET=$(docker network inspect kind -f "{{(index .IPAM.Config 0).Subnet}}")
  echo "Your kind subnet is: $SUBNET"
}

if [[ -z ${IP_RANGE} ]]; then
  echo "Error: Please indicate an IP range"
  echo
  usage
  exit 1
fi

kubectl apply -f https://raw.githubusercontent.com/metallb/metallb/v0.13.7/config/manifests/metallb-native.yaml

kubectl -n metallb-system wait deploy/controller --for=condition=Available=True
sleep 5

kubectl apply -f - <<EOF
apiVersion: metallb.io/v1beta1
kind: IPAddressPool
metadata:
  name: localhost
  namespace: metallb-system
spec:
  addresses:
  - $IP_RANGE
---
apiVersion: metallb.io/v1beta1
kind: L2Advertisement
metadata:
  name: empty
  namespace: metallb-system
EOF
