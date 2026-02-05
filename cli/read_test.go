// Copyright 2022 Northern.tech AS
//
//    Licensed under the Apache License, Version 2.0 (the "License");
//    you may not use this file except in compliance with the License.
//    You may obtain a copy of the License at
//
//        http://www.apache.org/licenses/LICENSE-2.0
//
//    Unless required by applicable law or agreed to in writing, software
//    distributed under the License is distributed on an "AS IS" BASIS,
//    WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
//    See the License for the specific language governing permissions and
//    limitations under the License.

package cli

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/urfave/cli"
)

func TestArtifactsRead(t *testing.T) {
	// first create archive, that we will be able to read
	updateTestDir, _ := os.MkdirTemp("", "update")
	defer os.RemoveAll(updateTestDir)

	err := WriteArtifact(updateTestDir, 2, "")
	assert.NoError(t, err)

	err = Run([]string{"mender-artifact", "read"})
	assert.Error(t, err)
	assert.Contains(t, errors.Cause(err).Error(),
		"Nothing specified, nothing read.")

	err = Run([]string{"mender-artifact", "read",
		filepath.Join(updateTestDir, "artifact.mender")})
	assert.NoError(t, err)

	fakeErrWriter.Reset()
	err = Run([]string{"mender-artifact", "validate", "non-existing"})
	assert.Error(t, err)
	assert.Equal(t, errArtifactOpen, lastExitCode)
	assert.Contains(t, fakeErrWriter.String(), "no such file")
}

func TestReadArtifactOutput(t *testing.T) {
	cliContext := getCliContext()

	tmpdir, err := os.MkdirTemp("", "mendertest")
	require.NoError(t, err)
	defer os.RemoveAll(tmpdir)
	artfile := filepath.Join(tmpdir, "artifact.mender")

	files := map[string]string{
		"meta-data":         "{\"metadata\": \"test\"}",
		"updateFile":        "updateContent",
		"meta-data-augment": "{\"metadata\": \"augment\"}",
		"updateFileAugment": "augmentContent",
	}
	for file, content := range files {
		fd, err := os.OpenFile(filepath.Join(tmpdir, file), os.O_WRONLY|os.O_CREATE, 0644)
		require.NoError(t, err)
		fd.Write([]byte(content))
		fd.Close()
	}

	args := []string{
		"mender-artifact", "write", "module-image",
		"-o", artfile,
		"-n", "testName",
		"-t", "testDevice",
		"-N", "testNameDepends1",
		"-N", "testNameDepends2",
		"-g", "testGroupProvide",
		"-G", "testGroupDepends1",
		"-G", "testGroupDepends2",
		"-T", "testType",
		"-p", "testProvideKey1:testProvideValue1",
		"-p", "testProvideKey2:testProvideValue2",
		"-p", "overrideProvideKey:originalOverrideProvideValue",
		"-d", "testDependKey1:testDependValue1",
		"-d", "testDependKey2:testDependValue2",
		"-d", "overrideDependKey:originalOverrideDependValue",
		"-m", filepath.Join(tmpdir, "meta-data"),
		"-f", filepath.Join(tmpdir, "updateFile"),
		"--augment-type", "augmentType",
		"--augment-provides", "augmentProvideKey1:augmentProvideValue1",
		"--augment-provides", "augmentProvideKey2:augmentProvideValue2",
		"--augment-provides", "overrideProvideKey:augmentOverrideProvideValue",
		"--augment-depends", "augmentDependKey1:augmentDependValue1",
		"--augment-depends", "augmentDependKey2:augmentDependValue2",
		"--augment-depends", "overrideDependKey:augmentOverrideDependValue",
		"--augment-meta-data", filepath.Join(tmpdir, "meta-data-augment"),
		"--augment-file", filepath.Join(tmpdir, "updateFileAugment"),
	}
	err = cliContext.Run(args)
	require.NoError(t, err)

	expectedOutput := `Mender Artifact:
  Name: testName
  Format: mender
  Version: 3
  Signature: no signature
  Compatible types: [testDevice]
  Provides group: testGroupProvide
  Depends on one of artifact(s): [testNameDepends1, testNameDepends2]
  Depends on one of group(s): [testGroupDepends1, testGroupDepends2]
  State scripts: []

Updates:
  - Type: augmentType
    Provides:
      augmentProvideKey1: augmentProvideValue1
      augmentProvideKey2: augmentProvideValue2
      overrideProvideKey: augmentOverrideProvideValue
      rootfs-image.testType.version: testName
      testProvideKey1: testProvideValue1
      testProvideKey2: testProvideValue2
    Depends:
      augmentDependKey1: augmentDependValue1
      augmentDependKey2: augmentDependValue2
      overrideDependKey: augmentOverrideDependValue
      testDependKey1: testDependValue1
      testDependKey2: testDependValue2
    Clears Provides: [rootfs-image.testType.*]
    Metadata:
      {
        "metadata": "augment"
      }
    Files:
        name: updateFile
        size: 13
        name: updateFileAugment
        size: 14

`

	checkMenderArtifactRead(t, tmpdir, artfile, expectedOutput, cliContext)

	args = []string{
		"mender-artifact", "write", "rootfs-image",
		"-o", artfile,
		"-n", "testName",
		"-t", "testDevice",
		"-f", filepath.Join(tmpdir, "updateFile"),
	}
	err = cliContext.Run(args)
	require.NoError(t, err)

	expectedOutput = `Mender Artifact:
  Name: testName
  Format: mender
  Version: 3
  Signature: no signature
  Compatible types: [testDevice]
  Provides group: 
  Depends on one of artifact(s): []
  Depends on one of group(s): []
  State scripts: []

Updates:
  - Type: rootfs-image
    Provides:
      rootfs-image.version: testName
    Depends: {}
    Clears Provides: [artifact_group, rootfs_image_checksum, rootfs-image.*]
    Metadata: {}
    Files:
        name: updateFile
        size: 13

`

	checkMenderArtifactRead(t, tmpdir, artfile, expectedOutput, cliContext)
}

