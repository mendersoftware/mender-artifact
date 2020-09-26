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
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"

	"github.com/mendersoftware/mender-artifact/areader"
	"github.com/mendersoftware/mender-artifact/artifact"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestArtifactsWrite(t *testing.T) {
	os.Args = []string{"mender-artifact", "write"}
	err := run()
	// should output help message and no error
	assert.NoError(t, err)

	fakeErrWriter.Reset()

	os.Args = []string{"mender-artifact", "write", "rootfs-image"}
	err = run()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "Required flags \" n,  t,  f\" not set",
		"Required flags error missing")

	updateTestDir, _ := ioutil.TempDir("", "update")
	defer os.RemoveAll(updateTestDir)

	err = MakeFakeUpdateDir(updateTestDir,
		[]TestDirEntry{
			{
				Path:    "update.ext4",
				Content: []byte("my update"),
				IsDir:   false,
			},
		})
	assert.NoError(t, err)

	// no whitespace allowed in artifact-name
	os.Args = []string{"mender-artifact", "write", "rootfs-image", "-t", "my-device",
		"-n", "mender-1. 1", "-f", filepath.Join(updateTestDir, "update.ext4"),
		"-o", filepath.Join(updateTestDir, "art.mender"), "-v", "2"}
	err = run()
	assert.Equal(t, "whitespace is not allowed in the artifact-name", err.Error())

	// store named file V2.
	os.Args = []string{"mender-artifact", "write", "rootfs-image", "-t", "my-device",
		"-n", "mender-1.1", "-f", filepath.Join(updateTestDir, "update.ext4"),
		"-o", filepath.Join(updateTestDir, "art.mender"), "-v", "2"}
	err = run()
	assert.NoError(t, err)

	fs, err := os.Stat(filepath.Join(updateTestDir, "art.mender"))
	assert.NoError(t, err)
	assert.False(t, fs.IsDir())

	// store named file V3.
	os.Args = []string{"mender-artifact", "write", "rootfs-image", "-t", "my-device",
		"-n", "mender-1.1", "-f", filepath.Join(updateTestDir, "update.ext4"),
		"-o", filepath.Join(updateTestDir, "art.mender"), "-v", "3"}
	err = run()
	assert.NoError(t, err)

	// Write invalid artifact-version.
	os.Args = []string{"mender-artifact", "write", "rootfs-image", "-t", "my-device",
		"-n", "mender-1.1", "-f", filepath.Join(updateTestDir, "update.ext4"),
		"-o", filepath.Join(updateTestDir, "art.mender"), "-v", "300"}
	err = run()
	assert.Error(t, err)
}

func TestWithScripts(t *testing.T) {
	updateTestDir, _ := ioutil.TempDir("", "update")
	defer os.RemoveAll(updateTestDir)

	err := MakeFakeUpdateDir(updateTestDir,
		[]TestDirEntry{
			{
				Path:    "update.ext4",
				Content: []byte("my update"),
				IsDir:   false,
			},
			{
				Path:    "ArtifactInstall_Enter_99",
				Content: []byte("this is first enter script"),
				IsDir:   false,
			},
			{
				Path:    "ArtifactInstall_Leave_01",
				Content: []byte("this is leave script"),
				IsDir:   false,
			},
			{
				Path:    "script-dir",
				Content: []byte(""),
				IsDir:   true,
			},
			{
				Path:    "script-dir/ArtifactReboot_Enter_99",
				Content: []byte("this is reboot enter script"),
				IsDir:   false,
			},
			{
				Path:    "script-dir/ArtifactReboot_Leave_01",
				Content: []byte("this is reboot leave script"),
				IsDir:   false,
			},
			{
				Path:    "InvalidScript",
				Content: []byte("this is invalid script"),
				IsDir:   false,
			},
		})
	assert.NoError(t, err)

	// write artifact
	os.Args = []string{"mender-artifact", "write", "rootfs-image", "-t", "my-device",
		"-n", "mender-1.1", "-f", filepath.Join(updateTestDir, "update.ext4"),
		"-o", filepath.Join(updateTestDir, "artifact.mender"),
		"-s", filepath.Join(updateTestDir, "ArtifactInstall_Enter_99"),
		"-s", filepath.Join(updateTestDir, "ArtifactInstall_Leave_01"),
		"-s", filepath.Join(updateTestDir, "script-dir")}
	err = run()
	assert.NoError(t, err)

	// read artifact
	os.Args = []string{"mender-artifact", "read",
		filepath.Join(updateTestDir, "artifact.mender")}
	err = run()
	assert.NoError(t, err)

	// write artifact vith invalid version
	os.Args = []string{"mender-artifact", "write", "rootfs-image", "-t", "my-device",
		"-n", "mender-1.1", "-f", filepath.Join(updateTestDir, "update.ext4"),
		"-o", filepath.Join(updateTestDir, "artifact.mender"),
		"-s", filepath.Join(updateTestDir, "ArtifactInstall_Enter_99"),
		"-v", "1"}
	fakeErrWriter.Reset()
	err = run()
	assert.Error(t, err)
	assert.Equal(t, "Artifact version 1 is not supported\n",
		fakeErrWriter.String())

	// write artifact vith invalid script name
	os.Args = []string{"mender-artifact", "write", "rootfs-image", "-t", "my-device",
		"-n", "mender-1.1", "-f", filepath.Join(updateTestDir, "update.ext4"),
		"-o", filepath.Join(updateTestDir, "artifact.mender"),
		"-s", filepath.Join(updateTestDir, "InvalidScript")}
	fakeErrWriter.Reset()
	err = run()
	assert.Error(t, err)
	assert.Equal(t, "scripter: invalid script: InvalidScript\n",
		fakeErrWriter.String())
}

