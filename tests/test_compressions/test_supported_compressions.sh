#!/bin/bash
# Copyright 2023 Northern.tech AS
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

set -xe

supported_compressions=`"${TEST_MENDER_ARTIFACT_PATH}" write rootfs-image --help | grep compression | cut -f2 -d: | tr -d , | tr -d .`
[[ "$supported_compressions" != "" ]] || exit 1
for c in $supported_compressions; do
  "${TEST_MENDER_ARTIFACT_PATH}" --compression "$c" write rootfs-image -t test -o "test-rfs-${c}.mender" -n "test-${c}" -f test.txt
  "${TEST_MENDER_ARTIFACT_PATH}" read "test-rfs-${c}.mender"
  "${TEST_MENDER_ARTIFACT_PATH}" validate "test-rfs-${c}.mender"
done

