// Copyright 2019 Northern.tech AS
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
	"os"
	"strings"

	"github.com/mendersoftware/mender-artifact/artifact"
	"github.com/mendersoftware/mender-artifact/awriter"
	"github.com/mendersoftware/mender-artifact/handlers"
	"github.com/pkg/errors"
	"github.com/urfave/cli"
)

func validateInput(c *cli.Context) error {
	// Version 2 and 3 validation.
	if len(c.StringSlice("device-type")) == 0 ||
		len(c.String("artifact-name")) == 0 ||
		len(c.String("file")) == 0 {
		return cli.NewExitError(
			"must provide `device-type`, `artifact-name` and `update`",
			errArtifactInvalidParameters,
		)
	}
	if len(strings.Fields(c.String("artifact-name"))) > 1 {
		// check for whitespace in artifact-name
		return cli.NewExitError(
			"whitespace is not allowed in the artifact-name",
			errArtifactInvalidParameters,
		)
	}
	return nil
}

func writeRootfs(c *cli.Context) error {
	comp, err := artifact.NewCompressorFromId(c.GlobalString("compression"))
	if err != nil {
		return cli.NewExitError("compressor '"+c.GlobalString("compression")+"' is not supported: "+err.Error(), 1)
	}

	if err := validateInput(c); err != nil {
		Log.Error(err.Error())
		return err
	}

	// set the default name
	name := "artifact.mender"
	if len(c.String("output-path")) > 0 {
		name = c.String("output-path")
	}
	version := c.Int("version")

	Log.Debugf("creating artifact [%s], version: %d", name, version)

	var h handlers.Composer
	switch version {
	case 1:
		return cli.NewExitError("Error: Mender-Artifact version 1 is not supported", errArtifactUnsupportedVersion)
	case 2:
		h = handlers.NewRootfsV2(c.String("file"))
	case 3:
		h = handlers.NewRootfsV3(c.String("file"))
	default:
		return cli.NewExitError(
			fmt.Sprintf("unsupported artifact version: %v", version),
			errArtifactUnsupportedVersion,
		)
	}

	upd := &awriter.Updates{
		Updates: []handlers.Composer{h},
	}

	f, err := os.Create(name + ".tmp")
	if err != nil {
		return cli.NewExitError("can not create artifact file", errArtifactCreate)
	}
	defer func() {
		f.Close()
		// in case of success `.tmp` suffix will be removed and below
		// will not remove valid artifact
		os.Remove(name + ".tmp")
	}()

	aw, err := artifactWriter(comp, f, c.String("key"), version)
	if err != nil {
		return cli.NewExitError(err.Error(), 1)
	}

	scr, err := scripts(c.StringSlice("script"))
	if err != nil {
		return cli.NewExitError(err.Error(), 1)
	}

	depends := artifact.ArtifactDepends{
		ArtifactName:      c.StringSlice("artifact-name-depends"),
		CompatibleDevices: c.StringSlice("device-type"),
		ArtifactGroup:     c.StringSlice("depends-groups"),
	}

	provides := artifact.ArtifactProvides{
		ArtifactName:  c.String("artifact-name"),
		ArtifactGroup: c.String("provides-group"),
	}

	typeInfoV3, _, err := makeTypeInfo(c)
	if err != nil {
		return err
	}

	err = aw.WriteArtifact(
		&awriter.WriteArtifactArgs{
			Format:     "mender",
			Version:    version,
			Devices:    c.StringSlice("device-type"),
			Name:       c.String("artifact-name"),
			Updates:    upd,
			Scripts:    scr,
			Depends:    &depends,
			Provides:   &provides,
			TypeInfoV3: typeInfoV3,
		})
	if err != nil {
		return cli.NewExitError(err.Error(), 1)
	}

	f.Close()
	err = os.Rename(name+".tmp", name)
	if err != nil {
		return cli.NewExitError(err.Error(), 1)
	}
	return nil
}

func artifactWriter(comp artifact.Compressor, f *os.File, key string,
	ver int) (*awriter.Writer, error) {
	if key != "" {
		if ver == 0 {
			// check if we are having correct version
			return nil, errors.New("can not use signed artifact with version 0")
		}
		privateKey, err := getKey(key)
		if err != nil {
			return nil, err
		}
		return awriter.NewWriterSigned(f, comp, artifact.NewSigner(privateKey)), nil
	}
	return awriter.NewWriter(f, comp), nil
}

