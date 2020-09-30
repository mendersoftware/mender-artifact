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
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()

	_, err = io.Copy(out, in)
	if err != nil {
		return err
	}
	return out.Close()
}

func TestDebugfs(t *testing.T) {
	tmp, err := ioutil.TempDir("", "mender-modify")
	assert.NoError(t, err)

	defer os.RemoveAll(tmp)

	err = copyFile("mender_test.img", filepath.Join(tmp, "mender_test.img"))
	assert.NoError(t, err)

	tDir, err := debugfsCopyFile("/etc/mender/artifact_info",
		filepath.Join(tmp, "mender_test.img"))

	assert.NoError(t, err)
	defer os.RemoveAll(tDir)
	st, err := os.Stat(filepath.Join(tDir, "artifact_info"))

	assert.NoError(t, err)
	assert.Equal(t, false, st.IsDir())

	tFile, err := ioutil.TempFile("", "test-mender-debugfs")
	assert.NoError(t, err)

	defer os.Remove(tFile.Name())

	_, err = io.WriteString(tFile, "my test data")
	assert.NoError(t, err)

	err = tFile.Close()
	assert.NoError(t, err)

	err = debugfsReplaceFile("artifact_info", tFile.Name(),
		filepath.Join(tmp, "mender_test.img"))
	assert.NoError(t, err)

	err = debugfsReplaceFile("/nonexisting/foo.txt", tFile.Name(), filepath.Join(tmp, "mender_test.img"))
	assert.Error(t, err)

	os.RemoveAll(tDir)
}

func verify(image, file, expected string) bool {
	tmp, err := debugfsCopyFile(file, image)
	if err != nil {
		return false
	}
	defer os.RemoveAll(tmp)

	data, err := ioutil.ReadFile(filepath.Join(tmp, filepath.Base(file)))
	if err != nil {
		return false
	}
	return strings.Contains(string(data), expected)
}

func verifySDImg(image, file, expected string) bool {

	part, err := virtualImage.Open(nil, nil, image)

	if err != nil {
		return false
	}
	defer part.Close()

	sdimg, ok := part.(*ModImageSdimg)
	if !ok {
		return false
	}

	return verify(sdimg.candidates[1].path, file, expected)
}

func TestModifyImage(t *testing.T) {
	tmp, err := ioutil.TempDir("", "mender-modify")
	assert.NoError(t, err)

	defer os.RemoveAll(tmp)

	err = copyFile("mender_test.img", filepath.Join(tmp, "mender_test.img"))
	assert.NoError(t, err)

	err = copyFile("mender_test.sdimg", filepath.Join(tmp, "mender_test.sdimg"))
	assert.NoError(t, err)

	os.Args = []string{"mender-artifact", "modify",
		filepath.Join(tmp, "mender_test.img"),
		"-n", "release-1"}
	err = run()
	assert.NoError(t, err)

	assert.True(t, verify(filepath.Join(tmp, "mender_test.img"),
		"/etc/mender/artifact_info", "artifact_name=release-1"))

	os.Args = []string{"mender-artifact", "modify",
		filepath.Join(tmp, "mender_test.img"),
		"-u", "https://docker.mender.io"}
	err = run()
	assert.NoError(t, err)

	assert.True(t, verify(filepath.Join(tmp, "mender_test.img"),
		"/etc/mender/mender.conf", "https://docker.mender.io"))

	os.Args = []string{"mender-artifact", "modify",
		filepath.Join(tmp, "mender_test.sdimg"),
		"--server-uri", "foo",
		"--tenant-token", "bar"}

	err = run()
	assert.NoError(t, err)

	assert.True(t, verifySDImg(filepath.Join(tmp, "mender_test.sdimg"),
		"/etc/mender/mender.conf", "foo"))

	assert.True(t, verifySDImg(filepath.Join(tmp, "mender_test.sdimg"),
		"/etc/mender/mender.conf", "bar"))
}

