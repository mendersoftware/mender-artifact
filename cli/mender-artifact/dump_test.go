// Copyright 2020 Northern.tech AS
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
	"sort"
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

	// Trim null byte (from --print0-cmdline).
	if printed[len(printed)-1] == 0 {
		printed = printed[:len(printed)-1]
	}
	return strings.TrimSpace(string(printed)), nil
}

func TestDumpContent(t *testing.T) {
	for _, printCmdline := range []string{"--print-cmdline", "--print0-cmdline"} {
		for _, imageType := range []string{"rootfs-image", "my-own-type"} {
			t.Run(fmt.Sprintf("%s/%s", imageType, printCmdline), func(t *testing.T) {
				testDumpContent(t, imageType, printCmdline)
			})
		}
	}
}

func testDumpContent(t *testing.T, imageType, printCmdline string) {
	tmpdir, err := ioutil.TempDir("", "")
	require.NoError(t, err)
	defer os.RemoveAll(tmpdir)

	var sep string
	switch printCmdline {
	case "--print-cmdline":
		sep = " "
	case "--print0-cmdline":
		sep = "\x00"
	default:
		t.Fatal("Unknown --print-cmdline mode")
	}

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
		"-G", "dependsGroup",
		"--no-default-software-version"})
	require.NoError(t, err)

	printed, err := runAndCollectStdout([]string{"mender-artifact", "dump",
		"--scripts", path.Join(tmpdir, "scripts"),
		"--meta-data", path.Join(tmpdir, "meta"),
		"--files", path.Join(tmpdir, "files"),
		printCmdline,
		path.Join(tmpdir, "artifact.mender")})

	assert.NoError(t, err)
	assert.Equal(t, strings.ReplaceAll(fmt.Sprintf(
		"write module-image"+
		" --artifact-name Name"+
		" --provides-group providesGroup"+
		" --artifact-name-depends dependsOnArtifact"+
		" --device-type TestDevice"+
		" --depends-groups dependsGroup"+
		" --type %s"+
		" --no-default-software-version"+
		" --provides testProvides:someProv"+
		" --depends testDepends:someDep"+
		" --no-default-clears-provides"+
		" --script %s/scripts/ArtifactInstall_Enter_45_test"+
		" --meta-data %s/meta/0000.meta-data"+
		" --file %s/files/file",
		imageType, tmpdir, tmpdir, tmpdir),
		// Replacing all spaces with sep is not safe in general when
		// using --print0-cmdline, but we know there are no
		// literal spaces in our test arguments.
		" ", sep),
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
		"--clears-provides", imageType + ".*",
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
		printCmdline,
		path.Join(tmpdir, "artifact.mender")})

	assert.NoError(t, err)
	printedStr := string(printed)

	// The provides, depends and scripts are stored in maps, where the order
	// is unpredictable, so split on the start of the flag, sort, and
	// compare that.
	expected := strings.Split(strings.ReplaceAll(
		"write module-image"+
		" --artifact-name Name"+
		" --provides-group providesGroup"+
		" --artifact-name-depends dependsOnArtifact"+
		" --artifact-name-depends dependsOnArtifact2"+
		" --device-type TestDevice"+
		" --device-type TestDevice2"+
		" --depends-groups dependsGroup"+
		" --depends-groups dependsGroup2"+
		fmt.Sprintf(" --type %s", imageType)+
		" --no-default-software-version"+
		" --no-default-clears-provides"+
		" --provides testProvides:someProv"+
		" --provides testProvides2:someProv2"+
		fmt.Sprintf(" --provides rootfs-image.%s.version:Name", imageType)+
		" --depends testDepends:someDep"+
		" --depends testDepends2:someDep2"+
		fmt.Sprintf(" --script %s/scripts/ArtifactInstall_Enter_45_test", tmpdir)+
		fmt.Sprintf(" --script %s/scripts/ArtifactCommit_Leave_55", tmpdir)+
		fmt.Sprintf(" --clears-provides %s.*", imageType)+
		fmt.Sprintf(" --clears-provides rootfs-image.%s.*", imageType)+
		fmt.Sprintf(" --meta-data %s/meta/0000.meta-data", tmpdir)+
		fmt.Sprintf(" --file %s/files/file", tmpdir)+
		fmt.Sprintf(" --file %s/files/file2", tmpdir),

		// Replacing all spaces with sep is not safe in general when
		// using --print0-cmdline, but we know there are no
		// literal spaces in our test arguments.
		" ", sep),

		// Split separator.
		fmt.Sprintf("%s--", sep))

	actual := strings.Split(printedStr, fmt.Sprintf("%s--", sep))
	sort.Strings(expected[1:])
	sort.Strings(actual[1:])

	assert.Equal(t, expected, actual)

	// --------------------------------------------------------------------
	// Flags
	// --------------------------------------------------------------------

	// Check that all flags which are documented on the command line are taken into
	// account in the "dump" command. *DO NOT* add flags to this list without making
	// sure that either:
	//
	// 1. It is tested somewhere in this file, by using the flag, dumping it, and
	// checking that it is recreated correctly.
	//
	// -or-
	//
	// 2. It does not need to be tested (no effect on dumping or tested elsewhere).
	flagChecker := newFlagChecker("write")
	flagChecker.addFlags([]string{
		"artifact-name",
		"artifact-name-depends",
		"clears-provides",
		"compression", // Not tested in "dump".
		"depends",
		"depends-groups",
		"device-type",
		"file",
		"key",                          // Not tested in "dump".
		"legacy-rootfs-image-checksum", // Not relevant for "dump", which uses "module-image".
		"meta-data",
		"no-checksum-provide", // Not relevant for "dump", which uses "module-image".
		"no-default-clears-provides",
		"no-default-software-version",
		"output-path", // Not relevant for "dump".
		"provides",
		"provides-group",
		"script",
		"software-filesystem", // These three indirectly handled by --provides.
		"software-name",       // <
		"software-version",    // <
		"ssh-args",            // Not relevant for "dump".
		"type",
		"version", // Could be supported, but in practice we only support >= v3.
		"no-progress",
	})

	flagChecker.checkAllFlagsTested(t)
}
