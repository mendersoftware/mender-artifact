// Copyright 2019 Northern.tech AS
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

package main

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"regexp"
	"testing"

	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/urfave/cli"
)

func TestArtifactsRead(t *testing.T) {
	// first create archive, that we will be able to read
	updateTestDir, _ := ioutil.TempDir("", "update")
	defer os.RemoveAll(updateTestDir)

	err := WriteArtifact(updateTestDir, 1, "")
	assert.NoError(t, err)

	os.Args = []string{"mender-artifact", "read"}
	err = run()
	assert.Error(t, err)
	assert.Contains(t, errors.Cause(err).Error(),
		"Nothing specified, nothing read.")

	os.Args = []string{"mender-artifact", "read",
		filepath.Join(updateTestDir, "artifact.mender")}
	err = run()
	assert.NoError(t, err)

	os.Args = []string{"mender-artifact", "validate", "non-existing"}
	fakeErrWriter.Reset()
	err = run()
	assert.Error(t, err)
	assert.Equal(t, errArtifactOpen, lastExitCode)
	assert.Contains(t, fakeErrWriter.String(), "no such file")
}

func TestReadArtifactOutput(t *testing.T) {
	cliContext := getCliContext()

	tmpdir, err := ioutil.TempDir("", "mendertest")
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

	expectedRegex := `Mender artifact:
  Name: testName
  Format: mender
  Version: 3
  Signature: no signature
  Compatible devices: '\[testDevice\]'
  Provides group: testGroupProvide
  Depends on one of artifact\(s\): \[testNameDepends1, testNameDepends2\]
  Depends on one of group\(s\): \[testGroupDepends1, testGroupDepends2\]
  State scripts:

Updates:
    0:
    Type:   augmentType
    Provides:
	augmentProvideKey1: augmentProvideValue1
	augmentProvideKey2: augmentProvideValue2
	overrideProvideKey: augmentOverrideProvideValue
	testProvideKey1: testProvideValue1
	testProvideKey2: testProvideValue2
    Depends:
	augmentDependKey1: augmentDependValue1
	augmentDependKey2: augmentDependValue2
	overrideDependKey: augmentOverrideDependValue
	testDependKey1: testDependValue1
	testDependKey2: testDependValue2
    Metadata:
	\{
	  "metadata": "augment"
	\}
    Files:
      name:     updateFile
      size:     13
      modified: .*
      checksum: ee7cd8c4f4613a5dd2bf585815a77209a13ea7410aa5dedcc8654993b30a4972
    Files:
      name:     updateFileAugment
      size:     14
      modified: .*
      checksum: 7511105a6f9a34b2b6877980400e99c5d3132cf8d73b28968a29f008667ed1de
`

	checkMenderArtifactRead(t, tmpdir, artfile, expectedRegex, cliContext)

	args = []string{
		"mender-artifact", "write", "rootfs-image",
		"-o", artfile,
		"-n", "testName",
		"-t", "testDevice",
		"-f", filepath.Join(tmpdir, "updateFile"),
	}
	err = cliContext.Run(args)
	require.NoError(t, err)

	expectedRegex = `Mender artifact:
  Name: testName
  Format: mender
  Version: 3
  Signature: no signature
  Compatible devices: '\[testDevice\]'
  Provides group: 
  Depends on one of artifact\(s\): \[\]
  Depends on one of group\(s\): \[\]
  State scripts:

Updates:
    0:
    Type:   rootfs-image
    Provides: Nothing
    Depends: Nothing
    Metadata: Nothing
    Files:
      name:     updateFile
      size:     13
      modified: .*
      checksum: ee7cd8c4f4613a5dd2bf585815a77209a13ea7410aa5dedcc8654993b30a4972
`

	checkMenderArtifactRead(t, tmpdir, artfile, expectedRegex, cliContext)
}

func checkMenderArtifactRead(t *testing.T, tmpdir, artfile, expectedRegex string,
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
	output, err := ioutil.ReadAll(outputFile)
	outputFile.Close()
	require.NoError(t, err)

	match, err := regexp.Match(expectedRegex, output)
	require.NoError(t, err)
	assert.True(t, match, fmt.Sprintf("%s\n--- DOESN'T MATCH ---\n%s", string(output), expectedRegex))
}
