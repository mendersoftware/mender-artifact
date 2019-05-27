#!/bin/sh

set -e

# A simple shell script to verify that an Artifact is capeable of reading and writing
# an artifact on macOS (see https://tracker.mender.io/browse/MEN-2505).

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
