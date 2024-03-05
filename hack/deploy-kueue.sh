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

# Installs a kueue release onto an existing cluster

KUEUE_VERSION=v0.6.0

export ROOT_DIR="$(dirname "$(dirname "$(readlink -fn "$0")")")"

echo "Downloading and patching Kueue ${KUEUE_VERSION} manifests"
wget https://github.com/kubernetes-sigs/kueue/releases/download/${KUEUE_VERSION}/manifests.yaml -O $ROOT_DIR/hack/kueue/kueue-manifest.yaml
patch -p 0 $ROOT_DIR/hack/kueue/kueue-manifest.yaml <  $ROOT_DIR/hack/kueue/patch-01-v0.6.0.txt

echo "Deploying Kueue version $KUEUE_VERSION"
kubectl apply --server-side -f $ROOT_DIR/hack/kueue/kueue-manifest.yaml

# Sleep until the kueue manager is running
echo "Waiting for pods in the kueue-system namespace to become ready"
while [[ $(kubectl get pods -n kueue-system -o 'jsonpath={..status.conditions[?(@.type=="Ready")].status}' | tr ' ' '\n' | sort -u) != "True" ]]
do
    echo -n "." && sleep 1;
done
echo ""

rm -f $ROOT_DIR/hack/kueue/kueue-manifest.yaml

# Define a default local queue in the default namespace
echo "Attempting to define default local queue"

# This won't work until kueue's webhooks are actually configured and working,
# so first sleep for five seconds, then try it in a loop
sleep 5
until kubectl apply -f $ROOT_DIR/hack/kueue/default-queues.yaml
do
    echo -n "." && sleep 1;
done