func TestModifySdimage(t *testing.T) {
	tmp, err := ioutil.TempDir("", "mender-modify")
	assert.NoError(t, err)

	defer os.RemoveAll(tmp)

	err = copyFile("mender_test.sdimg", filepath.Join(tmp, "mender_test.sdimg"))
	assert.NoError(t, err)

	os.Args = []string{"mender-artifact", "modify",
		filepath.Join(tmp, "mender_test.sdimg"),
		"-n", "mender-test"}
	err = run()
	assert.NoError(t, err)

	os.Args = []string{"mender-artifact", "modify",
		filepath.Join(tmp, "mender_test.sdimg"),
		"-u", "https://docker.mender.io"}
	err = run()
	assert.NoError(t, err)

}

func modifyAndRead(t *testing.T, artFile string, args ...string) string {
	os.Args = []string{"mender-artifact", "modify"}
	os.Args = append(os.Args, args...)
	os.Args = append(os.Args, artFile)

	err := run()
	require.NoError(t, err)

	os.Args = []string{"mender-artifact", "read", artFile}

	r, w, err := os.Pipe()
	out := os.Stdout
	defer func() {
		os.Stdout = out
	}()
	os.Stdout = w

	goErr := make(chan error)

	go func() {
		err := run()
		w.Close()
		goErr <- err
	}()

	data, err := ioutil.ReadAll(r)
	require.NoError(t, err)
	err = <-goErr
	require.NoError(t, err)

	return string(data)
}

func TestModifyRootfsArtifact(t *testing.T) {
	tmp, err := ioutil.TempDir("", "mender-modify")
	require.NoError(t, err)
	defer os.RemoveAll(tmp)

	err = copyFile("mender_test.img", filepath.Join(tmp, "mender_test.img"))
	require.NoError(t, err)

	for _, ver := range []int{2, 3} {
		err = WriteArtifact(tmp, ver, filepath.Join(tmp, "mender_test.img"))
		assert.NoError(t, err)

		data := modifyAndRead(t, filepath.Join(tmp, "artifact.mender"), "-n", "release-1")
		assert.Contains(t, data, "Name: release-1")
	}
}

func TestModifyRootfsServerCert(t *testing.T) {
	tmp, err := ioutil.TempDir("", "mender-modify")
	assert.NoError(t, err)
	defer os.RemoveAll(tmp)

	err = copyFile("mender_test.img", filepath.Join(tmp, "mender_test.img"))
	assert.NoError(t, err)

	os.Args = []string{"mender-artifact", "modify",
		"-c", "non-existing",
		filepath.Join(tmp, "mender_test.img")}

	fakeErrWriter.Reset()

	err = run()
	assert.Error(t, err)
	assert.Contains(t, fakeErrWriter.String(), "invalid server certificate")

	tmpCert, err := ioutil.TempFile("", "mender-test-cert")
	assert.NoError(t, err)
	defer os.Remove(tmpCert.Name())

	os.Args = []string{"mender-artifact", "modify",
		"-c", tmpCert.Name(),
		filepath.Join(tmp, "mender_test.img")}
	err = run()
	assert.NoError(t, err)
}

