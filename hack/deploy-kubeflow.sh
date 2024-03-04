#!/bin/bash

# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

# Installs the kubeflow operator onto an existing cluster

KUBEFLOW_VERSION=v1.7.0

export ROOT_DIR="$(dirname "$(dirname "$(readlink -fn "$0")")")"

echo "Deploying Kubeflow operator version $KUBEFLOW_VERSION"

kubectl apply -k "github.com/kubeflow/training-operator/manifests/overlays/standalone?ref=$KUBEFLOW_VERSION"

# Sleep until the kubeflow operator is running
echo "Waiting for pods in the kueueflow namespace to become ready"
while [[ $(kubectl get pods -n kubeflow -o 'jsonpath={..status.conditions[?(@.type=="Ready")].status}' | tr ' ' '\n' | sort -u) != "True" ]]
do
    echo -n "." && sleep 1;
done
echo ""