func TestWriteModuleImage(t *testing.T) {
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

	os.Args = []string{
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
	err = run()
	assert.NoError(t, err)

	artFd, err := os.Open(artfile)
	assert.NoError(t, err)
	reader := areader.NewReader(artFd)
	err = reader.ReadArtifact()
	assert.NoError(t, err)

	assert.Equal(t, "testName", reader.GetArtifactName())
	assert.Equal(t, []artifact.UpdateType{artifact.UpdateType{Type: "testType"}}, reader.GetUpdates())

	provides := reader.GetArtifactProvides()
	assert.NotNil(t, provides)
	assert.Equal(t, "testName", provides.ArtifactName)
	assert.Equal(t, "testGroupProvide", provides.ArtifactGroup)

	depends := reader.GetArtifactDepends()
	assert.NotNil(t, depends)
	assert.Equal(t, []string{"testNameDepends1", "testNameDepends2"}, depends.ArtifactName)
	assert.Equal(t, []string{"testGroupDepends1", "testGroupDepends2"}, depends.ArtifactGroup)

	updates := reader.GetUpdates()
	assert.Equal(t, 1, len(updates))
	assert.Equal(t, "testType", updates[0].Type)

	handlers := reader.GetHandlers()
	assert.Equal(t, 1, len(handlers))
	handler := handlers[0]
	assert.Equal(t, "augmentType", handler.GetUpdateType())
	assert.Equal(t, "testType", handler.GetUpdateOriginalType())

	updDepends := handler.GetUpdateOriginalDepends()
	assert.Equal(t, artifact.TypeInfoDepends{
		"testDependKey1":    "testDependValue1",
		"testDependKey2":    "testDependValue2",
		"overrideDependKey": "originalOverrideDependValue",
	}, updDepends)
	updDepends = handler.GetUpdateAugmentDepends()
	assert.Equal(t, artifact.TypeInfoDepends{
		"augmentDependKey1": "augmentDependValue1",
		"augmentDependKey2": "augmentDependValue2",
		"overrideDependKey": "augmentOverrideDependValue",
	}, updDepends)
	updDepends, err = handler.GetUpdateDepends()
	require.NoError(t, err)
	assert.Equal(t, artifact.TypeInfoDepends{
		"testDependKey1":    "testDependValue1",
		"testDependKey2":    "testDependValue2",
		"augmentDependKey1": "augmentDependValue1",
		"augmentDependKey2": "augmentDependValue2",
		"overrideDependKey": "augmentOverrideDependValue",
	}, updDepends)

	updProvides := handler.GetUpdateOriginalProvides()
	assert.Equal(t, artifact.TypeInfoProvides{
		"testProvideKey1":      "testProvideValue1",
		"testProvideKey2":      "testProvideValue2",
		"overrideProvideKey":   "originalOverrideProvideValue",
		"rootfs-image.version": "testName",
	}, updProvides)
	updProvides = handler.GetUpdateAugmentProvides()
	assert.Equal(t, artifact.TypeInfoProvides{
		"augmentProvideKey1": "augmentProvideValue1",
		"augmentProvideKey2": "augmentProvideValue2",
		"overrideProvideKey": "augmentOverrideProvideValue",
	}, updProvides)
	updProvides, err = handler.GetUpdateProvides()
	require.NoError(t, err)
	assert.Equal(t, artifact.TypeInfoProvides{
		"testProvideKey1":      "testProvideValue1",
		"testProvideKey2":      "testProvideValue2",
		"augmentProvideKey1":   "augmentProvideValue1",
		"augmentProvideKey2":   "augmentProvideValue2",
		"overrideProvideKey":   "augmentOverrideProvideValue",
		"rootfs-image.version": "testName",
	}, updProvides)

	assert.Equal(t, map[string]interface{}{"metadata": "test"}, handler.GetUpdateOriginalMetaData())
	assert.Equal(t, map[string]interface{}{"metadata": "augment"}, handler.GetUpdateAugmentMetaData())
	metaData, err := handler.GetUpdateMetaData()
	require.NoError(t, err)
	assert.Equal(t, map[string]interface{}{"metadata": "augment"}, metaData)

	os.Args = []string{
		"mender-artifact",
		"write",
		"module-image",
		"-v", "1",
		"-f", "foobar",
	}

	err = reader.ReadArtifact()
	assert.Error(t, err)
}

func TestWriteRootfsArtifactDependsAndProvides(t *testing.T) {
	tmpdir, err := ioutil.TempDir("", "mendertest")
	require.NoError(t, err)
	defer os.RemoveAll(tmpdir)
	artfile := filepath.Join(tmpdir, "artifact.mender")

	updateTestDir, _ := ioutil.TempDir("", "update")
	defer os.RemoveAll(updateTestDir)

	err = MakeFakeUpdateDir(updateTestDir,
		[]TestDirEntry{
			{
				Path:    "update.ext4",
				Content: []byte("my update"),
				IsDir:   false,
			},
		})
	assert.NoError(t, err)

	os.Args = []string{
		"mender-artifact", "write", "rootfs-image",
		"-t", "mydevice",
		"-o", artfile,
		"-f", filepath.Join(updateTestDir, "update.ext4"),
		"-n", "testName",
		"-N", "testNameDepends1",
		"-N", "testNameDepends2",
		"-G", "testGroupDepends1",
		"-G", "testGroupDepends2",
		"-g", "testGroupProvide",
		"-d", "testDependKey1:testDependValue1",
		"-d", "testDependKey2:testDependValue2",
		"-p", "testProvideKey1:testProvideValue1",
		"-p", "testProvideKey2:testProvideValue2",
	}
	err = run()
	assert.NoError(t, err)

	artFd, err := os.Open(artfile)
	assert.NoError(t, err)
	reader := areader.NewReader(artFd)
	err = reader.ReadArtifact()
	assert.NoError(t, err)

	// Verify name
	assert.Equal(t, "testName", reader.GetArtifactName())
	assert.Equal(t, []artifact.UpdateType{artifact.UpdateType{Type: "rootfs-image"}}, reader.GetUpdates())

	// Verify Provides
	provides := reader.GetArtifactProvides()
	assert.NotNil(t, provides)
	assert.Equal(t, "testName", provides.ArtifactName)
	assert.Equal(t, "testGroupProvide", provides.ArtifactGroup)

	// Verify Depends
	depends := reader.GetArtifactDepends()
	assert.NotNil(t, depends)
	assert.Equal(t, []string{"testNameDepends1", "testNameDepends2"}, depends.ArtifactName)
	assert.Equal(t, []string{"testGroupDepends1", "testGroupDepends2"}, depends.ArtifactGroup)

	// Verify update
	updates := reader.GetUpdates()
	assert.Equal(t, 1, len(updates))
	assert.Equal(t, "rootfs-image", updates[0].Type)
	handlers := reader.GetHandlers()
	assert.Equal(t, 1, len(handlers))
	handler := handlers[0]
	assert.Equal(t, "rootfs-image", handler.GetUpdateType())

	// Type-Info Depends
	updDepends, err := handler.GetUpdateDepends()
	require.NoError(t, err)
	assert.Equal(t, artifact.TypeInfoDepends{
		"testDependKey1": "testDependValue1",
		"testDependKey2": "testDependValue2",
	}, updDepends)

	// Type-Info Provides
	updProvides, err := handler.GetUpdateProvides()
	require.NoError(t, err)
	assert.Equal(t, artifact.TypeInfoProvides{
		// `rootfs-image.checksum` is always enabled
		"rootfs-image.checksum": "bfb4567944c5730face9f3d54efc0c1ff3b5dd1338862b23b849ac87679e162f",
		"testProvideKey1":       "testProvideValue1",
		"testProvideKey2":       "testProvideValue2",
		"rootfs-image.version":  "testName",
	}, updProvides)

	// Test the `--no-checksum-provide` flag
	tart := filepath.Join(tmpdir, "noprovides.mender")

	os.Args = []string{
		"mender-artifact", "write", "rootfs-image",
		"-t", "mydevice",
		"-o", tart,
		"-f", filepath.Join(updateTestDir, "update.ext4"),
		"-n", "noprovides",
		"--no-checksum-provide",
	}
	err = run()
	assert.NoError(t, err)

	artFd, err = os.Open(tart)
	assert.NoError(t, err)
	reader = areader.NewReader(artFd)
	err = reader.ReadArtifact()
	assert.NoError(t, err)

	assert.Equal(t, "noprovides", reader.GetArtifactName())
	assert.Equal(t, []artifact.UpdateType{artifact.UpdateType{Type: "rootfs-image"}}, reader.GetUpdates())

	handlers = reader.GetHandlers()
	assert.Equal(t, 1, len(handlers))
	handler = handlers[0]
	assert.Equal(t, "rootfs-image", handler.GetUpdateType())

	updProvides, err = handler.GetUpdateProvides()
	require.NoError(t, err)
	expected := artifact.TypeInfoProvides(artifact.TypeInfoProvides{"rootfs-image.version": "noprovides"})
	assert.Equal(t, expected, updProvides)
}

func TestWriteRootfsArtifactDependsAndProvidesOverrides(t *testing.T) {
	tmpdir, err := ioutil.TempDir("", "mendertest")
	require.NoError(t, err)
	defer os.RemoveAll(tmpdir)
	artfile := filepath.Join(tmpdir, "artifact.mender")

	updateTestDir, _ := ioutil.TempDir("", "update")
	defer os.RemoveAll(updateTestDir)

	err = MakeFakeUpdateDir(updateTestDir,
		[]TestDirEntry{
			{
				Path:    "update.ext4",
				Content: []byte("my update"),
				IsDir:   false,
			},
		})
	assert.NoError(t, err)

	testCases := map[string]struct {
		args            []string
		softwareVersion string
	}{
		"default": {
			args: []string{
				"mender-artifact", "write", "rootfs-image",
				"-t", "mydevice",
				"-o", artfile,
				"-f", filepath.Join(updateTestDir, "update.ext4"),
				"-n", "testName",
				"-N", "testNameDepends1",
				"-N", "testNameDepends2",
				"-G", "testGroupDepends1",
				"-G", "testGroupDepends2",
				"-g", "testGroupProvide",
				"-d", "testDependKey1:testDependValue1",
				"-d", "testDependKey2:testDependValue2",
				"-p", "testProvideKey1:testProvideValue1",
				"-p", "testProvideKey2:testProvideValue2",
			},
			softwareVersion: "testName",
		},
		"override with provides": {
			args: []string{
				"mender-artifact", "write", "rootfs-image",
				"-t", "mydevice",
				"-o", artfile,
				"-f", filepath.Join(updateTestDir, "update.ext4"),
				"-n", "testName",
				"-N", "testNameDepends1",
				"-N", "testNameDepends2",
				"-G", "testGroupDepends1",
				"-G", "testGroupDepends2",
				"-g", "testGroupProvide",
				"-d", "testDependKey1:testDependValue1",
				"-d", "testDependKey2:testDependValue2",
				"-p", "testProvideKey1:testProvideValue1",
				"-p", "testProvideKey2:testProvideValue2",
				"-p", "rootfs-image.version:v1",
			},
			softwareVersion: "v1",
		},
		"override with software-version": {
			args: []string{
				"mender-artifact", "write", "rootfs-image",
				"-t", "mydevice",
				"-o", artfile,
				"-f", filepath.Join(updateTestDir, "update.ext4"),
				"-n", "testName",
				"-N", "testNameDepends1",
				"-N", "testNameDepends2",
				"-G", "testGroupDepends1",
				"-G", "testGroupDepends2",
				"-g", "testGroupProvide",
				"-d", "testDependKey1:testDependValue1",
				"-d", "testDependKey2:testDependValue2",
				"-p", "testProvideKey1:testProvideValue1",
				"-p", "testProvideKey2:testProvideValue2",
				"-p", "rootfs-image.version:v1",
				"--software-version", "v2",
			},
			softwareVersion: "v2",
		},
	}
	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			os.Args = tc.args
			err = run()
			assert.NoError(t, err)

			artFd, err := os.Open(artfile)
			assert.NoError(t, err)
			reader := areader.NewReader(artFd)
			err = reader.ReadArtifact()
			assert.NoError(t, err)

			handlers := reader.GetHandlers()
			assert.Equal(t, 1, len(handlers))
			handler := handlers[0]
			assert.Equal(t, "rootfs-image", handler.GetUpdateType())

			updProvides, err := handler.GetUpdateProvides()
			require.NoError(t, err)

			assert.Equal(t, artifact.TypeInfoProvides{
				"rootfs-image.checksum": "bfb4567944c5730face9f3d54efc0c1ff3b5dd1338862b23b849ac87679e162f",
				"testProvideKey1":       "testProvideValue1",
				"testProvideKey2":       "testProvideValue2",
				"rootfs-image.version":  tc.softwareVersion,
			}, updProvides)
		})
	}
}