const (
	PrivateRSAKey = `-----BEGIN RSA PRIVATE KEY-----
MIICXAIBAAKBgQDSTLzZ9hQq3yBB+dMDVbKem6iav1J6opg6DICKkQ4M/yhlw32B
CGm2ArM3VwQRgq6Q1sNSq953n5c1EO3Xcy/qTAKcXwaUNml5EhW79AdibBXZiZt8
fMhCjUd/4ce3rLNjnbIn1o9L6pzV4CcVJ8+iNhne5vbA+63vRCnrc8QuYwIDAQAB
AoGAQKIRELQOsrZsxZowfj/ia9jPUvAmO0apnn2lK/E07k2lbtFMS1H4m1XtGr8F
oxQU7rLyyP/FmeJUqJyRXLwsJzma13OpxkQtZmRpL9jEwevnunHYJfceVapQOJ7/
6Oz0pPWEq39GCn+tTMtgSmkEaSH8Ki9t32g9KuQIKBB2hbECQQDsg7D5fHQB1BXG
HJm9JmYYX0Yk6Z2SWBr4mLO0C4hHBnV5qPCLyevInmaCV2cOjDZ5Sz6iF5RK5mw7
qzvFa8ePAkEA46Anom3cNXO5pjfDmn2CoqUvMeyrJUFL5aU6W1S6iFprZ/YwdHcC
kS5yTngwVOmcnT65Vnycygn+tZan2A0h7QJBAJNlowZovDdjgEpeCqXp51irD6Dz
gsLwa6agK+Y6Ba0V5mJyma7UoT//D62NYOmdElnXPepwvXdMUQmCtpZbjBsCQD5H
VHDJlCV/yzyiJz9+tZ5giaAkO9NOoUBsy6GvdfXWn2prXmiPI0GrrpSvp7Gj1Tjk
r3rtT0ysHWd7l+Kx/SUCQGlitd5RDfdHl+gKrCwhNnRG7FzRLv5YOQV81+kh7SkU
73TXPIqLESVrqWKDfLwfsfEpV248MSRou+y0O1mtFpo=
-----END RSA PRIVATE KEY-----`

	PrivateECDSAKey = `-----BEGIN EC PRIVATE KEY-----
MHcCAQEEIMOJJlcKM0sMwsOezNKeUXm4BiN6+ZPggu87yuZysDgIoAoGCCqGSM49
AwEHoUQDQgAE9iC/hyQO1UQfw0fFj1RjEjwOvPIBsz6Of3ock/gIwmnhnC/7USo3
yOTl4wVLQKA6mFvMV9o8B9yTBNg3mQS0vA==
-----END EC PRIVATE KEY-----`
)

// Remove entries from 'mender-artifact read' that are always changing and
// therefore cannot be compared.
func removeVolatileEntries(input string) string {
	var output strings.Builder
	for _, line := range strings.Split(input, "\n") {
		if strings.HasPrefix(line, "      checksum:") ||
			strings.HasPrefix(line, "      modified:") ||
			strings.HasPrefix(line, "\trootfs-image.checksum:") {
			continue
		}
		output.WriteString(line)
		output.WriteByte('\n')
	}
	return output.String()
}

