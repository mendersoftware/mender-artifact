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
	"io/ioutil"
	"os"

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

	tFile, err := ioutil.TempFile("", "mender-artifact")
	if err != nil {
		return errors.Wrap(err,
			"Can not create temporary file for storing artifact")
	}
	defer os.Remove(tFile.Name())
	defer tFile.Close()

	f, err := os.Open(c.Args().First())
	if err != nil {
		return errors.Wrapf(err, "Can not open: %s", c.Args().First())
	}
	defer f.Close()

	reader, err := repack(f, tFile, privateKey, "", "")
	if err != nil {
		return err
	}

	switch ver := reader.GetInfo().Version; ver {
	case 1:
		return cli.NewExitError("Can not sign v1 artifact", 1)
	case 2:
		if reader.IsSigned && !c.Bool("force") {
			return cli.NewExitError("Trying to sign already signed artifact; "+
				"please use force option", 1)
		}
	default:
		return cli.NewExitError("Unsupported version of artifact file: "+string(ver), 1)
	}

	if err = tFile.Close(); err != nil {
		return err
	}

	name := c.Args().First()
	if len(c.String("output-path")) > 0 {
		name = c.String("output-path")
	}

	err = os.Rename(tFile.Name(), name)
	if err != nil {
		return cli.NewExitError("Can not store signed artifact: "+err.Error(), 1)
	}
	return nil
}
