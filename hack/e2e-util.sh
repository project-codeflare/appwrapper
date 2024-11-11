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
export CLUSTER_CONTEXT=${CLUSTER_CONTEXT:-"--name test"}
export KIND_OPT=${KIND_OPT:=" --config ${ROOT_DIR}/hack/kind-config.yaml"}
export KIND_K8S_VERSION=${KIND_K8S_VERSION:-"1.27"}
export KA_BIN=_output/bin
export WAIT_TIME="20s"
export KUTTL_VERSION=0.15.0
DUMP_LOGS="true"

# These must be kept in synch -- we pull and load the image to mitigate dockerhub rate limits
export KUBEFLOW_VERSION=v1.7.0
export IMAGE_KUBEFLOW_OPERATOR="docker.io/kubeflow/training-operator:v1-855e096"

export KUBERAY_VERSION=1.1.0
export IMAGE_KUBERAY_OPERATOR="quay.io/kuberay/operator:v1.1.1"

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
    # Download kind binary (0.25.0)
    echo "Downloading and installing kind v0.25.0...."
    sudo curl -o /usr/local/bin/kind -L https://github.com/kubernetes-sigs/kind/releases/download/v0.25.0/kind-linux-${arch} && \
    sudo chmod +x /usr/local/bin/kind
    [ $? -ne 0 ] && echo "Failed to download kind" && exit 1
    echo "Kind was sucessfully installed."
  fi

  which helm >/dev/null 2>&1
  if [ $? -ne 0 ]
  then
    # Installing helm3
    echo "Downloading and installing helm..."
    curl -fsSL -o ${ROOT_DIR}/get_helm.sh https://raw.githubusercontent.com/helm/helm/main/scripts/get-helm-3 &&
      chmod 700 ${ROOT_DIR}/get_helm.sh && ${ROOT_DIR}/get_helm.sh
    [ $? -ne 0 ] && echo "Failed to download and install helm" && exit 1
    echo "Helm was sucessfully installed."
    rm -rf ${ROOT_DIR}/get_helm.sh
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

  which helm >/dev/null 2>&1
  if [ $? -ne 0 ]
  then
    echo "helm not installed, exiting."
    exit 1
  else
    echo -n "found helm, " && helm version
  fi
}

function pull_images {
  for image in ${IMAGE_ECHOSERVER} ${IMAGE_BUSY_BOX_LATEST} ${IMAGE_KUBEFLOW_OPERATOR} ${IMAGE_KUBERAY_OPERATOR}
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
  # Determine node image tag based on kind version and desired kubernetes version
  KIND_ACTUAL_VERSION=$(kind version | awk '/ /{print $2}')
  case $KIND_ACTUAL_VERSION in
    v0.25.0)
      case $KIND_K8S_VERSION in
        1.27)
          KIND_NODE_TAG=${KIND_NODE_TAG:="v1.27.16@sha256:2d21a61643eafc439905e18705b8186f3296384750a835ad7a005dceb9546d20"}
          ;;
        1.29)
          KIND_NODE_TAG=${KIND_NODE_TAG:="v1.29.10@sha256:3b2d8c31753e6c8069d4fc4517264cd20e86fd36220671fb7d0a5855103aa84b"}
          ;;
        1.30)
          KIND_NODE_TAG=${KIND_NODE_TAG:="v1.30.6@sha256:b6d08db72079ba5ae1f4a88a09025c0a904af3b52387643c285442afb05ab994"}
          ;;
        1.31)
          KIND_NODE_TAG=${KIND_NODE_TAG:="v1.31.2@sha256:18fbefc20a7113353c7b75b5c869d7145a6abd6269154825872dc59c1329912e"}
          ;;
        *)
          echo "Unexpected kubernetes version: $KIND_K8S__VERSION"
          exit 1
          ;;
      esac
      ;;

    v0.24.0)
      case $KIND_K8S_VERSION in
        1.27)
          KIND_NODE_TAG=${KIND_NODE_TAG:="v1.27.16@sha256:3fd82731af34efe19cd54ea5c25e882985bafa2c9baefe14f8deab1737d9fabe"}
          ;;
        1.29)
          KIND_NODE_TAG=${KIND_NODE_TAG:="v1.29.8@sha256:d46b7aa29567e93b27f7531d258c372e829d7224b25e3fc6ffdefed12476d3aa"}
          ;;
        1.30)
          KIND_NODE_TAG=${KIND_NODE_TAG:="v1.30.4@sha256:976ea815844d5fa93be213437e3ff5754cd599b040946b5cca43ca45c2047114"}
          ;;
        1.31)
          KIND_NODE_TAG=${KIND_NODE_TAG:="v1.31.0@sha256:53df588e04085fd41ae12de0c3fe4c72f7013bba32a20e7325357a1ac94ba865"}
          ;;
        *)
          echo "Unexpected kubernetes version: $KIND_K8S__VERSION"
          exit 1
          ;;
      esac
      ;;

    *)
      echo "Unexpected kind version: $KIND_ACTUAL_VERSION"
      exit 1
      ;;
  esac

  echo "Running kind: [kind create cluster ${CLUSTER_CONTEXT} --image kindest/node:${KIND_NODE_TAG} ${KIND_OPT}]"
  kind create cluster ${CLUSTER_CONTEXT} --image kindest/node:${KIND_NODE_TAG} ${KIND_OPT} --wait ${WAIT_TIME}
  if [ $? -ne 0 ]
  then
    echo "Failed to start kind cluster"
    exit 1
  fi
  CLUSTER_STARTED="true"
}

