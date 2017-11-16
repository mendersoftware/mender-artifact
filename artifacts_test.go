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

func WriteArtifact(dir string, ver int, update string) error {
	if err := func() error {
		if update != "" {
			return nil
		}
		uFile, err := os.Create(filepath.Join(dir, "update.ext4"))
		if err != nil {
			return err
		}
		defer uFile.Close()

		_, err = uFile.WriteString("my update")
		if err != nil {
			return err
		}
		update = uFile.Name()
		return nil
	}(); err != nil {
		return err
	}

	f, err := os.Create(filepath.Join(dir, "artifact.mender"))
	if err != nil {
		return err
	}
	defer f.Close()

	u := handlers.NewRootfsV1(update)

	aw := awriter.NewWriter(f)
	switch ver {
	case 1:
		// we are alrady having v1 handlers; do nothing
	case 2:
		u = handlers.NewRootfsV2(update)
	}

	updates := &awriter.Updates{U: []handlers.Composer{u}}
	return aw.WriteArtifact("mender", ver, []string{"vexpress"},
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

	// no whitespace allowed in artifact-name
	os.Args = []string{"mender-artifact", "write", "rootfs-image", "-t", "my-device",
		"-n", "mender-1. 1", "-u", filepath.Join(updateTestDir, "update.ext4"),
		"-o", filepath.Join(updateTestDir, "art.mender")}
	err = run()
	assert.Equal(t, "whitespace is not allowed in the artifact-name", err.Error())

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

func generateKeys() ([]byte, []byte, error) {
	// key size needs to be 512 bits to handle message length
	priv, err := rsa.GenerateKey(rand.Reader, 512)
	if err != nil {
		return nil, nil, err
	}

	pub, err := x509.MarshalPKIXPublicKey(priv.Public())
	if err != nil {
		return nil, nil, err
	}

	pubSer := &bytes.Buffer{}
	err = pem.Encode(pubSer, &pem.Block{
		Type:  "PUBLIC KEY",
		Bytes: pub,
	})
	if err != nil {
		return nil, nil, err
	}

	privSer := &bytes.Buffer{}
	err = pem.Encode(privSer, &pem.Block{
		Type:  "PRIVATE KEY",
		Bytes: x509.MarshalPKCS1PrivateKey(priv),
	})
	if err != nil {
		return nil, nil, err
	}
	return privSer.Bytes(), pubSer.Bytes(), nil
}

func TestArtifactsSigned(t *testing.T) {
	updateTestDir, _ := ioutil.TempDir("", "update")
	defer os.RemoveAll(updateTestDir)

	priv, pub, err := generateKeys()
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

	// invalid private key
	os.Args = []string{"mender-artifact", "write", "rootfs-image", "-t", "my-device",
		"-n", "mender-1.1", "-u", filepath.Join(updateTestDir, "update.ext4"),
		"-o", filepath.Join(updateTestDir, "artifact.mender"),
		"-k", "non-existing-private.key"}
	err = run()
	assert.Error(t, err)
	assert.Contains(t, errors.Cause(err).Error(), "Invalid key path")

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
	assert.Contains(t, errors.Cause(err).Error(), "Invalid key path")

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
	assert.Contains(t, errors.Cause(err).Error(), "Invalid key path")

	// invalid version
	os.Args = []string{"mender-artifact", "write", "rootfs-image", "-t", "my-device",
		"-n", "mender-1.1", "-u", filepath.Join(updateTestDir, "update.ext4"),
		"-o", filepath.Join(updateTestDir, "artifact.mender"),
		"-k", filepath.Join(updateTestDir, "private.key"),
		"-v", "1"}
	fakeErrWriter.Reset()
	err = run()
	assert.Error(t, err)
	assert.Equal(t, "can not use signed artifact with version 1\n",
		fakeErrWriter.String())
}

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
	assert.Contains(t, fakeErrWriter.String(), "no such file")
}

func TestArtifactsValidate(t *testing.T) {
	// first create archive, that we will be able to read
	updateTestDir, _ := ioutil.TempDir("", "update")
	defer os.RemoveAll(updateTestDir)

	err := WriteArtifact(updateTestDir, 1, "")
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
	assert.Equal(t, 1, lastExitCode)
	assert.Contains(t, fakeErrWriter.String(), "no such file")
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
		"-n", "mender-1.1", "-u", filepath.Join(updateTestDir, "update.ext4"),
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
		"-n", "mender-1.1", "-u", filepath.Join(updateTestDir, "update.ext4"),
		"-o", filepath.Join(updateTestDir, "artifact.mender"),
		"-s", filepath.Join(updateTestDir, "ArtifactInstall_Enter_99"),
		"-v", "1"}
	fakeErrWriter.Reset()
	err = run()
	assert.Error(t, err)
	assert.Equal(t, "can not use scripts artifact with version 1\n",
		fakeErrWriter.String())

	// write artifact vith invalid script name
	os.Args = []string{"mender-artifact", "write", "rootfs-image", "-t", "my-device",
		"-n", "mender-1.1", "-u", filepath.Join(updateTestDir, "update.ext4"),
		"-o", filepath.Join(updateTestDir, "artifact.mender"),
		"-s", filepath.Join(updateTestDir, "InvalidScript")}
	fakeErrWriter.Reset()
	err = run()
	assert.Error(t, err)
	assert.Equal(t, "scripter: invalid script: InvalidScript\n",
		fakeErrWriter.String())
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