func TestModifyRootfsSigned(t *testing.T) {
	tmp, err := ioutil.TempDir("", "mender-modify")
	assert.NoError(t, err)
	defer os.RemoveAll(tmp)

	err = copyFile("mender_test.img", filepath.Join(tmp, "mender_test.img"))
	assert.NoError(t, err)

	err = ioutil.WriteFile(filepath.Join(tmp, "rsa.key"), []byte(PrivateRSAKey), 0711)
	assert.NoError(t, err)

	err = ioutil.WriteFile(filepath.Join(tmp, "ecdsa.key"), []byte(PrivateECDSAKey), 0711)
	assert.NoError(t, err)

	for _, key := range []string{"rsa.key", "ecdsa.key"} {

		// Create and sign artifact using RSA private key.
		os.Args = []string{"mender-artifact", "write", "rootfs-image", "-t", "my-device",
			"-n", "release-1", "-f", filepath.Join(tmp, "mender_test.img"),
			"-o", filepath.Join(tmp, "artifact.mender"),
			"-k", filepath.Join(tmp, key)}
		err = run()
		assert.NoError(t, err)

		// Modify the artifact, the result shall be unsigned
		data := modifyAndRead(t, filepath.Join(tmp, "artifact.mender"), "-n", "release-2")
		expected := `Mender artifact:
  Name: release-2
  Format: mender
  Version: 3
  Signature: no signature
  Compatible devices: '[my-device]'
  Provides group: 
  Depends on one of artifact(s): []
  Depends on one of group(s): []
  State scripts:

Updates:
    0:
    Type:   rootfs-image
    Provides:
	rootfs-image.version: release-1
    Depends: Nothing
    Metadata: Nothing
    Files:
      name:     mender_test.img
      size:     524288

`
		assert.Equal(t, expected, removeVolatileEntries(data))

		// Modify again with a private key, and the result shall be signed
		data = modifyAndRead(t, filepath.Join(tmp, "artifact.mender"),
			"-n", "release-3", "-k", filepath.Join(tmp, key))
		expected = `Mender artifact:
  Name: release-3
  Format: mender
  Version: 3
  Signature: signed but no key for verification provided; please use ` + "`-k`" + ` option for providing verification key
  Compatible devices: '[my-device]'
  Provides group: 
  Depends on one of artifact(s): []
  Depends on one of group(s): []
  State scripts:

Updates:
    0:
    Type:   rootfs-image
    Provides:
	rootfs-image.version: release-1
    Depends: Nothing
    Metadata: Nothing
    Files:
      name:     mender_test.img
      size:     524288

`
		assert.Equal(t, expected, removeVolatileEntries(data))
	}

	// Make sure scripts are preserved.

	err = ioutil.WriteFile(filepath.Join(tmp, "ArtifactInstall_Enter_00"), []byte("commands"), 0755)
	require.NoError(t, err)
	err = ioutil.WriteFile(filepath.Join(tmp, "ArtifactCommit_Leave_00"), []byte("more commands"), 0755)
	require.NoError(t, err)

	os.Args = []string{"mender-artifact", "write", "rootfs-image", "-t", "my-device",
		"-n", "release-1", "-f", filepath.Join(tmp, "mender_test.img"),
		"-o", filepath.Join(tmp, "artifact.mender"),
		"-s", filepath.Join(tmp, "ArtifactInstall_Enter_00"),
		"-s", filepath.Join(tmp, "ArtifactCommit_Leave_00"),
	}
	err = run()
	assert.NoError(t, err)

	data := modifyAndRead(t, filepath.Join(tmp, "artifact.mender"),
		"-n", "release-2")

	// State scripts can unfortunately be in any order.
	var expectedScripts string
	if strings.Index(data, "ArtifactInstall") < strings.Index(data, "ArtifactCommit") {
		expectedScripts = `    ArtifactInstall_Enter_00
    ArtifactCommit_Leave_00`
	} else {
		expectedScripts = `    ArtifactCommit_Leave_00
    ArtifactInstall_Enter_00`
	}
	expected := `Mender artifact:
  Name: release-2
  Format: mender
  Version: 3
  Signature: no signature
  Compatible devices: '[my-device]'
  Provides group: 
  Depends on one of artifact(s): []
  Depends on one of group(s): []
  State scripts:
` + expectedScripts + `

Updates:
    0:
    Type:   rootfs-image
    Provides:
	rootfs-image.version: release-1
    Depends: Nothing
    Metadata: Nothing
    Files:
      name:     mender_test.img
      size:     524288

`
	assert.Equal(t, expected, removeVolatileEntries(data))
}

