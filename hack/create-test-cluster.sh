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

# Create and configure a kind cluster for running the e2e tests
# Does NOT install mcad

export ROOT_DIR="$(dirname "$(dirname "$(readlink -fn "$0")")")"
CLUSTER_STARTED="false"

source ${ROOT_DIR}/hack/e2e-util.sh

update_test_host
check_prerequisites
pull_images
kind_up_cluster
configure_cluster
