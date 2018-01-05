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
	"os"

	"github.com/urfave/cli"
)

func modifyArtifact(c *cli.Context) error {
	if c.NArg() == 0 {
		return cli.NewExitError("Nothing specified, nothing will be modified. \n"+
			"Maybe you wanted to say 'artifacts read <pathspec>'?", 1)
	}

	if _, err := os.Stat(c.Args().First()); err != nil && os.IsNotExist(err) {
		return cli.NewExitError("File ["+c.Args().First()+"] does not exist.", 1)
	}

	pubKey, err := processModifyKey(c.String("key"))
	if err != nil {
		return cli.NewExitError("Error processing private key: "+err.Error(), 1)
	}

	modifyCandidates, isArtifact, err :=
		getCandidatesForModify(c.Args().First(), pubKey)

	if err != nil {
		return cli.NewExitError("Error selecting images for modification: "+err.Error(), 1)
	}

	if len(modifyCandidates) > 1 || isArtifact {
		for _, mc := range modifyCandidates {
			defer os.Remove(mc.path)
		}
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
		err := repackArtifact(c.Args().First(), modifyCandidates[0].path,
			c.String("key"), c.String("name"))
		if err != nil {
			return cli.NewExitError("Can not recreate artifact: "+err.Error(), 1)
		}
	}
	return nil
}