func makeUpdates(ctx *cli.Context) (*awriter.Updates, error) {
	version := ctx.Int("version")

	var handler, augmentHandler handlers.Composer
	switch version {
	case 2:
		return nil, cli.NewExitError(
			"Module images need at least artifact format version 3",
			errArtifactInvalidParameters)
	case 3:
		handler = handlers.NewModuleImage(ctx.String("type"))
	default:
		return nil, cli.NewExitError(
			fmt.Sprintf("unsupported artifact version: %v", version),
			errArtifactUnsupportedVersion,
		)
	}

	dataFiles := make([](*handlers.DataFile), 0, len(ctx.StringSlice("file")))
	for _, file := range ctx.StringSlice("file") {
		dataFiles = append(dataFiles, &handlers.DataFile{Name: file})
	}
	handler.SetUpdateFiles(dataFiles)

	upd := &awriter.Updates{
		Updates: []handlers.Composer{handler},
	}

	if ctx.String("augment-type") != "" {
		augmentHandler = handlers.NewAugmentedModuleImage(handler, ctx.String("augment-type"))
		dataFiles = make([](*handlers.DataFile), 0, len(ctx.StringSlice("augment-file")))
		for _, file := range ctx.StringSlice("augment-file") {
			dataFiles = append(dataFiles, &handlers.DataFile{Name: file})
		}
		augmentHandler.SetUpdateAugmentFiles(dataFiles)
		upd.Augments = []handlers.Composer{augmentHandler}
	}

	return upd, nil
}

func makeTypeInfo(ctx *cli.Context) (*artifact.TypeInfoV3, *artifact.TypeInfoV3, error) {
	// Make key value pairs from the type-info fields supplied on command
	// line.
	var keyValues *map[string]string

	var typeInfoDepends artifact.TypeInfoDepends
	keyValues, err := extractKeyValues(ctx.StringSlice("depends"))
	if err != nil {
		return nil, nil, err
	} else if keyValues != nil {
		if typeInfoDepends, err = artifact.NewTypeInfoDepends(*keyValues); err != nil {
			return nil, nil, err
		}
	}

	var typeInfoProvides artifact.TypeInfoProvides
	keyValues, err = extractKeyValues(ctx.StringSlice("provides"))
	if err != nil {
		return nil, nil, err
	} else if keyValues != nil {
		if typeInfoProvides, err = artifact.NewTypeInfoProvides(*keyValues); err != nil {
			return nil, nil, err
		}
	}

	var augmentTypeInfoDepends artifact.TypeInfoDepends
	keyValues, err = extractKeyValues(ctx.StringSlice("augment-depends"))
	if err != nil {
		return nil, nil, err
	} else if keyValues != nil {
		if augmentTypeInfoDepends, err = artifact.NewTypeInfoDepends(*keyValues); err != nil {
			return nil, nil, err
		}
	}

	var augmentTypeInfoProvides artifact.TypeInfoProvides
	keyValues, err = extractKeyValues(ctx.StringSlice("augment-provides"))
	if err != nil {
		return nil, nil, err
	} else if keyValues != nil {
		if augmentTypeInfoProvides, err = artifact.NewTypeInfoProvides(*keyValues); err != nil {
			return nil, nil, err
		}
	}

	typeInfoV3 := &artifact.TypeInfoV3{
		Type:             ctx.String("type"),
		ArtifactDepends:  typeInfoDepends,
		ArtifactProvides: typeInfoProvides,
	}

	if ctx.String("augment-type") == "" {
		// Non-augmented artifact
		if len(ctx.StringSlice("augment-file")) != 0 ||
			len(ctx.StringSlice("augment-depends")) != 0 ||
			len(ctx.StringSlice("augment-provides")) != 0 ||
			ctx.String("augment-meta-data") != "" {

			err = errors.New("Must give --augment-type argument if making augmented artifact")
			fmt.Println(err.Error())
			return nil, nil, err
		}
		return typeInfoV3, nil, nil
	}

	augmentTypeInfoV3 := &artifact.TypeInfoV3{
		Type:             ctx.String("augment-type"),
		ArtifactDepends:  augmentTypeInfoDepends,
		ArtifactProvides: augmentTypeInfoProvides,
	}

	return typeInfoV3, augmentTypeInfoV3, nil
}

