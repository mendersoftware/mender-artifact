#!/bin/sh
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
set -e

# A simple shell script to verify that an Artifact is capeable of reading and writing
# an artifact on macOS (see https://northerntech.atlassian.net/browse/MEN-2505).

touch rootfs.ext4

########## Step 0 - Help text produced #########
mender-artifact write | diff ma-write-help-text.golden -

########## Step 1 - Write an Artifact  ##########
mender-artifact write rootfs-image -t beaglebone -n release-1 -f rootfs.ext4 -o artifact.mender

########## Step 2 - Verify an Artifact ##########
mender-artifact validate artifact.mender > /dev/null

########## Step 3 - Read an Artifact   ##########
mender-artifact read artifact.mender | diff --ignore-matching-lines='modified:.*' ma-read-output.golden -

########## Step 4 - Clean up           ##########
rm rootfs.ext4 artifact.mender
