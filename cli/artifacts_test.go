// Copyright 2022 Northern.tech AS
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

package cli

import (
	"bytes"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"testing"

	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	"github.com/urfave/cli"

	"github.com/mendersoftware/mender-artifact/artifact"
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

func CreateFakeUpdate() (string, error) {
	upd, err := os.CreateTemp("", "mender-update")
	if err != nil {
		return "", err
	}
	_, err = upd.WriteString("my update")
	return upd.Name(), nil
}

func WriteTestArtifact(version int, update string, key []byte) (io.Reader, error) {
	comp := artifact.NewCompressorGzip()
	buff := bytes.NewBuffer(nil)

	aw := new(awriter.Writer)
	if key != nil {
		s, err := artifact.NewPKISigner(key)
		if err != nil {
			return nil, errors.Wrap(err, "artifact.NewPKISigner")
		}
		aw = awriter.NewWriterSigned(buff, comp, s)
		fmt.Println("write signed artifact")
	} else {
		aw = awriter.NewWriter(buff, comp)
	}

	var err error
	if update == "" {
		update, err = CreateFakeUpdate()
		if err != nil {
			return nil, nil
		}
		defer os.Remove(update)
	}

	rfs := handlers.NewRootfsV2(update)

	switch version {
	case 1:
		// we are alrady having v1 handlers; do nothing
	case 2:
		rfs = handlers.NewRootfsV2(update)
	}

	updates := &awriter.Updates{Updates: []handlers.Composer{rfs}}

	err = aw.WriteArtifact(&awriter.WriteArtifactArgs{
		Format:  "mender",
		Name:    "test-artifact",
		Version: version,
		Devices: []string{"vexpress"},
		Updates: updates,
	})
	if err != nil {
		return nil, err
	}

	return buff, nil
}

func WriteArtifact(dir string, ver int, update string) error {
	comp := artifact.NewCompressorGzip()

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

	u := handlers.NewRootfsV2(update)

	aw := awriter.NewWriter(f, comp)
	switch ver {
	case 1:
		// we are already having v1 handlers; do nothing
	case 2:
		u = handlers.NewRootfsV2(update)
	case 3:
		u = handlers.NewRootfsV3(update)
	}

	updates := &awriter.Updates{Updates: []handlers.Composer{u}}

	depends := artifact.ArtifactDepends{
		ArtifactName:      []string{""},
		CompatibleDevices: []string{""},
		ArtifactGroup:     []string{""},
	}

	provides := artifact.ArtifactProvides{
		ArtifactName:  "test-artifact",
		ArtifactGroup: "",
	}

	typeInfo := "rootfs-image"
	typeInfoV3 := artifact.TypeInfoV3{
		Type: &typeInfo,
		// Keeping these empty for now. We will likely introduce these
		// later, when we add support for augmented artifacts.
		// ArtifactDepends:  &artifact.TypeInfoDepends{"rootfs-image.checksum": c.String("depends-rootfs-image-checksum")},
		// ArtifactProvides: &artifact.TypeInfoProvides{"rootfs-image.checksum": c.String("provides-rootfs-image-checksum")},
		ArtifactDepends:  artifact.TypeInfoDepends{},
		ArtifactProvides: artifact.TypeInfoProvides{},
	}

	if ver >= 3 {
		err = writeRootfsImageChecksum(update, &typeInfoV3, false)
		if err != nil {
			return err
		}
	}

	return aw.WriteArtifact(&awriter.WriteArtifactArgs{
		Format:     "mender",
		Name:       "test-artifact",
		Version:    ver,
		Devices:    []string{"vexpress"},
		Updates:    updates,
		Provides:   &provides,
		Depends:    &depends,
		TypeInfoV3: &typeInfoV3,
	})
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
	updateTestDir, _ := os.MkdirTemp("", "update")
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
	args := []string{"mender-artifact", "write", "rootfs-image", "-t", "my-device",
		"-n", "mender-1.1", "-f", filepath.Join(updateTestDir, "update.ext4"),
		"-o", filepath.Join(updateTestDir, "artifact.mender"),
		"-k", "non-existing-private.key"}
	err = Run(args)
	assert.Error(t, err)
	assert.Contains(t, errors.Cause(err).Error(), "Error reading key file")

	// store named file
	args = []string{"mender-artifact", "write", "rootfs-image", "-t", "my-device",
		"-n", "mender-1.1", "-f", filepath.Join(updateTestDir, "update.ext4"),
		"-o", filepath.Join(updateTestDir, "artifact.mender"),
		"-k", filepath.Join(updateTestDir, "private.key")}
	err = Run(args)
	assert.NoError(t, err)
	fs, err := os.Stat(filepath.Join(updateTestDir, "artifact.mender"))
	assert.NoError(t, err)
	assert.False(t, fs.IsDir())

	// read
	args = []string{"mender-artifact", "read",
		"-k", filepath.Join(updateTestDir, "public.key"),
		filepath.Join(updateTestDir, "artifact.mender")}
	err = Run(args)
	assert.NoError(t, err)

	// read invalid key
	args = []string{"mender-artifact", "read",
		"-k", filepath.Join(updateTestDir, "private.key"),
		filepath.Join(updateTestDir, "artifact.mender")}
	err = Run(args)
	assert.Error(t, err)

	// read non-existing key
	args = []string{"mender-artifact", "read",
		"-k", "non-existing-public.key",
		filepath.Join(updateTestDir, "artifact.mender")}
	err = Run(args)
	assert.Error(t, err)
	assert.Contains(t, errors.Cause(err).Error(), "Error reading key file")

	// validate
	args = []string{"mender-artifact", "validate",
		"-k", filepath.Join(updateTestDir, "public.key"),
		filepath.Join(updateTestDir, "artifact.mender")}
	err = Run(args)
	assert.NoError(t, err)

	// validate non-existing key
	args = []string{"mender-artifact", "validate",
		"-k", "non-existing-public.key",
		filepath.Join(updateTestDir, "artifact.mender")}
	err = Run(args)
	assert.Error(t, err)
	assert.Contains(t, errors.Cause(err).Error(), "Error reading key file")

	// invalid version
	args = []string{"mender-artifact", "write", "rootfs-image", "-t", "my-device",
		"-n", "mender-1.1", "-f", filepath.Join(updateTestDir, "update.ext4"),
		"-o", filepath.Join(updateTestDir, "artifact.mender"),
		"-k", filepath.Join(updateTestDir, "private.key"),
		"-v", "1"}
	fakeErrWriter.Reset()
	err = Run(args)
	assert.Error(t, err)
	assert.Equal(t, "Artifact version 1 is not supported\n",
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
