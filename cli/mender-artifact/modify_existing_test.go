// Copyright 2017 Northern.tech AS
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

func TestModifyImage(t *testing.T) {
	tmp, err := ioutil.TempDir("", "mender-modify")
	assert.NoError(t, err)

	defer os.RemoveAll(tmp)

	err = copyFile("mender_test.img", filepath.Join(tmp, "mender_test.img"))
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

func TestModifyArtifact(t *testing.T) {
	tmp, err := ioutil.TempDir("", "mender-modify")
	assert.NoError(t, err)

	defer os.RemoveAll(tmp)

	err = copyFile("mender_test.img", filepath.Join(tmp, "mender_test.img"))
	assert.NoError(t, err)

	err = WriteArtifact(tmp, 2, filepath.Join(tmp, "mender_test.img"))
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

func TestModifyServerCert(t *testing.T) {
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

func TestModifySigned(t *testing.T) {
	tmp, err := ioutil.TempDir("", "mender-modify")
	assert.NoError(t, err)
	defer os.RemoveAll(tmp)

	err = copyFile("mender_test.img", filepath.Join(tmp, "mender_test.img"))
	assert.NoError(t, err)

	err = ioutil.WriteFile(filepath.Join(tmp, "rsa.key"), []byte(PrivateRSAKey), 0711)
	assert.NoError(t, err)

	err = ioutil.WriteFile(filepath.Join(tmp, "ecdsa.key"), []byte(PrivateECDSAKey), 0711)
	assert.NoError(t, err)

	os.Args = []string{"mender-artifact", "write", "rootfs-image", "-t", "my-device",
		"-n", "release-1", "-u", filepath.Join(tmp, "mender_test.img"),
		"-o", filepath.Join(tmp, "artifact.mender"),
		"-k", filepath.Join(tmp, "rsa.key")}
	err = run()
	assert.NoError(t, err)

	os.Args = []string{"mender-artifact", "modify",
		"-n", "release-2",
		filepath.Join(tmp, "artifact.mender")}

	err = run()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), ErrInvalidSignature.Error())

	os.Args = []string{"mender-artifact", "modify",
		"-n", "release-2",
		"-k", filepath.Join(tmp, "rsa.key"),
		filepath.Join(tmp, "artifact.mender")}

	err = run()
	assert.NoError(t, err)

	os.Args = []string{"mender-artifact", "write", "rootfs-image", "-t", "my-device",
		"-n", "release-1", "-u", filepath.Join(tmp, "mender_test.img"),
		"-o", filepath.Join(tmp, "artifact_ecdsa.mender"),
		"-k", filepath.Join(tmp, "ecdsa.key")}
	err = run()
	assert.NoError(t, err)

	os.Args = []string{"mender-artifact", "modify",
		"-n", "release-2",
		"-k", filepath.Join(tmp, "ecdsa.key"),
		filepath.Join(tmp, "artifact_ecdsa.mender")}

	err = run()
	assert.NoError(t, err)
}
