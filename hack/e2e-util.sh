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
export KIND_K8S_VERSION=${KIND_K8S_VERSION:-"1.35"}
export KA_BIN=_output/bin
export WAIT_TIME="20s"
export KUTTL_VERSION=0.15.0
DUMP_LOGS="true"

# These must be kept in synch -- we pull and load the image to mitigate dockerhub rate limits
export KUBEFLOW_VERSION=v1.8.1
export IMAGE_KUBEFLOW_OPERATOR="docker.io/kubeflow/training-operator:v1-5170a36"

export KUBERAY_VERSION=1.1.1
export IMAGE_KUBERAY_OPERATOR="quay.io/kuberay/operator:v1.1.1"

export JOBSET_VERSION=v0.11.1
export IMAGE_JOBSET_OPERATOR="registry.k8s.io/jobset/jobset:v0.11.1"

# These are small images used by the e2e tests.
# Pull and kind load to avoid long delays during testing
export IMAGE_ECHOSERVER="quay.io/project-codeflare/echo-server:1.0"
export IMAGE_BUSY_BOX_LATEST="quay.io/project-codeflare/busybox:latest"
export IMAGE_CURL="quay.io/curl/curl:8.11.1"

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
    # Download kind binary (0.33.0)
    echo "Downloading and installing kind v0.33.0...."
    sudo curl -o /usr/local/bin/kind -L https://github.com/kubernetes-sigs/kind/releases/download/v0.33.0/kind-linux-${arch} && \
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
  for image in ${IMAGE_ECHOSERVER} ${IMAGE_BUSY_BOX_LATEST} ${IMAGE_CURL} ${IMAGE_KUBEFLOW_OPERATOR} ${IMAGE_KUBERAY_OPERATOR} ${IMAGE_JOBSET_OPERATOR}
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

    v0.33.0)
      case $KIND_K8S_VERSION in
        1.34)
          KIND_NODE_TAG=${KIND_NODE_TAG:="v1.34.11@sha256:44e222ee2132dab25ff87301682f89eb82c7880ea3a1bf543bfe9708fd08d67d"}
          ;;
        1.35)
          KIND_NODE_TAG=${KIND_NODE_TAG:="v1.35.8@sha256:07b2536e30b803ed61d1677a79df6115f798ce64c80f9e22f6ed45afd09323c0"}
          ;;
        1.36)
          KIND_NODE_TAG=${KIND_NODE_TAG:="v1.36.4@sha256:099e049362a1526b2db71494e1947aae99bd16290d7c895f2b7ea312e3cbfaed"}
          ;;
        1.37)
          KIND_NODE_TAG=${KIND_NODE_TAG:="v1.37.0@sha256:a1ed56cfb0e7b93589bdf97c8cd566405a265939e3620fc4f5de89adff580ae5"}
          ;;
        *)
          echo "Unexpected kubernetes version: $KIND_K8S__VERSION"
          exit 1
          ;;
      esac
      ;;

    v0.32.0)
      case $KIND_K8S_VERSION in
        1.33)
          KIND_NODE_TAG=${KIND_NODE_TAG:="v1.33.12@sha256:3f5c8443c620245e4d355cfe09e96a91ead32ceaa569d3f1ca9edf0cb2fe2ff4"}
          ;;
        1.34)
          KIND_NODE_TAG=${KIND_NODE_TAG:="v1.34.8@sha256:02722c2dedddcfc00febf5d27fbeb9b7b2c14294c82109ff4a85d89ac9ba3256"}
          ;;
        1.35)
          KIND_NODE_TAG=${KIND_NODE_TAG:="v1.35.5@sha256:ce977ae6d65918d0b58a5f8b5e940429c2ce42fa3a5619ec2bbc60b949c0ac95"}
          ;;
        1.36)
          KIND_NODE_TAG=${KIND_NODE_TAG:="v1.36.1@sha256:3489c7674813ba5d8b1a9977baea8a6e553784dab7b84759d1014dbd78f7ebd5"}
          ;;
        *)
          echo "Unexpected kubernetes version: $KIND_K8S__VERSION"
          exit 1
          ;;
      esac
      ;;

    v0.31.0)
      case $KIND_K8S_VERSION in
        1.31)
          KIND_NODE_TAG=${KIND_NODE_TAG:="v1.31.14@sha256:6f86cf509dbb42767b6e79debc3f2c32e4ee01386f0489b3b2be24b0a55aac2b"}
          ;;
        1.32)
          KIND_NODE_TAG=${KIND_NODE_TAG:="v1.32.11@sha256:5fc52d52a7b9574015299724bd68f183702956aa4a2116ae75a63cb574b35af8"}
          ;;
        1.33)
          KIND_NODE_TAG=${KIND_NODE_TAG:="v1.33.7@sha256:d26ef333bdb2cbe9862a0f7c3803ecc7b4303d8cea8e814b481b09949d353040"}
          ;;
        1.34)
          KIND_NODE_TAG=${KIND_NODE_TAG:="v1.34.3@sha256:08497ee19eace7b4b5348db5c6a1591d7752b164530a36f855cb0f2bdcbadd48"}
          ;;
        1.35)
          KIND_NODE_TAG=${KIND_NODE_TAG:="v1.35.0@sha256:452d707d4862f52530247495d180205e029056831160e22870e37e3f6c1ac31f"}
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
  for image in ${IMAGE_ECHOSERVER} ${IMAGE_BUSY_BOX_LATEST} ${IMAGE_CURL} ${IMAGE_KUBEFLOW_OPERATOR} ${IMAGE_KUBERAY_OPERATOR} ${IMAGE_JOBSET_OPERATOR}
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

  echo "Installing JobSet operator version $JOBSET_VERSION"
  kubectl apply --server-side -f "https://github.com/kubernetes-sigs/jobset/releases/download/${JOBSET_VERSION}/manifests.yaml"
  echo "Waiting for pods in the jobset namespace to become ready"
  while [[ $(kubectl get pods -n jobset-system -o 'jsonpath={..status.conditions[?(@.type=="Ready")].status}' | tr ' ' '\n' | sort -u) != "True" ]]
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