func TestModifyModuleArtifact(t *testing.T) {

	tmpdir, err := ioutil.TempDir("", "mendertest")
	require.NoError(t, err)
	defer os.RemoveAll(tmpdir)
	artfile := filepath.Join(tmpdir, "artifact.mender")

	err = ioutil.WriteFile(filepath.Join(tmpdir, "updateFile"), []byte("updateContent"), 0644)
	require.NoError(t, err)

	err = ioutil.WriteFile(filepath.Join(tmpdir, "updateFile2"), []byte("updateContent2"), 0644)
	require.NoError(t, err)

	os.Args = []string{
		"mender-artifact", "write", "module-image",
		"-o", artfile,
		"-n", "testName",
		"-t", "testDevice",
		"-T", "testType",
		"-f", filepath.Join(tmpdir, "updateFile"),
		"-f", filepath.Join(tmpdir, "updateFile2"),
	}

	err = run()
	assert.NoError(t, err)

	// Modify Artifact name shall work
	data := modifyAndRead(t, artfile, "-n", "release-1")
	expected := `Mender artifact:
  Name: release-1
  Format: mender
  Version: 3
  Signature: no signature
  Compatible devices: '[testDevice]'
  Provides group: 
  Depends on one of artifact(s): []
  Depends on one of group(s): []
  State scripts:

Updates:
    0:
    Type:   testType
    Provides:
	rootfs-image.testType.version: testName
    Depends: Nothing
    Metadata: Nothing
    Files:
      name:     updateFile
      size:     13
      name:     updateFile2
      size:     14

`
	assert.Equal(t, expected, removeVolatileEntries(data))

	// The rest of modifications shall not work
	os.Args = []string{
		"mender-artifact", "modify", "-u", "dummy-uri", artfile,
	}
	err = run()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), errFsTypeUnsupported.Error())

	require.NoError(t, ioutil.WriteFile("dummy-cert", []byte("SecretCert"), 0644))
	defer os.Remove("dummy-cert")
	os.Args = []string{
		"mender-artifact", "modify", "-c", "dummy-cert", artfile,
	}
	err = run()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), errFsTypeUnsupported.Error())

	require.NoError(t, ioutil.WriteFile("dummy-key", []byte("SecretKey"), 0644))
	defer os.Remove("dummy-key")
	os.Args = []string{
		"mender-artifact", "modify", "-v", "dummy-key", artfile,
	}
	err = run()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), errFsTypeUnsupported.Error())

	os.Args = []string{
		"mender-artifact", "modify", "-t", "dummy-token", artfile,
	}
	err = run()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), errFsTypeUnsupported.Error())

	// Make sure scripts and meta-data are preserved.

	err = ioutil.WriteFile(filepath.Join(tmpdir, "ArtifactInstall_Enter_00"), []byte("commands"), 0755)
	require.NoError(t, err)
	err = ioutil.WriteFile(filepath.Join(tmpdir, "ArtifactCommit_Leave_00"), []byte("more commands"), 0755)
	require.NoError(t, err)
	err = ioutil.WriteFile(filepath.Join(tmpdir, "meta-data"), []byte(`{"a":"b"}`), 0644)
	require.NoError(t, err)

	os.Args = []string{
		"mender-artifact", "write", "module-image",
		"-o", artfile,
		"-n", "testName",
		"-t", "testDevice",
		"-T", "testType",
		"-f", filepath.Join(tmpdir, "updateFile"),
		"-f", filepath.Join(tmpdir, "updateFile2"),
		"-s", filepath.Join(tmpdir, "ArtifactInstall_Enter_00"),
		"-s", filepath.Join(tmpdir, "ArtifactCommit_Leave_00"),
		"-m", filepath.Join(tmpdir, "meta-data"),
	}

	err = run()
	assert.NoError(t, err)

	// Modify Artifact name shall work
	data = modifyAndRead(t, artfile, "-n", "release-1")
	// State scripts can unfortunately be in any order.
	var expectedScripts string
	if strings.Index(string(data), "ArtifactInstall") < strings.Index(string(data), "ArtifactCommit") {
		expectedScripts = `    ArtifactInstall_Enter_00
    ArtifactCommit_Leave_00`
	} else {
		expectedScripts = `    ArtifactCommit_Leave_00
    ArtifactInstall_Enter_00`
	}
	expected = `Mender artifact:
  Name: release-1
  Format: mender
  Version: 3
  Signature: no signature
  Compatible devices: '[testDevice]'
  Provides group: 
  Depends on one of artifact(s): []
  Depends on one of group(s): []
  State scripts:
` + expectedScripts + `

Updates:
    0:
    Type:   testType
    Provides:
	rootfs-image.testType.version: testName
    Depends: Nothing
    Metadata:
	{
	  "a": "b"
	}
    Files:
      name:     updateFile
      size:     13
      name:     updateFile2
      size:     14

`
	assert.Equal(t, expected, removeVolatileEntries(data))
}

func TestModifyBrokenArtifact(t *testing.T) {
	tmpdir, err := ioutil.TempDir("", "")
	require.NoError(t, err)
	defer os.RemoveAll(tmpdir)

	artFile := filepath.Join(tmpdir, "artifact.mender")
	err = ioutil.WriteFile(artFile, []byte("bogus content"), 0644)
	require.NoError(t, err)

	os.Args = []string{"mender-artifact", "modify",
		"-n", "release-1", artFile}
	err = run()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "can not execute `parted` command or image is broken")
}