func makeMetaData(ctx *cli.Context) (map[string]interface{}, map[string]interface{}, error) {
	var metaData map[string]interface{}
	var augmentMetaData map[string]interface{}

	if len(ctx.String("meta-data")) > 0 {
		file, err := os.Open(ctx.String("meta-data"))
		if err != nil {
			return metaData, augmentMetaData, cli.NewExitError(err, errArtifactInvalidParameters)
		}
		defer file.Close()
		dec := json.NewDecoder(file)
		err = dec.Decode(&metaData)
		if err != nil {
			return metaData, augmentMetaData, cli.NewExitError(err, errArtifactInvalidParameters)
		}
	}

	if len(ctx.String("augment-meta-data")) > 0 {
		file, err := os.Open(ctx.String("augment-meta-data"))
		if err != nil {
			return metaData, augmentMetaData, cli.NewExitError(err, errArtifactInvalidParameters)
		}
		defer file.Close()
		dec := json.NewDecoder(file)
		err = dec.Decode(&augmentMetaData)
		if err != nil {
			return metaData, augmentMetaData, cli.NewExitError(err, errArtifactInvalidParameters)
		}
	}

	return metaData, augmentMetaData, nil
}

func writeModuleImage(ctx *cli.Context) error {
	comp, err := artifact.NewCompressorFromId(ctx.GlobalString("compression"))
	if err != nil {
		return cli.NewExitError("compressor '"+ctx.GlobalString("compression")+"' is not supported: "+err.Error(), 1)
	}

	// set the default name
	name := "artifact.mender"
	if len(ctx.String("output-path")) > 0 {
		name = ctx.String("output-path")
	}
	version := ctx.Int("version")

	if version == 1 {
		return cli.NewExitError("Mender-Artifact version 1 is not supported", 1)
	}

	upd, err := makeUpdates(ctx)
	if err != nil {
		return err
	}

	f, err := os.Create(name + ".tmp")
	if err != nil {
		return cli.NewExitError("can not create artifact file", errArtifactCreate)
	}
	defer func() {
		f.Close()
		// in case of success `.tmp` suffix will be removed and below
		// will not remove valid artifact
		os.Remove(name + ".tmp")
	}()

	aw, err := artifactWriter(comp, f, ctx.String("key"), version)
	if err != nil {
		return cli.NewExitError(err.Error(), 1)
	}

	scr, err := scripts(ctx.StringSlice("script"))
	if err != nil {
		return cli.NewExitError(err.Error(), 1)
	}

	depends := artifact.ArtifactDepends{
		ArtifactName:      ctx.StringSlice("artifact-name-depends"),
		CompatibleDevices: ctx.StringSlice("device-type"),
		ArtifactGroup:     ctx.StringSlice("depends-groups"),
	}

	provides := artifact.ArtifactProvides{
		ArtifactName:  ctx.String("artifact-name"),
		ArtifactGroup: ctx.String("provides-group"),
	}

	typeInfoV3, augmentTypeInfoV3, err := makeTypeInfo(ctx)
	if err != nil {
		return err
	}

	metaData, augmentMetaData, err := makeMetaData(ctx)
	if err != nil {
		return err
	}

	err = aw.WriteArtifact(
		&awriter.WriteArtifactArgs{
			Format:            "mender",
			Version:           version,
			Devices:           ctx.StringSlice("device-type"),
			Name:              ctx.String("artifact-name"),
			Updates:           upd,
			Scripts:           scr,
			Depends:           &depends,
			Provides:          &provides,
			TypeInfoV3:        typeInfoV3,
			MetaData:          metaData,
			AugmentTypeInfoV3: augmentTypeInfoV3,
			AugmentMetaData:   augmentMetaData,
		})
	if err != nil {
		return cli.NewExitError(err.Error(), 1)
	}

	f.Close()
	err = os.Rename(name+".tmp", name)
	if err != nil {
		return cli.NewExitError(err.Error(), 1)
	}
	return nil
}

func extractKeyValues(params []string) (*map[string]string, error) {
	var keyValues *map[string]string
	if len(params) > 0 {
		keyValues = &map[string]string{}
		for _, arg := range params {
			split := strings.SplitN(arg, ":", 2)
			if len(split) != 2 {
				return nil, cli.NewExitError(
					fmt.Sprintf("argument must have a delimiting colon: %s", arg),
					errArtifactInvalidParameters)
			}
			if _, exists := (*keyValues)[split[0]]; exists {
				return nil, cli.NewExitError(
					fmt.Sprintf("argument specified more than once: %s", split[0]),
					errArtifactInvalidParameters)
			}
			(*keyValues)[split[0]] = split[1]
		}
	}
	return keyValues, nil
}
