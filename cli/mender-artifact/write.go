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
	"encoding/json"
	"os"
	"strings"

	"github.com/mendersoftware/mender-artifact/artifact"
	"github.com/mendersoftware/mender-artifact/awriter"
	"github.com/mendersoftware/mender-artifact/handlers"
	"github.com/pkg/errors"
	"github.com/urfave/cli"
)

func validateInput(c *cli.Context) error {
	// Version 1,2 and 3 validation.
	if len(c.StringSlice("device-type")) == 0 ||
		len(c.String("artifact-name")) == 0 ||
		len(c.String("update")) == 0 {
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
		h = handlers.NewRootfsV1(c.String("update"))
	case 2:
		h = handlers.NewRootfsV2(c.String("update"))
	case 3:
		h = handlers.NewRootfsV3(c.String("update"))
	default:
		return cli.NewExitError(
			fmt.Sprintf("unsupported artifact version: %v", version),
			errArtifactUnsupportedVersion,
		)
	}

	upd := &awriter.Updates{
		U: []handlers.Composer{h},
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

	aw, err := artifactWriter(f, c.String("key"), version)
	if err != nil {
		return cli.NewExitError(err.Error(), 1)
	}

	scr, err := scripts(c.StringSlice("script"))
	if err != nil {
		return cli.NewExitError(err.Error(), 1)
	} else if len(scr.Get()) != 0 && version == 1 {
		// check if we are having correct version
		return cli.NewExitError("can not use scripts artifact with version 1", 1)
	}

	// NOTE: Update-types supported is currently hardcoded into the artifact!
	updateTypesSupported := []string{"rootfs-image"}

	depends := artifact.ArtifactDepends{
		ArtifactName:      c.StringSlice("artifact-name-depends"),
		CompatibleDevices: c.StringSlice("device-type"),
		ArtifactGroup:     c.StringSlice("depends-groups"),
	}

	provides := artifact.ArtifactProvides{
		ArtifactName:         c.String("artifact-name"),
		ArtifactGroup:        c.String("provides-group"),
		SupportedUpdateTypes: updateTypesSupported,
	}

	typeInfoV3 := artifact.TypeInfoV3{
		Type:             updateTypesSupported[0],
		ArtifactDepends:  &artifact.TypeInfoDepends{"rootfs_image_checksum": c.String("depends-rootfs-image-checksum")},
		ArtifactProvides: &artifact.TypeInfoProvides{"rootfs_image_checksum": c.String("provides-rootfs-image-checksum")},
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
			TypeInfoV3: &typeInfoV3,
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

func artifactWriter(f *os.File, key string,
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
		return awriter.NewWriterSigned(f, artifact.NewSigner(privateKey)), nil
	}
	return awriter.NewWriter(f), nil
}

func writeModuleImage(ctx *cli.Context) error {
	// set the default name
	name := "artifact.mender"
	if len(ctx.String("output-path")) > 0 {
		name = ctx.String("output-path")
	}
	version := ctx.Int("version")

	var h handlers.Composer
	switch version {
	case 1, 2:
		return cli.NewExitError(
			"Module images need at least artifact format version 3",
			errArtifactInvalidParameters)
	case 3:
		h = handlers.NewModuleImage(ctx.String("type"))
	default:
		return cli.NewExitError(
			fmt.Sprintf("unsupported artifact version: %v", version),
			errArtifactUnsupportedVersion,
		)
	}

	dataFiles := make([](*handlers.DataFile), 0, len(ctx.StringSlice("file")))
	for _, file := range ctx.StringSlice("file") {
		dataFiles = append(dataFiles, &handlers.DataFile{Name: file})
	}
	h.SetUpdateFiles(dataFiles)

	dataFiles = make([](*handlers.DataFile), 0, len(ctx.StringSlice("augment-file")))
	for _, file := range ctx.StringSlice("augment-file") {
		dataFiles = append(dataFiles, &handlers.DataFile{Name: file})
	}
	h.SetUpdateAugmentFiles(dataFiles)

	upd := &awriter.Updates{
		U: []handlers.Composer{h},
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

	aw, err := artifactWriter(f, ctx.String("key"), version)
	if err != nil {
		return cli.NewExitError(err.Error(), 1)
	}

	scr, err := scripts(ctx.StringSlice("script"))
	if err != nil {
		return cli.NewExitError(err.Error(), 1)
	} else if len(scr.Get()) != 0 && version == 1 {
		// check if we are having correct version
		return cli.NewExitError("can not use scripts artifact with version 1", 1)
	}

	// NOTE: Update-types supported is currently hardcoded into the artifact!
	updateTypesSupported := []string{"TODO"}

	depends := artifact.ArtifactDepends{
		ArtifactName:      ctx.StringSlice("artifact-name-depends"),
		CompatibleDevices: ctx.StringSlice("device-type"),
		ArtifactGroup:     ctx.StringSlice("depends-groups"),
	}

	provides := artifact.ArtifactProvides{
		ArtifactName:         ctx.String("artifact-name"),
		ArtifactGroup:        ctx.String("provides-group"),
		SupportedUpdateTypes: updateTypesSupported,
	}

	// Make key value pairs from the type-info fields supplied on command
	// line.
	var keyValues *map[string]string

	var typeInfoDepends *artifact.TypeInfoDepends
	keyValues, err = extractKeyValues(ctx.StringSlice("depends"))
	if err != nil {
		return err
	} else if keyValues != nil {
		typeInfoDepends = new(artifact.TypeInfoDepends)
		*typeInfoDepends = *keyValues
	}

	var typeInfoProvides *artifact.TypeInfoProvides
	keyValues, err = extractKeyValues(ctx.StringSlice("provides"))
	if err != nil {
		return err
	} else if keyValues != nil {
		typeInfoProvides = new(artifact.TypeInfoProvides)
		*typeInfoProvides = *keyValues
	}

	var augmentTypeInfoDepends *artifact.TypeInfoDepends
	keyValues, err = extractKeyValues(ctx.StringSlice("augment-depends"))
	if err != nil {
		return err
	} else if keyValues != nil {
		augmentTypeInfoDepends = new(artifact.TypeInfoDepends)
		*augmentTypeInfoDepends = *keyValues
	}

	var augmentTypeInfoProvides *artifact.TypeInfoProvides
	keyValues, err = extractKeyValues(ctx.StringSlice("augment-provides"))
	if err != nil {
		return err
	} else if keyValues != nil {
		augmentTypeInfoProvides = new(artifact.TypeInfoProvides)
		*augmentTypeInfoProvides = *keyValues
	}

	typeInfoV3 := artifact.TypeInfoV3{
		Type:             ctx.String("type"),
		ArtifactDepends:  typeInfoDepends,
		ArtifactProvides: typeInfoProvides,
	}
	augmentTypeInfoV3 := artifact.TypeInfoV3{
		Type:             ctx.String("type"),
		ArtifactDepends:  augmentTypeInfoDepends,
		ArtifactProvides: augmentTypeInfoProvides,
	}

	var metaData map[string]interface{}
	if len(ctx.String("meta-data")) > 0 {
		file, err := os.Open(ctx.String("meta-data"))
		if err != nil {
			return cli.NewExitError(err, errArtifactInvalidParameters)
		}
		defer file.Close()
		dec := json.NewDecoder(file)
		err = dec.Decode(&metaData)
		if err != nil {
			return cli.NewExitError(err, errArtifactInvalidParameters)
		}
	}

	var augmentMetaData map[string]interface{}
	if len(ctx.String("augment-meta-data")) > 0 {
		file, err := os.Open(ctx.String("augment-meta-data"))
		if err != nil {
			return cli.NewExitError(err, errArtifactInvalidParameters)
		}
		defer file.Close()
		dec := json.NewDecoder(file)
		err = dec.Decode(&augmentMetaData)
		if err != nil {
			return cli.NewExitError(err, errArtifactInvalidParameters)
		}
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
			TypeInfoV3:        &typeInfoV3,
			MetaData:          metaData,
			AugmentTypeInfoV3: &augmentTypeInfoV3,
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
		keyValues = new(map[string]string)
		*keyValues = make(map[string]string)
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
