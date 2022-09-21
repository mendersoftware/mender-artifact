#!/bin/bash
# Copyright 2022 Northern.tech AS
#
#    Licensed under the Apache License, Version 2.0 (the "License");
#    you may not use this file except in compliance with the License.
#    You may obtain a copy of the License at
#
#        http://www.apache.org/licenses/LICENSE-2.0
#
#    Unless required by applicable law or agreed to in writing, software
#    distributed under the License is distributed on an "AS IS" BASIS,
#    WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
#    See the License for the specific language governing permissions and
#    limitations under the License.

declare -A TEST_CONFIG=(
    [sopin]="0002"
    [pin]="0001"
    [keylen]="2048"
    [privatekey_path]="/tmp/private.key"
    [publickey_path]="/tmp/public.key"
    [artifact]="/tmp/ci-artifact.mender"
    [mender_artifact]="${TEST_MENDER_ARTIFACT_PATH}"
)

TEST_NAME="test_sign_with_hsm"

TEST_CONFIGURED=1
