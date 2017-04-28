// Copyright 2016 Mender Software AS
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
	"bytes"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"io/ioutil"
	"os"
	"path"
	"path/filepath"
	"testing"

	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	"github.com/urfave/cli"

	"github.com/mendersoftware/mender-artifact/awriter"
	"github.com/mendersoftware/mender-artifact/handlers"
)

var (
	lastExitCode = 0
	fakeOsExiter = func(rc int) {
		lastExitCode = rc
	}
	fakeErrWriter = &bytes.Buffer{}
)

func init() {
	cli.OsExiter = fakeOsExiter
	cli.ErrWriter = fakeErrWriter
}

func WriteRootfsImageArchive(dir string) error {
	if err := MakeFakeUpdateDir(dir,
		[]TestDirEntry{
			{
				Path:    "update.ext4",
				Content: []byte("my update"),
				IsDir:   false,
			},
		}); err != nil {
		return err
	}

	f, err := os.Create(filepath.Join(dir, "artifact.mender"))
	if err != nil {
		return err
	}
	defer f.Close()

	aw := awriter.NewWriter(f)
	u := handlers.NewRootfsV1(filepath.Join(dir, "update.ext4"))
	updates := &awriter.Updates{U: []handlers.Composer{u}}
	return aw.WriteArtifact("mender", 1, []string{"vexpress"},
		"mender-1.1", updates, nil)
}

func TestArtifactsWrite(t *testing.T) {
	os.Args = []string{"mender-artifact", "write"}
	err := run()
	// should output help message and no error
	assert.NoError(t, err)

	os.Args = []string{"mender-artifact", "write", "rootfs-image"}
	err = run()
	assert.Error(t, err)
	assert.Equal(t, 1, lastExitCode)
	assert.Equal(t, "must provide `device-type`, `artifact-name` and `update`\n",
		fakeErrWriter.String())

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

	// store named file
	os.Args = []string{"mender-artifact", "write", "rootfs-image", "-t", "my-device",
		"-n", "mender-1.1", "-u", filepath.Join(updateTestDir, "update.ext4"),
		"-o", filepath.Join(updateTestDir, "art.mender")}
	err = run()
	assert.NoError(t, err)

	fs, err := os.Stat(filepath.Join(updateTestDir, "art.mender"))
	assert.NoError(t, err)
	assert.False(t, fs.IsDir())

	// store named file
	os.Args = []string{"mender-artifact", "write", "rootfs-image", "-t", "my-device",
		"-n", "mender-1.1", "-u", filepath.Join(updateTestDir, "update.ext4"),
		"-o", filepath.Join(updateTestDir, "art.mender"), "-v", "3"}
	err = run()
	assert.Error(t, err)
}

func TestArtifactsSigned(t *testing.T) {
	updateTestDir, _ := ioutil.TempDir("", "update")
	defer os.RemoveAll(updateTestDir)

	// key size needs to be 512 bits to handle message length
	priv, err := rsa.GenerateKey(rand.Reader, 512)
	assert.NoError(t, err)

	pub, err := x509.MarshalPKIXPublicKey(priv.Public())
	assert.NoError(t, err)

	pubSer := &bytes.Buffer{}
	err = pem.Encode(pubSer, &pem.Block{
		Type:  "PUBLIC KEY",
		Bytes: pub,
	})
	assert.NoError(t, err)

	privSer := &bytes.Buffer{}
	err = pem.Encode(privSer, &pem.Block{
		Type:  "PRIVATE KEY",
		Bytes: x509.MarshalPKCS1PrivateKey(priv),
	})
	assert.NoError(t, err)

	err = MakeFakeUpdateDir(updateTestDir,
		[]TestDirEntry{
			{
				Path:    "update.ext4",
				Content: []byte("my update"),
				IsDir:   false,
			},
			{
				Path:    "private.key",
				Content: privSer.Bytes(),
				IsDir:   false,
			},
			{
				Path:    "public.key",
				Content: pubSer.Bytes(),
				IsDir:   false,
			},
		})
	assert.NoError(t, err)

	// invalid private key
	os.Args = []string{"mender-artifact", "write", "rootfs-image", "-t", "my-device",
		"-n", "mender-1.1", "-u", filepath.Join(updateTestDir, "update.ext4"),
		"-o", filepath.Join(updateTestDir, "artifact.mender"),
		"-k", "non-existing-private.key"}
	err = run()
	assert.Error(t, err)
	assert.Equal(t, "Invialid key path.", errors.Cause(err).Error())

	// store named file
	os.Args = []string{"mender-artifact", "write", "rootfs-image", "-t", "my-device",
		"-n", "mender-1.1", "-u", filepath.Join(updateTestDir, "update.ext4"),
		"-o", filepath.Join(updateTestDir, "artifact.mender"),
		"-k", filepath.Join(updateTestDir, "private.key")}
	err = run()
	assert.NoError(t, err)
	fs, err := os.Stat(filepath.Join(updateTestDir, "artifact.mender"))
	assert.NoError(t, err)
	assert.False(t, fs.IsDir())

	// read
	os.Args = []string{"mender-artifact", "read",
		"-k", filepath.Join(updateTestDir, "public.key"),
		filepath.Join(updateTestDir, "artifact.mender")}
	err = run()
	assert.NoError(t, err)

	// read invalid key
	os.Args = []string{"mender-artifact", "read",
		"-k", filepath.Join(updateTestDir, "private.key"),
		filepath.Join(updateTestDir, "artifact.mender")}
	err = run()
	assert.NoError(t, err)

	// read non-existing key
	os.Args = []string{"mender-artifact", "read",
		"-k", "non-existing-public.key",
		filepath.Join(updateTestDir, "artifact.mender")}
	err = run()
	assert.Error(t, err)
	assert.Equal(t, "Invialid key path.", errors.Cause(err).Error())

	// validate
	os.Args = []string{"mender-artifact", "validate",
		"-k", filepath.Join(updateTestDir, "public.key"),
		filepath.Join(updateTestDir, "artifact.mender")}
	err = run()
	assert.NoError(t, err)

	// validate non-existing key
	os.Args = []string{"mender-artifact", "validate",
		"-k", "non-existing-public.key",
		filepath.Join(updateTestDir, "artifact.mender")}
	err = run()
	assert.Error(t, err)
	assert.Equal(t, "Invialid key path.", errors.Cause(err).Error())
}

