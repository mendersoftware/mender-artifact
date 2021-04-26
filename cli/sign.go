// Copyright 2021 Northern.tech AS
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
	"io/ioutil"
	"os"
	"path/filepath"

	"github.com/mendersoftware/mender-artifact/awriter"
	"github.com/pkg/errors"
	"github.com/urfave/cli"
)

func signExisting(c *cli.Context) error {
	if c.NArg() == 0 {
		return cli.NewExitError("Nothing specified, nothing signed. \nMaybe you wanted"+
			" to say 'artifacts sign <pathspec>'?", 1)
	}

	if len(c.String("key")) == 0 {
		return cli.NewExitError("Missing signing key; "+
			"please use `-k` parameter for providing one", 1)
	}

	privateKey, err := getKey(c.String("key"))
	if err != nil {
		return cli.NewExitError("Can not use signing key provided: "+err.Error(), 1)
	}

	artFile := c.Args().First()
	outputFile := artFile
	if len(c.String("output-path")) > 0 {
		outputFile = c.String("output-path")
	}

	tFile, err := ioutil.TempFile(filepath.Dir(outputFile), "mender-artifact")
	if err != nil {
		err = errors.Wrap(err, "Can not create temporary file for storing artifact")
		cli.NewExitError(err, 1)
	}
	defer os.Remove(tFile.Name())
	defer tFile.Close()

	f, err := os.Open(artFile)
	if err != nil {
		err = errors.Wrapf(err, "Can not open: %s", artFile)
		return cli.NewExitError(err, 1)
	}
	defer f.Close()

	err = awriter.SignExisting(f, tFile, privateKey, c.Bool("force"))
	if err == awriter.ErrAlreadyExistingSignature {
		return cli.NewExitError("Artifact already signed, refusing to re-sign. Use force option to override", 1)
	} else if err != nil {
		return cli.NewExitError(err, 1)
	}

	if err = tFile.Close(); err != nil {
		return err
	}

	err = os.Rename(tFile.Name(), outputFile)
	if err != nil {
		return cli.NewExitError("Can not store signed artifact: "+err.Error(), 1)
	}
	return nil
}
