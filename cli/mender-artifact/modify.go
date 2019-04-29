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

func modifyArtifact(c *cli.Context) error {
	comp, err := artifact.NewCompressorFromId(c.GlobalString("compression"))
	if err != nil {
		return cli.NewExitError("compressor '"+c.GlobalString("compression")+"' is not supported: "+err.Error(), 1)
	}

	if c.NArg() == 0 {
		return cli.NewExitError("Nothing specified, nothing will be modified. \n"+
			"Maybe you wanted to say 'artifacts read <pathspec>'?", 1)
	}

	if _, err := os.Stat(c.Args().First()); err != nil && os.IsNotExist(err) {
		return cli.NewExitError("File ["+c.Args().First()+"] does not exist.", 1)
	}

	modifyCandidates, isArtifact, err :=
		getCandidatesForModify(c.Args().First(), nil)

	if err != nil {
		return cli.NewExitError("Error selecting images for modification: "+err.Error(), 1)
	}
	// strip the data and boot partitions
	if isArtifact {
		modifyCandidates = modifyCandidates[0:1]
		for _, mc := range modifyCandidates {
			defer os.Remove(mc.path)
		}
	} else if len(modifyCandidates) == 4 { // sdimg
		modifyCandidates = modifyCandidates[1:3]
	}

	for _, toModify := range modifyCandidates {
		if err := modifyExisting(c, toModify.path); err != nil {
			return cli.NewExitError("Error modifying artifact["+toModify.path+"]: "+
				err.Error(), 1)
		}
	}

	if len(modifyCandidates) > 1 {
		// make modified images part of sdimg again
		if err := repackSdimg(modifyCandidates, c.Args().First()); err != nil {
			return cli.NewExitError("Can not recreate sdimg file: "+err.Error(), 1)
		}
		return nil
	}

	if isArtifact {
		// re-create the artifact
		err := repackArtifact(comp, c.Args().First(), modifyCandidates[0].path,
			"", c.String("name"))
		if err != nil {
			return cli.NewExitError("Can not recreate artifact: "+err.Error(), 1)
		}
	}
	return nil
}

// oblivious to whether the file exists beforehand
func modifyName(name, image string) error {
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

	return debugfsReplaceFile("/etc/mender/artifact_info",
		tmpNameFile.Name(), image)
}

func modifyServerCert(newCert, image string) error {
	_, err := os.Stat(newCert)
	if err != nil {
		return errors.Wrap(err, "invalid server certificate")
	}
	return debugfsReplaceFile("/etc/mender/server.crt", newCert, image)
}

func modifyVerificationKey(newKey, image string) error {
	_, err := os.Stat(newKey)
	if err != nil {
		return errors.Wrapf(err, "invalid verification key")
	}
	return debugfsReplaceFile("/etc/mender/artifact-verify-key.pem", newKey, image)
}

func modifyMenderConfVar(confKey, confValue, image string) error {
	confFile := "/etc/mender/mender.conf"
	dir, err := debugfsCopyFile(confFile, image)
	if err != nil {
		return err
	}
	defer os.RemoveAll(dir)

	raw, err := ioutil.ReadFile(filepath.Join(dir, filepath.Base(confFile)))
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

	if err = ioutil.WriteFile(filepath.Join(dir, filepath.Base(confFile)), data, 0755); err != nil {
		return err
	}

	return debugfsReplaceFile(confFile, filepath.Join(dir,
		filepath.Base(confFile)), image)
}

func modifyExisting(c *cli.Context, image string) error {
	if err := debugfsRunFsck(image); err != nil {
		return err
	}
	if c.String("name") != "" {
		if err := modifyName(c.String("name"), image); err != nil {
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