func TestArtifactsValidateError(t *testing.T) {
	os.Args = []string{"mender-artifact", "validate"}
	err := run()
	assert.Error(t, err)
	assert.Contains(t, errors.Cause(err).Error(),
		"Nothing specified, nothing validated.")

	os.Args = []string{"mender-artifact", "validate", "non-existing"}
	fakeErrWriter.Reset()
	err = run()
	assert.Error(t, err)
	assert.Equal(t, 1, lastExitCode)
	assert.Equal(t, "Pathspec 'non-existing' does not match any files.\n",
		fakeErrWriter.String())
}

func TestArtifactsValidate(t *testing.T) {
	// first create archive, that we will be able to read
	updateTestDir, _ := ioutil.TempDir("", "update")
	defer os.RemoveAll(updateTestDir)

	err := WriteRootfsImageArchive(updateTestDir)
	assert.NoError(t, err)

	os.Args = []string{"mender-artifact", "validate",
		filepath.Join(updateTestDir, "artifact.mender")}
	err = run()
	assert.NoError(t, err)
}

func TestArtifactsRead(t *testing.T) {
	// first create archive, that we will be able to read
	updateTestDir, _ := ioutil.TempDir("", "update")
	defer os.RemoveAll(updateTestDir)

	err := WriteRootfsImageArchive(updateTestDir)
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
	assert.Equal(t, 1, lastExitCode)
	assert.Equal(t, "Pathspec 'non-existing' does not match any files.\n",
		fakeErrWriter.String())
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
				Path:    "99_ArtifactDownload.Enter",
				Content: []byte("this is first enter script"),
				IsDir:   false,
			},
			{
				Path:    "01_ArtifactPreinstall.Leave",
				Content: []byte("this is leave script"),
				IsDir:   false,
			},
		})
	assert.NoError(t, err)

	// write artifact
	os.Args = []string{"mender-artifact", "write", "rootfs-image", "-t", "my-device",
		"-n", "mender-1.1", "-u", filepath.Join(updateTestDir, "update.ext4"),
		"-o", filepath.Join(updateTestDir, "artifact.mender"),
		"-s", filepath.Join(updateTestDir, "99_ArtifactDownload.Enter"),
		"-s", filepath.Join(updateTestDir, "01_ArtifactPreinstall.Leave")}
	err = run()
	assert.NoError(t, err)

	// read artifact
	os.Args = []string{"mender-artifact", "read",
		filepath.Join(updateTestDir, "artifact.mender")}
	err = run()
	assert.NoError(t, err)
}

type TestDirEntry struct {
	Path    string
	Content []byte
	IsDir   bool
}

func MakeFakeUpdateDir(updateDir string, elements []TestDirEntry) error {
	for _, elem := range elements {
		if elem.IsDir {
			if err := os.MkdirAll(path.Join(updateDir, elem.Path), os.ModeDir|os.ModePerm); err != nil {
				return err
			}
		} else {
			f, err := os.Create(path.Join(updateDir, elem.Path))
			if err != nil {
				return err
			}
			defer f.Close()
			if len(elem.Content) > 0 {
				if _, err = f.Write(elem.Content); err != nil {
					return err
				}
			}
		}
	}
	return nil
}
