// Copyright 2022 Northern.tech AS
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
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"reflect"
	"sort"
	"strings"

	"github.com/pkg/errors"
	"github.com/urfave/cli"

	"github.com/mendersoftware/mender-artifact/areader"
	"github.com/mendersoftware/mender-artifact/artifact"
	"github.com/mendersoftware/mender-artifact/handlers"
	"github.com/mendersoftware/mender-artifact/utils"
)

var defaultIndentation = "  "

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

func printList(title string, iterable []string, err string, shouldFlow bool, indentationLevel int) {
	fmt.Printf("%s%s:", strings.Repeat(defaultIndentation, indentationLevel), title)
	if len(err) > 0 {
		fmt.Printf("%s\n", err)
	} else if len(iterable) == 0 {
		fmt.Printf(" []\n")
	} else if shouldFlow {
		fmt.Printf(" [%s]\n", strings.Join(iterable, ", "))
	} else {
		fmt.Printf("\n")
		for _, value := range iterable {
			fmt.Printf("%s- %s\n", strings.Repeat(defaultIndentation, indentationLevel+1), value)
		}
	}
}

func printObject(
	title string,
	someObject map[string]interface{},
	err string,
	indentationLevel int,
) {
	fmt.Printf("%s%s:", strings.Repeat(defaultIndentation, indentationLevel), title)
	if len(err) > 0 {
		fmt.Printf("%s\n", err)
	} else if len(someObject) == 0 {
		fmt.Printf(" {}\n")
	} else {
		fmt.Printf("\n")
		keys := sortedKeys(someObject)
		for _, key := range keys {
			fmt.Printf("%s%s: %s\n",
				strings.Repeat(defaultIndentation, indentationLevel+1), key, (someObject)[key])
		}
	}
}

func printUnnamedObject(
	someObject map[string]interface{},
	indentationLevel int,
) {
	if len(someObject) == 0 {
		fmt.Printf("%s- {}\n", strings.Repeat(defaultIndentation, indentationLevel))
	} else {
		keys := sortedKeys(someObject)
		for index, key := range keys {
			entry := fmt.Sprintf("%s: %s", key, (someObject)[key])
			if index == 0 {
				fmt.Printf("%s- %s\n", strings.Repeat(defaultIndentation, indentationLevel), entry)
				continue
			}
			// here we assume indentationLevel is 2 spaces to increase by the 2 character length
			// the list indicator "- " inserted above has
			fmt.Printf("%s%s\n", strings.Repeat(defaultIndentation, indentationLevel+1), entry)
		}
	}
}

func printHeader(ar *areader.Reader, sigInfo string, indentationLevel int) {
	info := ar.GetInfo()
	fmt.Printf("%sMender Artifact:\n", strings.Repeat(defaultIndentation, indentationLevel))
	fmt.Printf(
		"%sName: %s\n",
		strings.Repeat(defaultIndentation, indentationLevel+1),
		ar.GetArtifactName(),
	)
	fmt.Printf(
		"%sFormat: %s\n",
		strings.Repeat(defaultIndentation, indentationLevel+1),
		info.Format,
	)
	fmt.Printf(
		"%sVersion: %d\n",
		strings.Repeat(defaultIndentation, indentationLevel+1),
		info.Version,
	)
	fmt.Printf("%sSignature: %s\n", strings.Repeat(defaultIndentation, indentationLevel+1), sigInfo)
	printList("Compatible devices", ar.GetCompatibleDevices(), "", true, indentationLevel+1)
}

func printStateScripts(scripts []string, indentationLevel int) {
	printList("State scripts", scripts, "", false, indentationLevel)
}

func printFiles(files []*handlers.DataFile, indentationLevel int) {
	if len(files) == 0 {
		fmt.Printf("%sFiles: []\n", strings.Repeat(defaultIndentation, indentationLevel))
	} else {
		fmt.Printf("%sFiles:\n", strings.Repeat(defaultIndentation, indentationLevel))
		for _, f := range files {
			data := map[string]interface{}{
				"name":     f.Name,
				"size":     fmt.Sprintf("%d", f.Size),
				"modified": f.Date,
				"checksum": f.Checksum,
			}
			printUnnamedObject(data, indentationLevel+1)
		}
	}
}

func printProvides(p handlers.Installer, indentationLevel int) {
	provides, err := p.GetUpdateProvides()
	error := ""
	if err != nil {
		error = fmt.Sprintf(" Invalid provides section: %s", err.Error())
	}
	providesWorkaround := make(map[string]interface{}, len(provides))
	for k, v := range provides {
		providesWorkaround[k] = v
	}
	printObject("Provides", providesWorkaround, error, indentationLevel)
}

