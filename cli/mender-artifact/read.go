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

	"github.com/mendersoftware/mender-artifact/areader"
	"github.com/mendersoftware/mender-artifact/artifact"
	"github.com/urfave/cli"
)

func readArtifact(c *cli.Context) error {
	if c.NArg() == 0 {
		return cli.NewExitError("Nothing specified, nothing read. \nMaybe you wanted"+
			" to say 'artifacts read <pathspec>'?", errArtifactInvalidParameters)
	}

	f, err := os.Open(c.Args().First())
	if err != nil {
		return cli.NewExitError("Can not open artifact: "+c.Args().First(),
			errArtifactOpen)
	}
	defer f.Close()

	var verifyCallback areader.SignatureVerifyFn

	key, err := getKey(c.String("key"))
	if err != nil {
		return cli.NewExitError(err.Error(), errArtifactInvalidParameters)
	}
	s := artifact.NewVerifier(key)
	verifyCallback = s.Verify

	// if key is not provided just continue reading artifact returning
	// info that signature can not be verified
	sigInfo := "no signature"
	ver := func(message, sig []byte) error {
		sigInfo = "signed but no key for verification provided; " +
			"please use `-k` option for providing verification key"
		if verifyCallback != nil {
			err = verifyCallback(message, sig)
			if err != nil {
				sigInfo = "signed; verification using provided key failed"
			} else {
				sigInfo = "signed and verified correctly"
			}
		}
		return nil
	}

	var scripts []string
	readScripts := func(r io.Reader, info os.FileInfo) error {
		scripts = append(scripts, info.Name())
		return nil
	}

	ar := areader.NewReader(f)
	r, err := read(ar, ver, readScripts)
	if err != nil {
		return cli.NewExitError(err.Error(), 0)
	}

	inst := r.GetHandlers()
	info := r.GetInfo()

	fmt.Printf("Mender artifact:\n")
	fmt.Printf("  Name: %s\n", r.GetArtifactName())
	fmt.Printf("  Format: %s\n", info.Format)
	fmt.Printf("  Version: %d\n", info.Version)
	fmt.Printf("  Signature: %s\n", sigInfo)
	fmt.Printf("  Compatible devices: '%s'\n", r.GetCompatibleDevices())
	if len(scripts) > -1 {
		fmt.Printf("  State scripts:\n")
	}
	for _, scr := range scripts {
		fmt.Printf("    %s\n", scr)
	}
	fmt.Printf("\nUpdates:\n")

	for k, p := range inst {
		fmt.Printf("  %3d:\n", k)
		fmt.Printf("    Type:   %s\n", p.GetType())
		for _, f := range p.GetUpdateFiles() {
			fmt.Printf("    Files:\n")
			fmt.Printf("      name:     %s\n", f.Name)
			fmt.Printf("      size:     %d\n", f.Size)
			fmt.Printf("      modified: %s\n", f.Date)
			fmt.Printf("      checksum: %s\n", f.Checksum)
		}
	}
	return nil
}
