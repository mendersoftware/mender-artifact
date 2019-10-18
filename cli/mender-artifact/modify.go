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
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"

	"github.com/mendersoftware/mender-artifact/artifact"
	"github.com/pkg/errors"
	"github.com/urfave/cli"
)

func modifyArtifact(c *cli.Context) (err error) {
	comp, err := artifact.NewCompressorFromId(c.GlobalString("compression"))
	if err != nil {
		return cli.NewExitError("compressor '"+c.GlobalString("compression")+"' is not supported: "+err.Error(), 1)
	}

	privateKey, err := getKey(c.String("key"))
	if err != nil {
		return cli.NewExitError("Unable to load key: "+err.Error(), 1)
	}

	if c.NArg() == 0 {
		return cli.NewExitError("Nothing specified, nothing will be modified. \n"+
			"Maybe you wanted to say 'artifacts read <pathspec>'?", 1)
	}

	if _, err := os.Stat(c.Args().First()); err != nil && os.IsNotExist(err) {
		return cli.NewExitError("File ["+c.Args().First()+"] does not exist.", 1)
	}

	image, err := virtualImage.Open(comp, privateKey, c.Args().First())

	if err != nil {
		return cli.NewExitError("Error selecting images for modification: "+err.Error(), 1)
	}
	defer func() {
		if err == nil {
			err = image.Close()
			if err != nil {
				err = cli.NewExitError("Error closing image: "+err.Error(), 1)
			}
		} else {
			image.Close()
		}
	}()

	if err := modifyExisting(c, image); err != nil {
		return cli.NewExitError("Error modifying artifact["+c.Args().First()+"]: "+
			err.Error(), 1)
	}

	return nil
}

// oblivious to whether the file exists beforehand
func modifyArtifactInfoName(name string, image VPImage) error {
	art, isArt := image.(*ModImageArtifact)
	if isArt {
		// For artifacts, modify name in attributes.
		art.writeArgs.Name = name
		if art.writeArgs.Provides != nil {
			art.writeArgs.Provides.ArtifactName = name
		}
	}

	data := fmt.Sprintf("artifact_name=%s", name)
	tmpNameFile, err := ioutil.TempFile("", "mender-name")
	if err != nil {
		return err
	}
	defer os.Remove(tmpNameFile.Name())
	defer tmpNameFile.Close()

	if _, err = tmpNameFile.WriteString(data); err != nil {
		return err
	}

	if err = tmpNameFile.Close(); err != nil {
		return err
	}

	err = CopyIntoImage(tmpNameFile.Name(), image, "/etc/mender/artifact_info")
	if errors.Cause(err) == errFsTypeUnsupported && isArt {
		// This is ok as long as we at least modified the artifact
		// attributes. However, if it wasn't an artifact, and we also
		// couldn't modify the filesystem, return the error.
		return nil
	}

	return err
}

func modifyServerCert(newCert string, image VPImage) error {
	_, err := os.Stat(newCert)
	if err != nil {
		return errors.Wrap(err, "invalid server certificate")
	}
	return CopyIntoImage(newCert, image, "/etc/mender/server.crt")
}

func modifyVerificationKey(newKey string, image VPImage) error {
	_, err := os.Stat(newKey)
	if err != nil {
		return errors.Wrapf(err, "invalid verification key")
	}
	return CopyIntoImage(newKey, image, "/etc/mender/artifact-verify-key.pem")
}

func modifyMenderConfVar(confKey, confValue string, image VPImage) error {
	confFile := "/etc/mender/mender.conf"

	dir, err := ioutil.TempDir("", "")
	if err != nil {
		return err
	}
	defer os.RemoveAll(dir)

	localFile := filepath.Join(dir, filepath.Base(confFile))

	err = CopyFromImage(image, confFile, localFile)
	if err != nil {
		return err
	}

	raw, err := ioutil.ReadFile(localFile)
	if err != nil {
		return err
	}

	var rawData interface{}
	if err = json.Unmarshal(raw, &rawData); err != nil {
		return err
	}
	rawData.(map[string]interface{})[confKey] = confValue

	data, err := json.Marshal(&rawData)
	if err != nil {
		return err
	}

	if err = ioutil.WriteFile(localFile, data, 0755); err != nil {
		return err
	}

	return CopyIntoImage(localFile, image, confFile)
}

func modifyExisting(c *cli.Context, image VPImage) error {
	if c.String("name") != "" {
		if err := modifyArtifactInfoName(c.String("name"), image); err != nil {
			return err
		}
	}

	if c.String("server-uri") != "" {
		if err := modifyMenderConfVar("ServerURL",
			c.String("server-uri"), image); err != nil {
			return err
		}
	}

	if c.String("server-cert") != "" {
		if err := modifyServerCert(c.String("server-cert"), image); err != nil {
			return err
		}
	}

	if c.String("verification-key") != "" {
		if err := modifyVerificationKey(c.String("verification-key"), image); err != nil {
			return err
		}
	}

	if c.String("tenant-token") != "" {
		if err := modifyMenderConfVar("TenantToken",
			c.String("tenant-token"), image); err != nil {
			return err
		}
	}

	return nil
}
