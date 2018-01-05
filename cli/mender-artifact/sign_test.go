// Copyright 2018 Northern.tech AS
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

	"github.com/stretchr/testify/assert"
)

func TestSignExistingV1(t *testing.T) {
	// first create archive, that we will be able to read
	updateTestDir, _ := ioutil.TempDir("", "update")
	defer os.RemoveAll(updateTestDir)

	priv, pub, err := generateKeys()
	assert.NoError(t, err)

	err = WriteArtifact(updateTestDir, 1, "")
	assert.NoError(t, err)

	err = MakeFakeUpdateDir(updateTestDir,
		[]TestDirEntry{
			{
				Path:    "private.key",
				Content: priv,
				IsDir:   false,
			},
			{
				Path:    "public.key",
				Content: pub,
				IsDir:   false,
			},
		})
	assert.NoError(t, err)

	os.Args = []string{"mender-artifact", "sign",
		"-k", filepath.Join(updateTestDir, "private.key"),
		filepath.Join(updateTestDir, "artifact.mender")}

	err = run()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "can not create version 1 signed artifact")
}

func TestSignExistingV2(t *testing.T) {
	// first create archive, that we will be able to read
	updateTestDir, _ := ioutil.TempDir("", "update")
	defer os.RemoveAll(updateTestDir)

	priv, pub, err := generateKeys()
	assert.NoError(t, err)

	err = WriteArtifact(updateTestDir, 2, "")
	assert.NoError(t, err)

	err = MakeFakeUpdateDir(updateTestDir,
		[]TestDirEntry{
			{
				Path:    "private.key",
				Content: priv,
				IsDir:   false,
			},
			{
				Path:    "public.key",
				Content: pub,
				IsDir:   false,
			},
		})
	assert.NoError(t, err)

	os.Args = []string{"mender-artifact", "sign",
		"-k", filepath.Join(updateTestDir, "private.key"),
		"-o", filepath.Join(updateTestDir, "artifact.mender.sig"),
		filepath.Join(updateTestDir, "artifact.mender")}

	err = run()
	assert.NoError(t, err)

	os.Args = []string{"mender-artifact", "validate",
		"-k", filepath.Join(updateTestDir, "public.key"),
		filepath.Join(updateTestDir, "artifact.mender.sig")}

	err = run()
	assert.NoError(t, err)

	// now check if signing already signed will fail
	os.Args = []string{"mender-artifact", "sign",
		"-k", filepath.Join(updateTestDir, "private.key"),
		"-o", filepath.Join(updateTestDir, "artifact.mender.sig"),
		filepath.Join(updateTestDir, "artifact.mender.sig")}
	err = run()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "Trying to sign already signed artifact")

	// and the same as above with force option
	os.Args = []string{"mender-artifact", "sign",
		"-k", filepath.Join(updateTestDir, "private.key"),
		"-o", filepath.Join(updateTestDir, "artifact.mender.sig"), "-f",
		filepath.Join(updateTestDir, "artifact.mender.sig")}
	err = run()
	assert.NoError(t, err)
}

func TestSignExistingWithScripts(t *testing.T) {
	updateTestDir, _ := ioutil.TempDir("", "update")
	defer os.RemoveAll(updateTestDir)

	priv, pub, err := generateKeys()
	assert.NoError(t, err)

	err = MakeFakeUpdateDir(updateTestDir,
		[]TestDirEntry{
			{
				Path:    "private.key",
				Content: priv,
				IsDir:   false,
			},
			{
				Path:    "public.key",
				Content: pub,
				IsDir:   false,
			},
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
		})
	assert.NoError(t, err)

	// write artifact
	os.Args = []string{"mender-artifact", "write", "rootfs-image", "-t", "my-device",
		"-n", "mender-1.1", "-u", filepath.Join(updateTestDir, "update.ext4"),
		"-o", filepath.Join(updateTestDir, "artifact.mender"),
		"-s", filepath.Join(updateTestDir, "ArtifactInstall_Enter_99"),
		"-s", filepath.Join(updateTestDir, "ArtifactInstall_Leave_01")}
	err = run()
	assert.NoError(t, err)

	// test sign exisiting
	os.Args = []string{"mender-artifact", "sign",
		"-k", "-o", filepath.Join(updateTestDir, "artifact.mender.sig"),
		filepath.Join(updateTestDir, "artifact.mender")}

	err = run()
	assert.Error(t, err)

	// test sign exisiting
	os.Args = []string{"mender-artifact", "sign",
		"-o", filepath.Join(updateTestDir, "artifact.mender.sig"),
		filepath.Join(updateTestDir, "artifact.mender")}

	err = run()
	assert.Error(t, err)

	// test sign exisiting
	os.Args = []string{"mender-artifact", "sign",
		"-k", filepath.Join(updateTestDir, "private.key"),
		"-o", filepath.Join(updateTestDir, "artifact.mender.sig"),
		filepath.Join(updateTestDir, "artifact.mender")}

	err = run()
	assert.NoError(t, err)

}
