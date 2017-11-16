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
	"fmt"
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
	fmt.Printf("expected: [%s]; actual: [%s]\n", expected, string(data))
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
		filepath.Join(tmp, "artifact.mender")}

	err = run()
	assert.NoError(t, err)
}