function kind_load_images {
  for image in ${IMAGE_ECHOSERVER} ${IMAGE_BUSY_BOX_LATEST} ${IMAGE_KUBEFLOW_OPERATOR} ${IMAGE_KUBERAY_OPERATOR}
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
  echo "Installing Kubeflow operator version $KUBEFLOW_VERSION"
  kubectl apply -k "github.com/kubeflow/training-operator/manifests/overlays/standalone?ref=$KUBEFLOW_VERSION"
  echo "Waiting for pods in the kubeflow namespace to become ready"
  while [[ $(kubectl get pods -n kubeflow -o 'jsonpath={..status.conditions[?(@.type=="Ready")].status}' | tr ' ' '\n' | sort -u) != "True" ]]
  do
      echo -n "." && sleep 1;
  done
  echo ""

  echo "Installing Kuberay operator version $KUBERAY_VERSION"
  helm install kuberay-operator kuberay-operator --repo https://ray-project.github.io/kuberay-helm/ --version $KUBERAY_VERSION --create-namespace -n kuberay-system
  echo "Waiting for pods in the kuberay namespace to become ready"
  while [[ $(kubectl get pods -n kuberay-system -o 'jsonpath={..status.conditions[?(@.type=="Ready")].status}' | tr ' ' '\n' | sort -u) != "True" ]]
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

function add_virtual_GPUs {
    # Patch nodes to provide GPUs resources without physical GPUs.
    # This enables testing of our autopilot integration.
    echo "Adding virtual GPUs to all nodes"
    for node_name in $(kubectl get nodes --no-headers -o custom-columns=":metadata.name")
    do
        kubectl patch node $node_name --subresource=status --type=json -p='[{"op":"add","path":"/status/capacity/nvidia.com~1gpu","value":"8"}]'
    done
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
      kubectl get pods -n e2e-test

      echo "---"
      echo "'test' Pod yaml..."
      kubectl get pods -n e2e-test -o yaml

      echo "---"
      echo "'test' Pod descriptions..."
      kubectl describe pods -n e2e-test

      echo "---"
      echo "'all' Namespaces  list..."
      kubectl get namespaces

      local appwrapper_controller_pod=$(kubectl get pods -n appwrapper-system | grep appwrapper-controller | awk '{print $1}')
      if [[ "$appwrapper_controller_pod" != "" ]]
      then
        echo "===================================================================================="
        echo "======================>>>>> AppWrapper Controller Logs <<<<<========================"
        echo "===================================================================================="
        echo "kubectl logs ${appwrapper_controller_pod} -n appwrapper-system"
        kubectl logs ${appwrapper_controller_pod} -n appwrapper-system
      fi

      local kueue_controller_pod=$(kubectl get pods -n kueue-system | grep kueue-controller | awk '{print $1}')
      if [[ "$kueue_controller_pod" != "" ]]
      then
        echo "===================================================================================="
        echo "=========================>>>>> Kueue Controller Logs <<<<<=========================="
        echo "===================================================================================="
        echo "kubectl logs ${kueue_controller_pod} -n kueue-system"
        kubectl logs ${kueue_controller_pod} -n kueue-system
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
