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

export LOG_LEVEL=${TEST_LOG_LEVEL:-2}
export CLEANUP_CLUSTER=${CLEANUP_CLUSTER:-"true"}
export CLUSTER_CONTEXT="--name test"
export KIND_OPT=${KIND_OPT:=" --config ${ROOT_DIR}/hack/kind-config.yaml"}
export KA_BIN=_output/bin
export WAIT_TIME="20s"
export KUTTL_VERSION=0.15.0
export CERTMANAGER_VERSION=v1.13.3
DUMP_LOGS="true"

# These must be kept in synch -- we prepull and load these to mitigate dockerhub rate limits
export KUBEFLOW_VERSION=v1.7.0
export IMAGE_KUBEFLOW_OPERATOR="docker.io/kubeflow/training-operator:v1-855e096"

# These are small images used by the e2e tests.
# Pull and kind load to avoid long delays during testing
export IMAGE_ECHOSERVER="quay.io/project-codeflare/echo-server:1.0"
export IMAGE_BUSY_BOX_LATEST="quay.io/project-codeflare/busybox:latest"

function update_test_host {

  local arch="$(go env GOARCH)"
  if [ -z $arch ]
  then
    echo "Unable to determine downloads architecture"
    exit 1
  fi
  echo "CPU architecture for downloads is: ${arch}"

  which curl >/dev/null 2>&1
  if [ $? -ne 0 ]
  then
    echo "curl not installed, exiting."
    exit 1
  fi

  which kubectl >/dev/null 2>&1
  if [ $? -ne 0 ]
  then
      sudo apt-get install -y --allow-unauthenticated kubectl
      [ $? -ne 0 ] && echo "Failed to install kubectl" && exit 1
      echo "kubectl was sucessfully installed."
  fi

  which kind >/dev/null 2>&1
  if [ $? -ne 0 ]
  then
    # Download kind binary (0.19.0)
    echo "Downloading and installing kind...."
    sudo curl -o /usr/local/bin/kind -L https://github.com/kubernetes-sigs/kind/releases/download/v0.19.0/kind-linux-${arch} && \
    sudo chmod +x /usr/local/bin/kind
    [ $? -ne 0 ] && echo "Failed to download kind" && exit 1
    echo "Kind was sucessfully installed."
  fi

  kubectl kuttl version >/dev/null 2>&1
  if [ $? -ne 0 ]
  then
    if [[ "$arch" == "amd64" ]]
    then
      local kuttl_arch="x86_64"
    else
      local kuttl_arch=$arch
    fi
    # Download kuttl plugin
    echo "Downloading and installing kuttl...."
    sudo curl -sSLf --output /tmp/kubectl-kuttl https://github.com/kudobuilder/kuttl/releases/download/v${KUTTL_VERSION}/kubectl-kuttl_${KUTTL_VERSION}_linux_${kuttl_arch} && \
    sudo mv /tmp/kubectl-kuttl /usr/local/bin && \
    sudo chmod a+x /usr/local/bin/kubectl-kuttl
    [ $? -ne 0 ] && echo "Failed to download and install kuttl" && exit 1
    echo "Kuttl was sucessfully installed."
  fi
}

# check if pre-requizites are installed.
function check_prerequisites {
  echo "checking prerequisites"
  which kind >/dev/null 2>&1
  if [ $? -ne 0 ]
  then
    echo "kind not installed, exiting."
    exit 1
  else
    echo -n "found kind, version: " && kind version
  fi

  which kubectl >/dev/null 2>&1
  if [ $? -ne 0 ]
  then
    echo "kubectl not installed, exiting."
    exit 1
  else
    echo -n "found kubectl, " && kubectl version --client
  fi
  kubectl kuttl version >/dev/null 2>&1
  if [ $? -ne 0 ]
  then
    echo "kuttl plugin for kubectl not installed, exiting."
    exit 1
  else
    echo -n "found kuttl plugin for kubectl, " && kubectl kuttl version
  fi
}

function pull_images {
  for image in ${IMAGE_ECHOSERVER} ${IMAGE_BUSY_BOX_LATEST} ${IMAGE_KUBEFLOW_OPERATOR}
  do
      docker pull $image
      if [ $? -ne 0 ]
      then
          echo "Failed to pull $image"
          exit 1
      fi
  done

  docker images
}

