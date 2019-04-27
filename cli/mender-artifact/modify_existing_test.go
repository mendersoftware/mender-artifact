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

	candidateType, modifyCandidates, err :=
		getCandidatesForModify(image)

	if err != nil {
		return false
	}

	if candidateType != RawSDImage {
		return false
	}

	return verify(modifyCandidates[1].path, file, expected)
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

func TestModifyRootfsArtifact(t *testing.T) {
	tmp, err := ioutil.TempDir("", "mender-modify")
	require.NoError(t, err)
	defer os.RemoveAll(tmp)

	err = copyFile("mender_test.img", filepath.Join(tmp, "mender_test.img"))
	require.NoError(t, err)

	for _, ver := range []int{2, 3} {
		err = WriteArtifact(tmp, ver, filepath.Join(tmp, "mender_test.img"))
		assert.NoError(t, err)

		os.Args = []string{"mender-artifact", "modify",
			"-n", "release-1",
			filepath.Join(tmp, "artifact.mender")}

		err = run()
		assert.NoError(t, err)

		os.Args = []string{"mender-artifact", "read",
			filepath.Join(tmp, "artifact.mender")}

		r, w, err := os.Pipe()
		out := os.Stdout
		defer func() {
			os.Stdout = out
		}()
		os.Stdout = w

		go func() {
			err = run()
			assert.NoError(t, err)
			w.Close()
		}()

		data, _ := ioutil.ReadAll(r)
		assert.Contains(t, string(data), "Name: release-1")
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
		os.Args = []string{"mender-artifact", "modify",
			"-n", "release-2",
			filepath.Join(tmp, "artifact.mender")}

		err = run()
		assert.NoError(t, err)

		// Check for field update and unsigned state
		os.Args = []string{"mender-artifact", "read",
		filepath.Join(tmp, "artifact.mender")}

		r, w, err := os.Pipe()
		out := os.Stdout
		defer func() {
			os.Stdout = out
		}()
		os.Stdout = w

		go func() {
			err = run()
			assert.NoError(t, err)
			w.Close()
		}()

		data, _ := ioutil.ReadAll(r)
		assert.Contains(t, string(data), "Name: release-2")
		assert.Contains(t, string(data), "Signature: no signature")

	}
}

func TestModifyModuleArtifact(t *testing.T) {

	tmpdir, err := ioutil.TempDir("", "mendertest")
	require.NoError(t, err)
	defer os.RemoveAll(tmpdir)
	artfile := filepath.Join(tmpdir, "artifact.mender")

	fd, err := os.OpenFile(filepath.Join(tmpdir, "updateFile"), os.O_WRONLY|os.O_CREATE, 0644)
	require.NoError(t, err)
	fd.Write([]byte("updateContent"))
	fd.Close()

	os.Args = []string{
		"mender-artifact", "write", "module-image",
		"-o", artfile,
		"-n", "testName",
		"-t", "testDevice",
		"-T", "testType",
		"-f", filepath.Join(tmpdir, "updateFile"),
	}

	err = run()
	assert.NoError(t, err)

	// Modify Artifact name shall work
	os.Args = []string{"mender-artifact", "modify",
		"-n", "release-1", artfile}

	err = run()
	assert.NoError(t, err)

	os.Args = []string{"mender-artifact", "read", artfile}

	r, w, err := os.Pipe()
	out := os.Stdout
	defer func() {
		os.Stdout = out
	}()
	os.Stdout = w

	go func() {
		err = run()
		assert.NoError(t, err)
		w.Close()
	}()

	data, _ := ioutil.ReadAll(r)
	assert.Contains(t, string(data), "Name: release-1")

	// The rest of modifications shall not work
	os.Args = []string{
		"mender-artifact", "modify", "-u", "dummy-uri", artfile,
	}
	err = run()
	assert.Error(t, err)
	assert.Contains(t, err.Error(),	"mender-artifact can only modify ext4 payloads")

	os.Args = []string{
		"mender-artifact", "modify", "-c", "dummy-cert", artfile,
	}
	err = run()
	assert.Error(t, err)
	assert.Contains(t, err.Error(),	"mender-artifact can only modify ext4 payloads")

	os.Args = []string{
		"mender-artifact", "modify", "-v", "dummy-key", artfile,
	}
	err = run()
	assert.Error(t, err)
	assert.Contains(t, err.Error(),	"mender-artifact can only modify ext4 payloads")

	os.Args = []string{
		"mender-artifact", "modify", "-t", "dummy-token", artfile,
	}
	err = run()
	assert.Error(t, err)
	assert.Contains(t, err.Error(),	"mender-artifact can only modify ext4 payloads")
}
