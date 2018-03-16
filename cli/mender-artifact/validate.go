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
	"fmt"
	"io"
	"os"

	"github.com/pkg/errors"

	"github.com/mendersoftware/mender-artifact/areader"
	"github.com/mendersoftware/mender-artifact/artifact"
	"github.com/urfave/cli"
)

var ErrInvalidSignature = errors.New("error validating signature")

func validate(art io.Reader, key []byte) error {
	// do not return error immediately if we can not validate signature;
	// just continue checking consistency and return info if
	// signature verification failed
	var validationError error
	verify := func(message, sig []byte) error {
		verifyCallback := func(message, sig []byte) error {
			return errors.New("artifact is signed but no verification key was provided")
		}
		if key != nil {
			s := artifact.NewVerifier(key)
			verifyCallback = s.Verify
		}

		if verifyCallback != nil {
			if err := verifyCallback(message, sig); err != nil {
				validationError = err
			}
		}
		return nil
	}

	ar := areader.NewReader(art)
	ar.VerifySignatureCallback = verify
	if err := ar.ReadArtifact(); err != nil {
		return err
	}
	if validationError != nil {
		Log.Debug("error validating signature: %s", validationError.Error())
		return ErrInvalidSignature
	}
	return nil
}

func validateArtifact(c *cli.Context) error {
	if c.NArg() == 0 {
		return cli.NewExitError("Nothing specified, nothing validated. \nMaybe you wanted"+
			" to say 'artifacts validate <pathspec>'?", errArtifactInvalidParameters)
	}

	key, err := getKey(c.String("key"))
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