func TestReadBootstrapArtifactOutput(t *testing.T) {
	cliContext := getCliContext()

	tmpdir, err := os.MkdirTemp("", "mendertest")
	require.NoError(t, err)
	defer os.RemoveAll(tmpdir)
	artfile := filepath.Join(tmpdir, "bootstrap.mender")
	args := []string{
		"mender-artifact", "write", "bootstrap-artifact",
		"-o", artfile,
		"-n", "testName",
		"-t", "testDevice",
		"-g", "testGroupProvide",
		"-G", "testGroupDepends1",
		"-G", "testGroupDepends2",
		"-p", "testProvideKey1:testProvideValue1",
		"-p", "testProvideKey2:testProvideValue2",
		"-p", "overrideProvideKey:originalOverrideProvideValue",
		"-d", "testDependKey1:testDependValue1",
		"-d", "testDependKey2:testDependValue2",
		"-d", "overrideDependKey:originalOverrideDependValue",
	}
	err = cliContext.Run(args)
	require.NoError(t, err)

	oldStdout := os.Stdout
	defer func() {
		os.Stdout = oldStdout
	}()

	outputFile, err := os.OpenFile(filepath.Join(tmpdir, "output.log"),
		os.O_RDWR|os.O_TRUNC|os.O_CREATE, 0644)
	require.NoError(t, err)
	os.Stdout = outputFile

	args = []string{"mender-artifact", "read", artfile}
	err = cliContext.Run(args)
	assert.NoError(t, err)

	outputFile.Seek(0, 0)
	output, err := io.ReadAll(outputFile)
	outputFile.Close()
	require.NoError(t, err)

	assert.Contains(t, string(output), "Mender Artifact:\n")
	assert.Contains(t, string(output), "Name: testName\n")
	assert.Contains(t, string(output), "Format: mender\n")
	assert.Contains(t, string(output), "Version: 3\n")
	assert.Contains(t, string(output), "Signature: no signature\n")
	assert.Contains(t, string(output), "Compatible types: [testDevice]\n")
	assert.Contains(t, string(output), "Provides group: testGroupProvide\n")
	assert.Contains(t, string(output), "Depends on one of artifact(s): []\n")
	assert.Contains(t, string(output), "Provides group: testGroupProvide\n")
	assert.Contains(
		t,
		string(output),
		"Depends on one of group(s): [testGroupDepends1, testGroupDepends2]\n",
	)
	assert.Contains(t, string(output), "State scripts: []\n")

	assert.Contains(t, string(output), "Updates:\n")
	assert.Contains(t, string(output), "- Type: Empty type\n")
	assert.Contains(t, string(output), "Provides:\n")
	assert.Contains(t, string(output), "overrideProvideKey: originalOverrideProvideValue\n")
	assert.Contains(t, string(output), "testProvideKey1: testProvideValue1\n")
	assert.Contains(t, string(output), "testProvideKey2: testProvideValue2\n")
	assert.Contains(t, string(output), "Depends:\n")
	assert.Contains(t, string(output), "overrideDependKey: originalOverrideDependValue\n")
	assert.Contains(t, string(output), "testDependKey1: testDependValue1\n")
	assert.Contains(t, string(output), "testDependKey2: testDependValue2\n")
	assert.Contains(t, string(output), "Metadata: {}\n")
	assert.Contains(t, string(output), "Files: []\n")
}

func checkMenderArtifactRead(t *testing.T, tmpdir, artfile, expected string,
	cliContext *cli.App) {

	oldStdout := os.Stdout
	defer func() {
		os.Stdout = oldStdout
	}()

	outputFile, err := os.OpenFile(filepath.Join(tmpdir, "output.log"),
		os.O_RDWR|os.O_TRUNC|os.O_CREATE, 0644)
	require.NoError(t, err)
	os.Stdout = outputFile

	args := []string{"mender-artifact", "read", artfile}
	err = cliContext.Run(args)
	assert.NoError(t, err)

	outputFile.Seek(0, 0)
	output, err := io.ReadAll(outputFile)
	result := string(output)
	outputFile.Close()
	require.NoError(t, err)

	require.NoError(t, err)
	assert.Contains(t, result, " modified: ")
	assert.Contains(t, result, " checksum: ")
	cleaned := removeVolatileEntries(result)
	assert.Equal(
		t,
		cleaned,
		expected,
		fmt.Sprintf("\n%s\n--- DOESN'T MATCH EXPECTED ---\n%s", cleaned, expected),
	)
}