func printDepends(p handlers.Installer, indentationLevel int) {
	depends, err := p.GetUpdateDepends()
	error := ""
	if err != nil {
		error = fmt.Sprintf(" Invalid depends section: %s", err.Error())
	}
	printObject("Depends", depends, error, indentationLevel)
}

func printClearsProvides(p handlers.Installer, indentationLevel int) {
	caps := p.GetUpdateClearsProvides()
	printList("Clears Provides", caps, "", true, indentationLevel)
}

func printUpdateMetadata(p handlers.Installer, indentationLevel int) {
	metaData, err := p.GetUpdateMetaData()
	fmt.Printf("%sMetadata:", strings.Repeat(defaultIndentation, indentationLevel))
	if err != nil {
		fmt.Printf(" Invalid metadata section: %s\n", err.Error())
	} else if len(metaData) == 0 {
		fmt.Printf(" {}\n")
	} else {
		var metaDataSlice []byte
		if err == nil {
			metaDataSlice, err = json.Marshal(metaData)
		}
		var metaDataBuf bytes.Buffer
		if err == nil {
			err = json.Indent(
				&metaDataBuf,
				metaDataSlice,
				strings.Repeat(defaultIndentation, indentationLevel+1),
				defaultIndentation)
		}
		if err != nil {
			fmt.Printf(" Invalid metadata section: %s\n", err.Error())
		} else {
			fmt.Printf("\n")
			fmt.Printf(
				"%s%s\n",
				strings.Repeat(defaultIndentation, indentationLevel+1),
				metaDataBuf.String())
		}
	}
}

func printType(p handlers.Installer, indentationLevel int) {
	updateType := p.GetUpdateType()
	if updateType == nil {
		emptyType := "Empty type"
		updateType = &emptyType
	}
	fmt.Printf(
		"%s- Type: %v\n",
		strings.Repeat(defaultIndentation, indentationLevel),
		*updateType,
	)
}

func printPayload(p handlers.Installer, indentationLevel int) {
	// here we assume indentationLevel is 2 spaces so the initial entry can omit
	// the indentation increase and rely on the 2 character length of the list item indicator "- "
	printType(p, indentationLevel)
	printProvides(p, indentationLevel+1)
	printDepends(p, indentationLevel+1)
	printClearsProvides(p, indentationLevel+1)
	printUpdateMetadata(p, indentationLevel+1)
	printFiles(p.GetUpdateAllFiles(), indentationLevel+1)
}

func printUpdates(updatePayloads map[int]handlers.Installer, indentationLevel int) {
	fmt.Printf("%sUpdates:\n", strings.Repeat(defaultIndentation, indentationLevel))
	for _, payload := range updatePayloads {
		printPayload(payload, indentationLevel+1)
	}
}

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

	key, err := getKey(c)
	if err != nil {
		return cli.NewExitError(err.Error(), errArtifactInvalidParameters)
	}
	if key != nil {
		verifyCallback = key.Verify
	}

	// if key is not provided just continue reading artifact returning
	// info that signature can not be verified
	sigInfo := "no signature"
	ver := func(message, sig []byte) error {
		sigInfo = "signed but no key for verification provided; " +
			"please use `-k` option for providing verification key"
		if key != nil {
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
	if !c.Bool("no-progress") {
		fmt.Fprintln(os.Stderr, "Reading Artifact...")
		ar.ProgressReader = utils.NewProgressReader()
	}
	ar.ScriptsReadCallback = readScripts
	ar.VerifySignatureCallback = ver
	err = ar.ReadArtifact()
	if err != nil {
		if errors.Cause(err) == artifact.ErrCompatibleDevices {
			return cli.NewExitError("Invalid Artifact. No 'device-type' found.", 1)
		}
		return cli.NewExitError(err.Error(), 1)
	}

	printHeader(ar, sigInfo, 0)

	provides := ar.GetArtifactProvides()
	if provides != nil {
		fmt.Printf("%sProvides group: %s\n", defaultIndentation, provides.ArtifactGroup)
	}

	depends := ar.GetArtifactDepends()
	if depends != nil {
		fmt.Printf(
			"%sDepends on one of artifact(s): [%s]\n",
			defaultIndentation, strings.Join(depends.ArtifactName, ", "),
		)
		fmt.Printf(
			"%sDepends on one of group(s): [%s]\n",
			defaultIndentation, strings.Join(depends.ArtifactGroup, ", "),
		)
	}

	printStateScripts(scripts, 1)
	fmt.Println()
	updatePayloads := ar.GetHandlers()
	printUpdates(updatePayloads, 0)

	return nil
}
