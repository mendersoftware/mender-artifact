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
	"fmt"
	"io"
	"os"

	"github.com/pkg/errors"
	"github.com/urfave/cli"

	"github.com/mendersoftware/mender-artifact/areader"
	"github.com/mendersoftware/mender-artifact/artifact"
)

func validate(art io.Reader, key artifact.Verifier) error {
	// do not return error immediately if we can not validate signature;
	// just continue checking consistency and return info if
	// signature verification failed
	var validationError error

	ar := areader.NewReader(art)
	ar.VerifySignatureCallback = func(message, sig []byte) error {
		if key != nil {
			if err := key.Verify(message, sig); err != nil {
				validationError = err
			}
		}
		return nil
	}

	if err := ar.ReadArtifact(); err != nil {
		return err
	}
	if validationError != nil {
		return validationError
	}
	if key != nil && !ar.IsSigned {
		return errors.New("missing signature")
	}
	if key == nil && ar.IsSigned {
		return errors.New("missing verifier")
	}
	return nil
}

func validateArtifact(c *cli.Context) error {
	if c.NArg() == 0 {
		return cli.NewExitError("Nothing specified, nothing validated. \nMaybe you wanted"+
			" to say 'artifacts validate <pathspec>'?", errArtifactInvalidParameters)
	}

	key, err := getKey(c)
	if err != nil {
		return cli.NewExitError(err.Error(), errArtifactInvalidParameters)
	}

	art, err := os.Open(c.Args().First())
	if err != nil {
		return cli.NewExitError("Can not open artifact: "+err.Error(), errArtifactOpen)
	}
	defer art.Close()

	if err := validate(art, key); err != nil {
		return cli.NewExitError(err.Error(), errArtifactInvalid)
	}

	fmt.Printf("Artifact file '%s' validated successfully\n", c.Args().First())
	return nil
}