func TestModifyExtraAttributes(t *testing.T) {
	tmpdir, err := ioutil.TempDir("", "mendertest")
	require.NoError(t, err)
	defer os.RemoveAll(tmpdir)
	artfile := filepath.Join(tmpdir, "artifact.mender")

	err = ioutil.WriteFile(filepath.Join(tmpdir, "updateFile"), []byte("updateContent"), 0644)
	require.NoError(t, err)

	err = ioutil.WriteFile(filepath.Join(tmpdir, "meta-data"), []byte(`{"meta":"data"}`), 0644)
	require.NoError(t, err)

	os.Args = []string{
		"mender-artifact", "write", "module-image",
		"-o", artfile,
		"-n", "testName",
		"-t", "testDevice",
		"-T", "testType",
		"-f", filepath.Join(tmpdir, "updateFile"),
		"--no-default-software-version",
		// This provide attribute is not used by most Update Module. We
		// put it here to make sure that the modification logic
		// *doesn't* modify it, since this belongs only to the
		// rootfs-image domain.
		"-p", "rootfs-image.checksum:test",
	}
	err = run()
	require.NoError(t, err)

	// Test that we can add attributes.
	data := modifyAndRead(t, artfile, "--artifact-name-depends", "testNameDepends",
		"--artifact-name-depends", "testNameDepends2",
		"--provides-group", "testProvidesGroup",
		"--depends-groups", "testDependsGroup",
		"--depends-groups", "testDependsGroup2",
		"--provides", "testProvide1:SomeStuff1",
		"--provides", "testProvide2:SomeStuff2",
		"--depends", "testDepends1:SomeStuff1",
		"--depends", "testDepends2:SomeStuff2",
		"--meta-data", filepath.Join(tmpdir, "meta-data"),
	)
	expected := `Mender artifact:
  Name: testName
  Format: mender
  Version: 3
  Signature: no signature
  Compatible devices: '[testDevice]'
  Provides group: testProvidesGroup
  Depends on one of artifact(s): [testNameDepends, testNameDepends2]
  Depends on one of group(s): [testDependsGroup, testDependsGroup2]
  State scripts:

Updates:
    0:
    Type:   testType
    Provides:
	testProvide1: SomeStuff1
	testProvide2: SomeStuff2
    Depends:
	testDepends1: SomeStuff1
	testDepends2: SomeStuff2
    Metadata:
	{
	  "meta": "data"
	}
    Files:
      name:     updateFile
      size:     13

`
	assert.Equal(t, expected, removeVolatileEntries(data))

	// Test that attributes are not disturbed by a no-op modification.
	data = modifyAndRead(t, artfile)
	assert.Equal(t, expected, removeVolatileEntries(data))
}

func TestModifyExtraAttributesOnNonArtifact(t *testing.T) {
	tmpdir, err := ioutil.TempDir("", "")
	require.NoError(t, err)
	defer os.RemoveAll(tmpdir)

	art := filepath.Join(tmpdir, "mender_test.img")
	err = copyFile("mender_test.img", art)
	require.NoError(t, err)

	err = ioutil.WriteFile(filepath.Join(tmpdir, "meta-data"), []byte(`{"meta":"data"}`), 0644)
	require.NoError(t, err)

	paramPairs := [][]string{
		{"--artifact-name-depends", "testNameDepends"},
		{"--provides-group", "testGroupProvides"},
		{"--depends-groups", "testGroupDepends"},
		{"--depends", "depends:value"},
		{"--provides", "provides:value"},
		{"--meta-data", filepath.Join(tmpdir, "meta-data")},
	}

	for _, p := range paramPairs {
		t.Run(p[0], func(t *testing.T) {
			testModifyExtraAttributesOnNonArtifact(t, art, p)
		})
	}
}

func testModifyExtraAttributesOnNonArtifact(t *testing.T, art string, p []string) {
	os.Args = []string{"mender-artifact", "modify", p[0], p[1], art}

	err := run()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "must be used with an Artifact")
}