function kind_up_cluster {
  echo "Running kind: [kind create cluster ${CLUSTER_CONTEXT} ${KIND_OPT}]"
  kind create cluster ${CLUSTER_CONTEXT} ${KIND_OPT} --wait ${WAIT_TIME}
  if [ $? -ne 0 ]
  then
    echo "Failed to start kind cluster"
    exit 1
  fi
  CLUSTER_STARTED="true"

  for image in ${IMAGE_ECHOSERVER} ${IMAGE_BUSY_BOX_LATEST} ${IMAGE_KUBEFLOW_OPERATOR}
  do
    kind load docker-image ${image} ${CLUSTER_CONTEXT}
    if [ $? -ne 0 ]
    then
      echo "Failed to load image ${image} in cluster"
      exit 1
    fi
  done
}

function configure_cluster {
  echo "Installing cert-manager"
  kubectl apply -f https://github.com/cert-manager/cert-manager/releases/download/$CERTMANAGER_VERSION/cert-manager.yaml

  # sleep to ensure cert-manager is fully functional
  echo "Waiting for pod in the cert-manager namespace to become ready"
  while [[ $(kubectl get pods -n cert-manager -o 'jsonpath={..status.conditions[?(@.type=="Ready")].status}' | tr ' ' '\n' | sort -u) != "True" ]]
  do
    echo -n "." && sleep 1;
  done
  echo ""

  echo "Installing Kubeflow operator version $KUBEFLOW_VERSION"
  kubectl apply -k "github.com/kubeflow/training-operator/manifests/overlays/standalone?ref=$KUBEFLOW_VERSION"

  # Sleep until the kubeflow operator is running
  echo "Waiting for pods in the kueueflow namespace to become ready"
  while [[ $(kubectl get pods -n kubeflow -o 'jsonpath={..status.conditions[?(@.type=="Ready")].status}' | tr ' ' '\n' | sort -u) != "True" ]]
  do
      echo -n "." && sleep 1;
  done
  echo ""
}

function wait_for_appwrapper_controller {
    # Sleep until the appwrapper controller is running
    echo "Waiting for pods in the appwrapper-system namespace to become ready"
    while [[ $(kubectl get pods -n appwrapper-system -o 'jsonpath={..status.conditions[?(@.type=="Ready")].status}' | tr ' ' '\n' | sort -u) != "True" ]]
    do
        echo -n "." && sleep 1;
    done
    echo ""
}

# clean up
function cleanup {
    echo "==========================>>>>> Cleaning up... <<<<<=========================="
    echo " "
    if [[ ${CLUSTER_STARTED} == "false" ]]
    then
      echo "Cluster was not started, nothing more to do."
      return
    fi

    if [[ ${DUMP_LOGS} == "true" ]]
    then

      echo "Custom Resource Definitions..."
      echo "kubectl get crds"
      kubectl get crds

      echo "---"
      echo "Get All AppWrappers..."
      kubectl get appwrappers --all-namespaces -o yaml

      echo "---"
      echo "Describe all AppWrappers..."
      kubectl describe appwrappers --all-namespaces

      echo "---"
      echo "'test' Pod list..."
      kubectl get pods -n test

      echo "---"
      echo "'test' Pod yaml..."
      kubectl get pods -n test -o yaml

      echo "---"
      echo "'test' Pod descriptions..."
      kubectl describe pods -n test

      echo "---"
      echo "'all' Namespaces  list..."
      kubectl get namespaces

      local appwrapper_controller_pod=$(kubectl get pods -n appwrapper-system | grep appwrapper-controller | awk '{print $1}')
      if [[ "$appwrapper_controller_pod" != "" ]]
      then
        echo "===================================================================================="
        echo "==========================>>>>> AppWrapper Controller Logs <<<<<=========================="
        echo "===================================================================================="
        echo "kubectl logs ${appwrapper_controller_pod} -n appwrapper-system"
        kubectl logs ${appwrapper_controller_pod} -n appwrapper-system
      fi
    fi

    rm -f kubeconfig

    if [[ $CLEANUP_CLUSTER == "true" ]]
    then
      kind delete cluster ${CLUSTER_CONTEXT}
    else
      echo "Cluster requested to stay up, not deleting cluster"
    fi
}

function run_kuttl_test_suite {
  for kuttl_test in ${KUTTL_TEST_SUITES[@]}; do
    echo "kubectl kuttl test --config ${kuttl_test}"
    kubectl kuttl test --config ${kuttl_test}
    if [ $? -ne 0 ]
    then
      echo "kuttl e2e test '${kuttl_test}' failure, exiting."
      exit 1
    fi
  done
}
