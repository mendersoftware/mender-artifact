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

func extractKeyValuesIfArtifact(ctx *cli.Context, key string, image VPImage) (*map[string]string, error) {
	keyValues, err := extractKeyValues(ctx.StringSlice(key))
	if keyValues == nil || err != nil {
		return nil, err
	}

	_, ok := image.(*ModImageArtifact)
	if !ok {
		return nil, errors.Errorf("Argument `--%s` must be used with an Artifact", key)
	}

	return keyValues, nil
}

func modifyExisting(c *cli.Context, image VPImage) error {
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

	err := modifyArtifactAttributes(c, image)
	if err != nil {
		return err
	}

	err = modifyPayloadAttributes(c, image)
	if err != nil {
		return err
	}

	return nil
}

func modifyArtifactAttributes(c *cli.Context, image VPImage) error {
	if c.String("artifact-name") != "" {
		if err := modifyArtifactInfoName(c.String("artifact-name"), image); err != nil {
			return err
		}
	}

	art, isArt := image.(*ModImageArtifact)

	if c.IsSet("artifact-name-depends") {
		if !isArt {
			return errors.New("`--artifact-name-depends` argument must be used with an Artifact")
		}
		art.writeArgs.Depends.ArtifactName = c.StringSlice("artifact-name-depends")
	}

	if c.IsSet("depends-groups") {
		if !isArt {
			return errors.New("`--depends-groups` argument must be used with an Artifact")
		}
		art.writeArgs.Depends.ArtifactGroup = c.StringSlice("depends-groups")
	}

	if c.IsSet("provides-group") {
		if !isArt {
			return errors.New("`--provides-group` argument must be used with an Artifact")
		}
		art.writeArgs.Provides.ArtifactGroup = c.String("provides-group")
	}

	return nil
}

func modifyPayloadAttributes(c *cli.Context, image VPImage) error {
	art, isArt := image.(*ModImageArtifact)

	keyValues, err := extractKeyValuesIfArtifact(c, "depends", image)
	if err != nil {
		return err
	} else if keyValues != nil {
		typeInfoDepends, err := artifact.NewTypeInfoDepends(*keyValues)
		if err != nil {
			return err
		}
		// The unconditional cast usage here is safe due to the
		// `extractKeyValuesIfArtifact` call above.
		art.writeArgs.TypeInfoV3.ArtifactDepends = typeInfoDepends
	}

	keyValues, err = extractKeyValuesIfArtifact(c, "provides", image)
	if err != nil {
		return err
	} else if keyValues != nil {
		typeInfoProvides, err := artifact.NewTypeInfoProvides(*keyValues)
		if err != nil {
			return err
		}
		art.writeArgs.TypeInfoV3.ArtifactProvides = typeInfoProvides
	}

	keyValues, err = extractKeyValuesIfArtifact(c, "augment-depends", image)
	if err != nil {
		return err
	} else if keyValues != nil {
		typeInfoDepends, err := artifact.NewTypeInfoDepends(*keyValues)
		if err != nil {
			return err
		}
		art.writeArgs.AugmentTypeInfoV3.ArtifactDepends = typeInfoDepends
	}

	keyValues, err = extractKeyValuesIfArtifact(c, "augment-provides", image)
	if err != nil {
		return err
	} else if keyValues != nil {
		typeInfoProvides, err := artifact.NewTypeInfoProvides(*keyValues)
		if err != nil {
			return err
		}
		art.writeArgs.AugmentTypeInfoV3.ArtifactProvides = typeInfoProvides
	}

	metaData, augMetaData, err := makeMetaData(c)
	if err != nil {
		return err
	}
	if metaData != nil {
		if !isArt {
			return errors.New("`--meta-data` argument must be used with an Artifact")
		}
		art.writeArgs.MetaData = metaData
	}
	if augMetaData != nil {
		if !isArt {
			return errors.New("`--augment-meta-data` argument must be used with an Artifact")
		}
		art.writeArgs.AugmentMetaData = augMetaData
	}

	return nil
}
