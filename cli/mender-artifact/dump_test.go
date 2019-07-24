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
	"path"
	"strings"
	"testing"

	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func makeFile(t *testing.T, tmpdir, name, content string) {
	err := ioutil.WriteFile(path.Join(tmpdir, name), []byte(content), 0644)
	require.NoError(t, err)
}

func runAndCollectStdout(args []string) (string, error) {
	savedStdout := os.Stdout
	pipeR, pipeW, err := os.Pipe()
	if err != nil {
		return "", err
	}

	os.Stdout = pipeW
	defer func() {
		os.Stdout = savedStdout
		pipeW.Close()
		pipeR.Close()
	}()

	var goRoutineErr error
	go func() {
		defer func() {
			if r := recover(); r != nil {
				goRoutineErr = errors.Errorf("%v", r)
			}
			os.Stdout.Close()
		}()

		goRoutineErr = getCliContext().Run(args)
	}()

	printed, err := ioutil.ReadAll(pipeR)
	if err != nil {
		return "", err
	}

	if goRoutineErr != nil {
		return "", goRoutineErr
	}

	return strings.TrimSpace(string(printed)), nil
}

func TestDumpContent(t *testing.T) {
	for _, imageType := range []string{"rootfs-image", "my-own-type"} {
		t.Run(imageType, func(t *testing.T) {
			testDumpContent(t, imageType)
		})
	}
}

func testDumpContent(t *testing.T, imageType string) {
	tmpdir, err := ioutil.TempDir("", "")
	require.NoError(t, err)
	defer os.RemoveAll(tmpdir)

	makeFile(t, tmpdir, "file", "payload")
	makeFile(t, tmpdir, "file2", "payload2")
	makeFile(t, tmpdir, "meta-data", "{\"a\":\"b\"}")
	makeFile(t, tmpdir, "ArtifactInstall_Enter_45_test", "Bash magic")
	makeFile(t, tmpdir, "ArtifactCommit_Leave_55", "More Bash magic")

	// --------------------------------------------------------------------
	// Single values
	// --------------------------------------------------------------------

	// Use "module-image" writer here, so that we can insert some extra
	// fields that aren't typically in rootfs-images. One of these is
	// meta-data, which we don't use at the time of writing this, but which
	// may be used later.
	err = getCliContext().Run([]string{"mender-artifact", "write", "module-image",
		"-o", path.Join(tmpdir, "artifact.mender"),
		"-n", "Name",
		"-t", "TestDevice",
		"-T", imageType,
		"-N", "dependsOnArtifact",
		"-f", path.Join(tmpdir, "file"),
		"-m", path.Join(tmpdir, "meta-data"),
		"-s", path.Join(tmpdir, "ArtifactInstall_Enter_45_test"),
		"-d", "testDepends:someDep",
		"-p", "testProvides:someProv",
		"-g", "providesGroup",
		"-G", "dependsGroup"})
	require.NoError(t, err)

	printed, err := runAndCollectStdout([]string{"mender-artifact", "dump",
		"--scripts", path.Join(tmpdir, "scripts"),
		"--meta-data", path.Join(tmpdir, "meta"),
		"--files", path.Join(tmpdir, "files"),
		"--print-cmdline",
		path.Join(tmpdir, "artifact.mender")})

	assert.NoError(t, err)
	assert.Equal(t, fmt.Sprintf("write module-image"+
		" --artifact-name Name"+
		" --provides-group providesGroup"+
		" --artifact-name-depends dependsOnArtifact"+
		" --device-type TestDevice"+
		" --depends-groups dependsGroup"+
		" --type %s"+
		" --script %s/scripts/ArtifactInstall_Enter_45_test"+
		" --meta-data %s/meta/0000.meta-data"+
		" --file %s/files/file",
		imageType, tmpdir, tmpdir, tmpdir),
		string(printed))

	// --------------------------------------------------------------------
	// Multiple values
	// --------------------------------------------------------------------

	if imageType == "rootfs-image" {
		// "rootfs-image" doesn't support multiple payload files, so
		// skip testing that any further.
		return
	}

	os.RemoveAll(path.Join(tmpdir, "scripts"))
	os.RemoveAll(path.Join(tmpdir, "meta"))
	os.RemoveAll(path.Join(tmpdir, "files"))

	err = getCliContext().Run([]string{"mender-artifact", "write", "module-image",
		"-o", path.Join(tmpdir, "artifact.mender"),
		"-n", "Name",
		"-t", "TestDevice",
		"-t", "TestDevice2",
		"-T", imageType,
		"-N", "dependsOnArtifact",
		"-N", "dependsOnArtifact2",
		"-f", path.Join(tmpdir, "file"),
		"-f", path.Join(tmpdir, "file2"),
		"-m", path.Join(tmpdir, "meta-data"),
		"-s", path.Join(tmpdir, "ArtifactInstall_Enter_45_test"),
		"-s", path.Join(tmpdir, "ArtifactCommit_Leave_55"),
		"-d", "testDepends:someDep",
		"-p", "testProvides:someProv",
		"-d", "testDepends2:someDep2",
		"-p", "testProvides2:someProv2",
		"-g", "providesGroup",
		"-G", "dependsGroup",
		"-G", "dependsGroup2"})
	require.NoError(t, err)

	printed, err = runAndCollectStdout([]string{"mender-artifact", "dump",
		"--scripts", path.Join(tmpdir, "scripts"),
		"--meta-data", path.Join(tmpdir, "meta"),
		"--files", path.Join(tmpdir, "files"),
		"--print-cmdline",
		path.Join(tmpdir, "artifact.mender")})

	assert.NoError(t, err)
	printedStr := string(printed)
	// The scripts are stored in a map, where the order is unpredictable, so
	// cover both cases.
	if strings.Index(printedStr, "ArtifactInstall_Enter_45_test") < strings.Index(printedStr, "ArtifactCommit_Leave_55") {
		assert.Equal(t, fmt.Sprintf("write module-image"+
			" --artifact-name Name"+
			" --provides-group providesGroup"+
			" --artifact-name-depends dependsOnArtifact"+
			" --artifact-name-depends dependsOnArtifact2"+
			" --device-type TestDevice"+
			" --device-type TestDevice2"+
			" --depends-groups dependsGroup"+
			" --depends-groups dependsGroup2"+
			" --type %s"+
			" --script %s/scripts/ArtifactInstall_Enter_45_test"+
			" --script %s/scripts/ArtifactCommit_Leave_55"+
			" --meta-data %s/meta/0000.meta-data"+
			" --file %s/files/file"+
			" --file %s/files/file2",
			imageType, tmpdir, tmpdir, tmpdir, tmpdir, tmpdir),
			printedStr)
	} else {
		assert.Equal(t, fmt.Sprintf("write module-image"+
			" --artifact-name Name"+
			" --provides-group providesGroup"+
			" --artifact-name-depends dependsOnArtifact"+
			" --artifact-name-depends dependsOnArtifact2"+
			" --device-type TestDevice"+
			" --device-type TestDevice2"+
			" --depends-groups dependsGroup"+
			" --depends-groups dependsGroup2"+
			" --type %s"+
			" --script %s/scripts/ArtifactCommit_Leave_55"+
			" --script %s/scripts/ArtifactInstall_Enter_45_test"+
			" --meta-data %s/meta/0000.meta-data"+
			" --file %s/files/file"+
			" --file %s/files/file2",
			imageType, tmpdir, tmpdir, tmpdir, tmpdir, tmpdir),
			printedStr)
	}
}
