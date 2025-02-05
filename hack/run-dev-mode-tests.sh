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

# Run backgrounded dev mode controller with e2e tests in foreground

export ROOT_DIR="$(dirname "$(dirname "$(readlink -fn "$0")")")"
export GORACE=1
export CLEANUP_CLUSTER=${CLEANUP_CLUSTER:-"false"}
export CLUSTER_STARTED="true"

source ${ROOT_DIR}/hack/e2e-util.sh

trap cleanup EXIT

# run background_services_gpid test_command
run () {
    PID=$1
    shift
    CODE=0
    "$@" || CODE=$?
    kill -- -$PID || true
    sleep 1
    return $CODE
}

NAMESPACE=dev go run ./cmd/main.go &
run $! go run github.com/onsi/ginkgo/v2/ginkgo -v -fail-fast --procs 1 -timeout 130m --label-filter=Standalone ./test/e2e

RC=$?
if [ ${RC} -eq 0 ]
then
  DUMP_LOGS="false"
fi
echo "End to end test script return code set to ${RC}"
exit ${RC}
