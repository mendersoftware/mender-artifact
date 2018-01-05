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
	"errors"
	"fmt"
	"os"

	"github.com/mendersoftware/mender-artifact/areader"
	"github.com/mendersoftware/mender-artifact/artifact"
	"github.com/urfave/cli"
)

func validateArtifact(c *cli.Context) error {
	if c.NArg() == 0 {
		return cli.NewExitError("Nothing specified, nothing validated. \nMaybe you wanted"+
			" to say 'artifacts validate <pathspec>'?", 1)
	}

	var key []byte
	var err error
	if c.String("key") != "" {
		key, err = getKey(c.String("key"))
		if err != nil {
			return cli.NewExitError("Can not read key: "+err.Error(), 1)
		}
	}

	if err := checkIfValid(c.Args().First(), key); err != nil {
		return cli.NewExitError(err.Error(), 1)
	}

	fmt.Printf("Artifact file '%s' validated successfully\n", c.Args().First())
	return nil
}

func checkIfValid(artifactPath string, key []byte) *artifactError {
	verifyCallback := func(message, sig []byte) error {
		return errors.New("artifact is signed but no verification key was provided")
	}

	if key != nil {
		s := artifact.NewVerifier(key)
		verifyCallback = s.Verify
	}

	// do not return error immediately if we can not validate signature;
	// just continue checking consistency and return info if
	// signature verification failed
	var validationError error
	ver := func(message, sig []byte) error {
		if verifyCallback != nil {
			if err := verifyCallback(message, sig); err != nil {
				validationError = err
			}
		}
		return nil
	}

	f, err := os.Open(artifactPath)
	if err != nil {
		return &artifactError{err: err}
	}
	defer f.Close()

	ar := areader.NewReader(f)
	_, err = read(ar, ver, nil)
	if err != nil {
		return &artifactError{err: err}
	}

	if validationError != nil {
		return &artifactError{
			err: fmt.Errorf("artifact file '%s' formatted correctly, "+
				"but error validating signature: %s", artifactPath, validationError),
			badSignature: true,
		}
	}
	return nil
}
