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
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path"
	"strings"

	"github.com/mendersoftware/mender-artifact/areader"
	"github.com/mendersoftware/mender-artifact/artifact"
	"github.com/mendersoftware/mender-artifact/handlers"

	"github.com/pkg/errors"
	"github.com/urfave/cli"
)

type dumpFileStore struct {
	fileDir string
	args    *[]string
}

func DumpCommand(c *cli.Context) error {
	var dumpArgs []string

	if c.NArg() != 1 {
		return cli.NewExitError("Need to specify exactly one Artifact with dump command",
			errArtifactInvalidParameters)
	}

	art, err := os.Open(c.Args().First())
	if err != nil {
		return cli.NewExitError(fmt.Sprintf(
			"Error opening Artifact: %s", err.Error()),
			errArtifactOpen)
	}
	defer art.Close()

	ar := areader.NewReader(art)

	scriptsReadCallback := func(r io.Reader, i os.FileInfo) error {
		fullPath := path.Join(c.String("scripts"), i.Name())
		script, err := os.OpenFile(fullPath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0755)
		if err != nil {
			return err
		}
		defer script.Close()

		_, err = io.Copy(script, r)
		if err != nil {
			return err
		}

		dumpArgs = append(dumpArgs, "--script", fullPath)

		return nil
	}
	if len(c.String("scripts")) > 0 {
		err = os.MkdirAll(c.String("scripts"), 0755)
		if err != nil {
			return cli.NewExitError(fmt.Sprintf(
				"Could not create directory: %s", err.Error()), errSystemError)
		}
		ar.ScriptsReadCallback = scriptsReadCallback
	}

	err = ar.ReadArtifactHeaders()
	if err != nil {
		return cli.NewExitError(fmt.Sprintf("Error dumping Artifact: %s",
			err.Error()), errArtifactInvalid)
	}

	err = dumpPayloads(c, ar, &dumpArgs)
	if err != nil {
		return err
	}

	if c.Bool("print-cmdline") {
		printCmdline(ar, dumpArgs)
	}

	return nil
}

func dumpPayloads(c *cli.Context, ar *areader.Reader, dumpArgs *[]string) error {
	handlers := ar.GetHandlers()
	if len(handlers) != 1 {
		return cli.NewExitError("The dump command can handle one payload only",
			errArtifactUnsupportedFeature)
	}

	if len(c.String("meta-data")) > 0 {
		err := dumpMetaData(c.String("meta-data"), dumpArgs, handlers)
		if err != nil {
			return err
		}
	}

	if len(c.String("files")) > 0 {
		store := &dumpFileStore{
			fileDir: c.String("files"),
			args:    dumpArgs,
		}
		for _, h := range handlers {
			h.SetUpdateStorerProducer(store)
		}
	}

	err := ar.ReadArtifactData()
	if err != nil {
		return cli.NewExitError(fmt.Sprintf("Error dumping Artifact: %s",
			err.Error()), errArtifactInvalid)
	}

	return nil
}

func dumpMetaData(metaDataDir string, dumpArgs *[]string, handlers map[int]handlers.Installer) error {
	err := os.MkdirAll(metaDataDir, 0755)
	if err != nil {
		return cli.NewExitError(fmt.Sprintf(
			"Unable to create directory: %s", err.Error()), errSystemError)
	}

	// Hardcode to 0 index for now.
	handler := handlers[0]

	for _, augmented := range []bool{false, true} {
		var metaData map[string]interface{}
		var fullPath string
		var metaDataArg string
		if augmented {
			metaData = handler.GetUpdateAugmentMetaData()
			fullPath = path.Join(metaDataDir, "0000.meta-data-augment")
			metaDataArg = "--augment-meta-data"
		} else {
			metaData = handler.GetUpdateOriginalMetaData()
			fullPath = path.Join(metaDataDir, "0000.meta-data")
			metaDataArg = "--meta-data"
		}

		if len(metaData) == 0 {
			continue
		}

		metaDataFd, err := os.OpenFile(fullPath,
			os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0644)
		if err != nil {
			return cli.NewExitError(fmt.Sprintf(
				"Unable to create meta-data file: %s", err.Error()), errSystemError)
		}
		defer metaDataFd.Close()

		w := json.NewEncoder(metaDataFd)
		err = w.Encode(metaData)
		if err != nil {
			return errors.New("Unencodeable map in dumpPayloads. Should not happen.")
		}

		*dumpArgs = append(*dumpArgs, metaDataArg, fullPath)
	}

	return nil
}

func printCmdline(ar *areader.Reader, args []string) {
	// Even if it is a rootfs payload, we use the module-image writer, since
	// this can recreate either type.
	fmt.Printf("write module-image")

	artProvs := ar.GetArtifactProvides()
	fmt.Printf(" --artifact-name %s", artProvs.ArtifactName)
	if len(artProvs.ArtifactGroup) > 0 {
		fmt.Printf(" --provides-group %s", artProvs.ArtifactGroup)
	}

	artDeps := ar.GetArtifactDepends()
	if len(artDeps.ArtifactName) > 0 {
		fmt.Printf(" --artifact-name-depends %s", strings.Join(artDeps.ArtifactName, " --artifact-name-depends "))
	}
	fmt.Printf(" --device-type %s", strings.Join(artDeps.CompatibleDevices, " --device-type "))
	if len(artDeps.ArtifactGroup) > 0 {
		fmt.Printf(" --depends-groups %s", strings.Join(artDeps.ArtifactGroup, " --depends-groups "))
	}

	handlers := ar.GetHandlers()
	handler := handlers[0]

	fmt.Printf(" --type %s", handler.GetUpdateType())

	provs := handler.GetUpdateOriginalProvides()
	if provs != nil {
		for key, value := range provs {
			fmt.Printf(" --provides %s:%s", key, value)
		}
	}

	deps := handler.GetUpdateOriginalDepends()
	if deps != nil {
		for key, value := range deps {
			fmt.Printf(" --depends %s:%s", key, value)
		}
	}

	if len(args) > 0 {
		fmt.Printf(" %s\n", strings.Join(args, " "))
	}
}

func (d *dumpFileStore) NewUpdateStorer(updateType string, payloadNum int) (handlers.UpdateStorer, error) {
	return d, nil
}

func (d *dumpFileStore) Initialize(artifactHeaders,
	artifactAugmentedHeaders artifact.HeaderInfoer,
	payloadHeaders handlers.ArtifactUpdateHeaders) error {

	return nil
}

func (d *dumpFileStore) PrepareStoreUpdate() error {
	return os.MkdirAll(d.fileDir, 0755)
}

func (d *dumpFileStore) StoreUpdate(r io.Reader, info os.FileInfo) error {
	fullPath := path.Join(d.fileDir, info.Name())
	file, err := os.OpenFile(fullPath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0644)
	if err != nil {
		return err
	}
	defer file.Close()

	_, err = io.Copy(file, r)
	if err != nil {
		return err
	}

	*d.args = append(*d.args, "--file", fullPath)

	return nil
}

func (d *dumpFileStore) FinishStoreUpdate() error {
	return nil
}
