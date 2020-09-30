// Copyright 2020 Northern.tech AS
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
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"reflect"
	"sort"
	"strings"

	"github.com/mendersoftware/mender-artifact/areader"
	"github.com/mendersoftware/mender-artifact/artifact"
	"github.com/mendersoftware/mender-artifact/handlers"
	"github.com/pkg/errors"
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
		if c.String("key") != "" {
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
	ar.ScriptsReadCallback = readScripts
	ar.VerifySignatureCallback = ver
	err = ar.ReadArtifact()
	if err != nil {
		if errors.Cause(err) == artifact.ErrCompatibleDevices {
			return cli.NewExitError("Invalid Artifact. No 'device-type' found.", 1)
		}
		return cli.NewExitError(err.Error(), 1)
	}

	inst := ar.GetHandlers()
	info := ar.GetInfo()

	fmt.Printf("Mender artifact:\n")
	fmt.Printf("  Name: %s\n", ar.GetArtifactName())
	fmt.Printf("  Format: %s\n", info.Format)
	fmt.Printf("  Version: %d\n", info.Version)
	fmt.Printf("  Signature: %s\n", sigInfo)
	fmt.Printf("  Compatible devices: '%s'\n", ar.GetCompatibleDevices())
	provides := ar.GetArtifactProvides()
	if provides != nil {
		fmt.Printf("  Provides group: %s\n", provides.ArtifactGroup)
	}

	depends := ar.GetArtifactDepends()
	if depends != nil {
		fmt.Printf("  Depends on one of artifact(s): [%s]\n", strings.Join(depends.ArtifactName, ", "))
		fmt.Printf("  Depends on one of group(s): [%s]\n", strings.Join(depends.ArtifactGroup, ", "))
	}

	if len(scripts) > -1 {
		fmt.Printf("  State scripts:\n")
	}
	for _, scr := range scripts {
		fmt.Printf("    %s\n", scr)
	}

	fmt.Printf("\nUpdates:\n")
	for k, p := range inst {
		printPayload(k, p)
	}
	return nil
}

func sortedKeys(mapWithKeys interface{}) sort.StringSlice {
	var keys sort.StringSlice
	mapVal := reflect.ValueOf(mapWithKeys)
	if mapVal.Kind() != reflect.Map {
		return nil
	}
	keys = make([]string, mapVal.Len())
	keysVal := mapVal.MapKeys()
	for i, keyVal := range keysVal {
		keys[i] = keyVal.String()
	}
	keys.Sort()
	return keys
}

func printPayload(index int, p handlers.Installer) {
	fmt.Printf("  %3d:\n", index)
	fmt.Printf("    Type:   %s\n", p.GetUpdateType())

	provides, err := p.GetUpdateProvides()
	fmt.Printf("    Provides:")
	if err != nil {
		fmt.Printf(" Invalid provides section: %s\n", err.Error())
	} else if provides == nil || len(provides) == 0 {
		fmt.Printf(" Nothing\n")
	} else {
		providesKeys := sortedKeys(provides)

		fmt.Printf("\n")
		for _, provideKey := range providesKeys {
			fmt.Printf("\t%s: %s\n", provideKey, (provides)[provideKey])
		}
	}

	depends, err := p.GetUpdateDepends()
	fmt.Printf("    Depends:")
	if err != nil {
		fmt.Printf(" Invalid depends section: %s\n", err.Error())
	} else if depends == nil || len(depends) == 0 {
		fmt.Printf(" Nothing\n")
	} else {
		dependsKeys := sortedKeys(depends)

		fmt.Printf("\n")
		for _, dependKey := range dependsKeys {
			fmt.Printf("\t%s: %s\n", dependKey, (depends)[dependKey])
		}
	}

	caps := p.GetUpdateClearsProvides()
	if caps != nil {
		fmt.Printf("    Clears Provides: [\"%s\"]\n", strings.Join(caps, "\", \""))
	}

	metaData, err := p.GetUpdateMetaData()
	fmt.Printf("    Metadata:")
	if err != nil {
		fmt.Printf(" Invalid metadata section: %s\n", err.Error())
	} else if len(metaData) == 0 {
		fmt.Printf(" Nothing\n")
	} else {
		var metaDataSlice []byte
		if err == nil {
			metaDataSlice, err = json.Marshal(metaData)
		}
		var metaDataBuf bytes.Buffer
		if err == nil {
			err = json.Indent(&metaDataBuf, metaDataSlice, "\t", "  ")
		}
		if err != nil {
			fmt.Printf(" Invalid metadata section: %s\n", err.Error())
		} else {
			fmt.Printf("\n\t%s\n", metaDataBuf.String())
		}
	}

	if len(p.GetUpdateAllFiles()) == 0 {
		fmt.Printf("    Files: None\n")
	} else {
		fmt.Printf("    Files:\n")
		for _, f := range p.GetUpdateAllFiles() {
			fmt.Printf("      name:     %s\n", f.Name)
			fmt.Printf("      size:     %d\n", f.Size)
			fmt.Printf("      modified: %s\n", f.Date)
			fmt.Printf("      checksum: %s\n", f.Checksum)
		}
	}
}