func TestWriteRootfsImageChecksum(t *testing.T) {

	// Cannot find payload file (nonexisting)
	err := writeRootfsImageChecksum("idonotexist", nil, false)
	assert.Contains(t, err.Error(), "Failed to open the payload file")

	// Checksum a dummy file
	tf, err := ioutil.TempFile("", "TestWriteRootfsImageChecksum")
	require.NoError(t, err)
	_, err = tf.Write([]byte("foobar"))
	require.NoError(t, err)
	require.NoError(t, tf.Close())
	typeInfo := artifact.TypeInfoV3{}

	err = writeRootfsImageChecksum(tf.Name(), &typeInfo, false)
	assert.NoError(t, err)
	require.NotNil(t, typeInfo.ArtifactProvides)
	_, ok := typeInfo.ArtifactProvides["rootfs-image.checksum"]
	assert.True(t, ok)

	// legacy key
	err = writeRootfsImageChecksum(tf.Name(), &typeInfo, true)
	assert.NoError(t, err)
	require.NotNil(t, typeInfo.ArtifactProvides)
	_, ok = typeInfo.ArtifactProvides["rootfs_image_checksum"]
	assert.True(t, ok)
}

func TestGetArtifactProvides(t *testing.T) {
	testCases := map[string]struct {
		artifactName             string
		artifactGroup            string
		softwareFilesystem       string
		softwareName             string
		softwareVersion          string
		noDefaultSoftwareVersion bool
		out                      map[string]string
	}{
		"rootfs, no software version": {
			artifactName:  "artifact-name",
			artifactGroup: "artifact-group",
			out: map[string]string{
				"rootfs-image.version": "artifact-name",
			},
		},
		"rootfs, software version": {
			artifactName:    "artifact-name",
			artifactGroup:   "artifact-group",
			softwareVersion: "v1",
			out: map[string]string{
				"rootfs-image.version": "v1",
			},
		},
		"rootfs, software name and version": {
			artifactName:    "artifact-name",
			artifactGroup:   "artifact-group",
			softwareName:    "my-software",
			softwareVersion: "v1",
			out: map[string]string{
				"rootfs-image.my-software.version": "v1",
			},
		},
		"rootfs, software filesystem, name and version": {
			artifactName:       "artifact-name",
			artifactGroup:      "artifact-group",
			softwareName:       "my-software",
			softwareVersion:    "v1",
			softwareFilesystem: "my-fs",
			out: map[string]string{
				"my-fs.my-software.version": "v1",
			},
		},
		"rootfs, software filesystem, name and version with no default software version": {
			artifactName:             "artifact-name",
			artifactGroup:            "artifact-group",
			softwareName:             "my-software",
			softwareVersion:          "v1",
			softwareFilesystem:       "my-fs",
			noDefaultSoftwareVersion: true,
			out: map[string]string{
				"my-fs.my-software.version": "v1",
			},
		},
		"rootfs, no default software version": {
			artifactName:             "artifact-name",
			artifactGroup:            "artifact-group",
			noDefaultSoftwareVersion: true,
			out:                      map[string]string{},
		},
	}

	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			result := getSoftwareVersion(tc.artifactName, tc.softwareFilesystem,
				tc.softwareName, tc.softwareVersion, tc.noDefaultSoftwareVersion)
			assert.Equal(t, tc.out, result)
		})
	}
}
