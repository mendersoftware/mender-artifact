// Copyright 2023 Northern.tech AS
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
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"regexp"
	"strings"
	"syscall"
	"time"

	"io"
	"io/ioutil"

	"github.com/pkg/errors"
	"github.com/urfave/cli"

	"github.com/mendersoftware/mender-artifact/artifact"
	"github.com/mendersoftware/mender-artifact/artifact/stage"
	"github.com/mendersoftware/mender-artifact/awriter"
	"github.com/mendersoftware/mender-artifact/cli/util"
	"github.com/mendersoftware/mender-artifact/handlers"
	"github.com/mendersoftware/mender-artifact/utils"
)

func writeRootfsImageChecksum(rootfsFilename string,
	typeInfo *artifact.TypeInfoV3, legacy bool) (err error) {
	chk := artifact.NewWriterChecksum(ioutil.Discard)
	payload, err := os.Open(rootfsFilename)
	if err != nil {
		return cli.NewExitError(
			fmt.Sprintf("Failed to open the payload file: %q", rootfsFilename),
			1,
		)
	}
	if _, err = io.Copy(chk, payload); err != nil {
		return cli.NewExitError("Failed to generate the checksum for the payload", 1)
	}
	checksum := string(chk.Checksum())

	checksumKey := "rootfs-image.checksum"
	if legacy {
		checksumKey = "rootfs_image_checksum"
	}

	Log.Debugf("Adding the `%s`: %q to Artifact provides", checksumKey, checksum)
	if typeInfo == nil {
		return errors.New("Type-info is unitialized")
	}
	if typeInfo.ArtifactProvides == nil {
		t, err := artifact.NewTypeInfoProvides(map[string]string{checksumKey: checksum})
		if err != nil {
			return errors.Wrapf(err, "Failed to write the "+"`"+checksumKey+"` provides")
		}
		typeInfo.ArtifactProvides = t
	} else {
		typeInfo.ArtifactProvides[checksumKey] = checksum
	}
	return nil
}

func validateInput(c *cli.Context) error {
	// Version 2 and 3 validation.
	fileMissing := false
	if c.Command.Name != "bootstrap-artifact" {
		if len(c.String("file")) == 0 {
			fileMissing = true
		}
	}
	if len(c.StringSlice("device-type")) == 0 ||
		len(c.String("artifact-name")) == 0 || fileMissing {
		return cli.NewExitError(
			"must provide `device-type`, `artifact-name` and `file`",
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

func createRootfsFromSSH(c *cli.Context) (string, error) {
	rootfsFilename, err := getDeviceSnapshot(c)
	if err != nil {
		return rootfsFilename, cli.NewExitError("SSH error: "+err.Error(), 1)
	}

	// check for blkid and get filesystem type
	fstype, err := imgFilesystemType(rootfsFilename)
	if err != nil {
		if err == errBlkidNotFound {
			Log.Warnf("Skipping running fsck on the Artifact: %v", err)
			return rootfsFilename, nil
		}
		return rootfsFilename, cli.NewExitError(
			"imgFilesystemType error: "+err.Error(),
			errArtifactCreate,
		)
	}

	// run fsck
	switch fstype {
	case fat:
		err = runFsck(rootfsFilename, "vfat")
	case ext:
		err = runFsck(rootfsFilename, "ext4")
	case unsupported:
		err = errors.New("createRootfsFromSSH: unsupported filesystem")

	}
	if err != nil {
		return rootfsFilename, cli.NewExitError("runFsck error: "+err.Error(), errArtifactCreate)
	}

	return rootfsFilename, nil
}

func makeEmptyUpdates(ctx *cli.Context) (*awriter.Updates, error) {
	handler := handlers.NewBootstrapArtifact()

	dataFiles := make([](*handlers.DataFile), 0)
	if err := handler.SetUpdateFiles(dataFiles); err != nil {
		return nil, cli.NewExitError(
			err,
			1,
		)
	}

	upd := &awriter.Updates{
		Updates: []handlers.Composer{handler},
	}
	return upd, nil
}

func writeBootstrapArtifact(c *cli.Context) error {
	comp, err := artifact.NewCompressorFromId(c.GlobalString("compression"))
	if err != nil {
		return cli.NewExitError(
			"compressor '"+c.GlobalString("compression")+"' is not supported: "+err.Error(),
			1,
		)
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

	Log.Debugf("creating bootstrap artifact [%s], version: %d", name, version)

	f, err := os.Create(name)
	if err != nil {
		return cli.NewExitError(
			"can not create bootstrap artifact file: "+err.Error(),
			errArtifactCreate,
		)
	}
	defer f.Close()

	aw, err := artifactWriter(c, comp, f, version)
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

	upd, err := makeEmptyUpdates(c)
	if err != nil {
		return err
	}

	typeInfoV3, _, err := makeTypeInfo(c)
	if err != nil {
		return err
	}

	if !c.Bool("no-progress") {
		ctx, cancel := context.WithCancel(context.Background())
		go reportProgress(ctx, aw.State)
		defer cancel()
		aw.ProgressWriter = utils.NewProgressWriter()
	}

	err = aw.WriteArtifact(
		&awriter.WriteArtifactArgs{
			Format:     "mender",
			Version:    version,
			Devices:    c.StringSlice("device-type"),
			Name:       c.String("artifact-name"),
			Updates:    upd,
			Scripts:    nil,
			Depends:    &depends,
			Provides:   &provides,
			TypeInfoV3: typeInfoV3,
			Bootstrap:  true,
		})
	if err != nil {
		return cli.NewExitError(err.Error(), 1)
	}
	return nil
}

func writeRootfs(c *cli.Context) error {
	comp, err := artifact.NewCompressorFromId(c.GlobalString("compression"))
	if err != nil {
		return cli.NewExitError(
			"compressor '"+c.GlobalString("compression")+"' is not supported: "+err.Error(),
			1,
		)
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
	rootfsFilename := c.String("file")
	if strings.HasPrefix(rootfsFilename, "ssh://") {
		rootfsFilename, err = createRootfsFromSSH(c)
		defer os.Remove(rootfsFilename)
		if err != nil {
			return cli.NewExitError(err.Error(), errArtifactCreate)
		}
	}

	var h handlers.Composer
	switch version {
	case 2:
		h = handlers.NewRootfsV2(rootfsFilename)
	case 3:
		h = handlers.NewRootfsV3(rootfsFilename)
	default:
		return cli.NewExitError(
			fmt.Sprintf("Artifact version %d is not supported", version),
			errArtifactUnsupportedVersion,
		)
	}

	upd := &awriter.Updates{
		Updates: []handlers.Composer{h},
	}

	f, err := os.Create(name)
	if err != nil {
		return cli.NewExitError(
			"can not create artifact file: "+err.Error(),
			errArtifactCreate,
		)
	}
	defer f.Close()

	aw, err := artifactWriter(c, comp, f, version)
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

	if !c.Bool("no-checksum-provide") {
		legacy := c.Bool("legacy-rootfs-image-checksum")
		if err = writeRootfsImageChecksum(rootfsFilename, typeInfoV3, legacy); err != nil {
			return cli.NewExitError(
				errors.Wrap(err, "Failed to write the `rootfs-image.checksum` to the artifact"),
				1,
			)
		}
	}

	if !c.Bool("no-progress") {
		ctx, cancel := context.WithCancel(context.Background())
		go reportProgress(ctx, aw.State)
		defer cancel()
		aw.ProgressWriter = utils.NewProgressWriter()
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
	return nil
}

func reportProgress(c context.Context, state chan string) {
	fmt.Fprintln(os.Stderr, "Writing Artifact...")
	str := fmt.Sprintf("%-20s\t", <-state)
	fmt.Fprint(os.Stderr, str)
	for {
		select {
		case str = <-state:
			if str == stage.Data {
				fmt.Fprintf(os.Stderr, "\033[1;32m\u2713\033[0m\n")
				fmt.Fprintln(os.Stderr, "Payload")
			} else {
				fmt.Fprintf(os.Stderr, "\033[1;32m\u2713\033[0m\n")
				str = fmt.Sprintf("%-20s\t", str)
				fmt.Fprint(os.Stderr, str)
			}
		case <-c.Done():
			return
		}
	}
}

func artifactWriter(c *cli.Context, comp artifact.Compressor, w io.Writer,
	ver int) (*awriter.Writer, error) {
	privateKey, err := getKey(c)
	if err != nil {
		return nil, err
	}
	if privateKey != nil {
		if ver == 0 {
			// check if we are having correct version
			return nil, errors.New("can not use signed artifact with version 0")
		}
		return awriter.NewWriterSigned(w, comp, privateKey), nil
	}
	return awriter.NewWriter(w, comp), nil
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
	if err := handler.SetUpdateFiles(dataFiles); err != nil {
		return nil, cli.NewExitError(
			err,
			1,
		)
	}

	upd := &awriter.Updates{
		Updates: []handlers.Composer{handler},
	}

	if ctx.String("augment-type") != "" {
		augmentHandler = handlers.NewAugmentedModuleImage(handler, ctx.String("augment-type"))
		dataFiles = make([](*handlers.DataFile), 0, len(ctx.StringSlice("augment-file")))
		for _, file := range ctx.StringSlice("augment-file") {
			dataFiles = append(dataFiles, &handlers.DataFile{Name: file})
		}
		if err := augmentHandler.SetUpdateAugmentFiles(dataFiles); err != nil {
			return nil, cli.NewExitError(
				err,
				1,
			)
		}
		upd.Augments = []handlers.Composer{augmentHandler}
	}

	return upd, nil
}

// makeTypeInfo returns the type-info provides and depends and the augmented
// type-info provides and depends, or nil.
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
	typeInfoProvides = applySoftwareVersionToTypeInfoProvides(ctx, typeInfoProvides)

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

	clearsArtifactProvides, err := makeClearsArtifactProvides(ctx)
	if err != nil {
		return nil, nil, err
	}

	var typeInfo *string
	if ctx.Command.Name != "bootstrap-artifact" {
		typeFlag := ctx.String("type")
		typeInfo = &typeFlag
	}
	typeInfoV3 := &artifact.TypeInfoV3{
		Type:                   typeInfo,
		ArtifactDepends:        typeInfoDepends,
		ArtifactProvides:       typeInfoProvides,
		ClearsArtifactProvides: clearsArtifactProvides,
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

	augmentType := ctx.String("augment-type")
	augmentTypeInfoV3 := &artifact.TypeInfoV3{
		Type:             &augmentType,
		ArtifactDepends:  augmentTypeInfoDepends,
		ArtifactProvides: augmentTypeInfoProvides,
	}

	return typeInfoV3, augmentTypeInfoV3, nil
}

func getSoftwareVersion(
	artifactName,
	softwareFilesystem,
	softwareName,
	softwareNameDefault,
	softwareVersion string,
	noDefaultSoftwareVersion bool,
) map[string]string {
	result := map[string]string{}
	softwareVersionName := "rootfs-image"
	if softwareFilesystem != "" {
		softwareVersionName = softwareFilesystem
	}
	if !noDefaultSoftwareVersion {
		if softwareName == "" {
			softwareName = softwareNameDefault
		}
		if softwareVersion == "" {
			softwareVersion = artifactName
		}
	}
	if softwareName != "" {
		softwareVersionName += fmt.Sprintf(".%s", softwareName)
	}
	if softwareVersionName != "" && softwareVersion != "" {
		result[softwareVersionName+".version"] = softwareVersion
	}
	return result
}

// applySoftwareVersionToTypeInfoProvides returns a new mapping, enriched with provides
// for the software version; the mapping provided as argument is not modified
func applySoftwareVersionToTypeInfoProvides(
	ctx *cli.Context,
	typeInfoProvides artifact.TypeInfoProvides,
) artifact.TypeInfoProvides {
	result := make(map[string]string)
	for key, value := range typeInfoProvides {
		result[key] = value
	}
	artifactName := ctx.String("artifact-name")
	softwareFilesystem := ctx.String(softwareFilesystemFlag)
	softwareName := ctx.String(softwareNameFlag)
	softwareNameDefault := ""
	if ctx.Command.Name == "module-image" {
		softwareNameDefault = ctx.String("type")
	}
	if ctx.Command.Name == "bootstrap-artifact" {
		return result
	}
	softwareVersion := ctx.String(softwareVersionFlag)
	noDefaultSoftwareVersion := ctx.Bool(noDefaultSoftwareVersionFlag)
	if softwareVersionMapping := getSoftwareVersion(
		artifactName,
		softwareFilesystem,
		softwareName,
		softwareNameDefault,
		softwareVersion,
		noDefaultSoftwareVersion,
	); len(softwareVersionMapping) > 0 {
		for key, value := range softwareVersionMapping {
			if result[key] == "" || softwareVersionOverridesProvides(ctx, key) {
				result[key] = value
			}
		}
	}
	return result
}

func softwareVersionOverridesProvides(ctx *cli.Context, key string) bool {
	mainCtx := ctx.Parent().Parent()
	cmdLine := strings.Join(mainCtx.Args(), " ")

	var providesVersion string = `(-p|--provides)(\s+|=)` + regexp.QuoteMeta(key) + ":"
	reProvidesVersion := regexp.MustCompile(providesVersion)
	providesIndexes := reProvidesVersion.FindAllStringIndex(cmdLine, -1)

	var softareVersion string = "--software-(name|version|filesystem)"
	reSoftwareVersion := regexp.MustCompile(softareVersion)
	softwareIndexes := reSoftwareVersion.FindAllStringIndex(cmdLine, -1)

	if len(providesIndexes) == 0 {
		return true
	} else if len(softwareIndexes) == 0 {
		return false
	} else {
		return softwareIndexes[len(softwareIndexes)-1][0] >
			providesIndexes[len(providesIndexes)-1][0]
	}
}

func makeClearsArtifactProvides(ctx *cli.Context) ([]string, error) {
	list := ctx.StringSlice(clearsProvidesFlag)

	if ctx.Bool(noDefaultClearsProvidesFlag) ||
		ctx.Bool(noDefaultSoftwareVersionFlag) ||
		ctx.Command.Name == "bootstrap-artifact" {
		return list, nil
	}

	var softwareFilesystem string
	if ctx.IsSet("software-filesystem") {
		softwareFilesystem = ctx.String("software-filesystem")
	} else {
		softwareFilesystem = "rootfs-image"
	}

	var softwareName string
	if len(ctx.String("software-name")) > 0 {
		softwareName = ctx.String("software-name") + "."
	} else if ctx.Command.Name == "rootfs-image" {
		softwareName = ""
		// "rootfs_image_checksum" is included for legacy
		// reasons. Previously, "rootfs_image_checksum" was the name
		// given to the checksum, but new artifacts follow the new dot
		// separated scheme, "rootfs-image.checksum", which also has the
		// correct dash instead of the incorrect underscore.
		//
		// "artifact_group" is included as a sane default for
		// rootfs-image updates. A standard rootfs-image update should
		// clear the group if it does not have one.
		if softwareFilesystem == "rootfs-image" {
			list = append(list, "artifact_group", "rootfs_image_checksum")
		}
	} else if ctx.Command.Name == "module-image" {
		softwareName = ctx.String("type") + "."
	} else {
		return nil, errors.New(
			"Unknown write command in makeClearsArtifactProvides(), this is a bug.",
		)
	}

	defaultCap := fmt.Sprintf("%s.%s*", softwareFilesystem, softwareName)
	for _, cap := range list {
		if defaultCap == cap {
			// Avoid adding it twice if the default is the same as a
			// specified provide.
			goto dontAdd
		}
	}
	list = append(list, defaultCap)

dontAdd:
	return list, nil
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
		return cli.NewExitError(
			"compressor '"+ctx.GlobalString("compression")+"' is not supported: "+err.Error(),
			1,
		)
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

	// The device-type flag is required
	if len(ctx.StringSlice("device-type")) == 0 {
		return cli.NewExitError("The `device-type` flag is required", 1)
	}

	upd, err := makeUpdates(ctx)
	if err != nil {
		return err
	}

	f, err := os.Create(name)
	if err != nil {
		return cli.NewExitError(
			"can not create artifact file: "+err.Error(),
			errArtifactCreate,
		)
	}
	defer f.Close()

	aw, err := artifactWriter(ctx, comp, f, version)
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
			(*keyValues)[split[0]] = split[1]
		}
	}
	return keyValues, nil
}

// SSH to remote host and dump rootfs snapshot to a local temporary file.
func getDeviceSnapshot(c *cli.Context) (string, error) {

	const sshInitMagic = "Initializing snapshot..."
	var userAtHost string
	var sigChan chan os.Signal
	var errChan chan error
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	port := "22"
	host := strings.TrimPrefix(c.String("file"), "ssh://")

	if remotePort := strings.Split(host, ":"); len(remotePort) == 2 {
		port = remotePort[1]
		userAtHost = remotePort[0]
	} else {
		userAtHost = host
	}

	// Prepare command-line arguments
	args := c.StringSlice("ssh-args")
	// Check if port is specified explicitly with the --ssh-args flag
	addPort := true
	for _, arg := range args {
		if strings.Contains(arg, "-p") {
			addPort = false
			break
		}
	}
	if addPort {
		args = append(args, "-p", port)
	}
	args = append(args, userAtHost)
	// First echo to stdout such that we know when ssh connection is
	// established (password prompt is written to /dev/tty directly,
	// and hence impossible to detect).
	// When user id is 0 do not bother with sudo.
	args = append(
		args,
		"/bin/sh",
		"-c",
		`'mender_snapshot="mender snapshot"`+
			`; which mender-snapshot 1> /dev/null && mender_snapshot="mender-snapshot"`+
			`; [ $(id -u) -eq 0 ] || sudo_cmd="sudo -S"`+
			`; $sudo_cmd /bin/sh -c "echo `+sshInitMagic+`; $mender_snapshot dump" | cat'`,
	)

	cmd := exec.Command("ssh", args...)

	// Simply connect stdin/stderr
	cmd.Stdin = os.Stdin
	cmd.Stderr = os.Stderr
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return "", errors.New("Error redirecting stdout on exec")
	}

	// Create tempfile for storing the snapshot
	f, err := ioutil.TempFile("", "rootfs.tmp")
	if err != nil {
		return "", err
	}
	filePath := f.Name()

	defer removeOnPanic(filePath)
	defer f.Close()

	// Disable tty echo before starting
	term, err := util.DisableEcho(int(os.Stdin.Fd()))
	if err == nil {
		sigChan = make(chan os.Signal, 1)
		errChan = make(chan error, 1)
		// Make sure that echo is enabled if the process gets
		// interrupted
		signal.Notify(sigChan)
		go util.EchoSigHandler(ctx, sigChan, errChan, term)
	} else if err != syscall.ENOTTY {
		return "", err
	}

	if err := cmd.Start(); err != nil {
		return "", err
	}

	// Wait for 60 seconds for ssh to establish connection
	err = waitForBufferSignal(stdout, os.Stdout, sshInitMagic, 2*time.Minute)
	if err != nil {
		_ = cmd.Process.Kill()
		return "", errors.Wrap(err,
			"Error waiting for ssh session to be established.")
	}

	_, err = recvSnapshot(f, stdout)
	if err != nil {
		_ = cmd.Process.Kill()
		return "", err
	}

	if cmd.ProcessState != nil && cmd.ProcessState.Exited() {
		return "", errors.New("SSH session closed unexpectedly")
	}

	if err = cmd.Wait(); err != nil {
		return "", errors.Wrap(err,
			"SSH session closed with error")
	}

	if sigChan != nil {
		// Wait for signal handler to execute
		signal.Stop(sigChan)
		cancel()
		err = <-errChan
	}

	return filePath, err
}

// Reads from src waiting for the string specified by signal, writing all other
// output appearing at src to sink. The function returns an error if occurs
// reading from the stream or the deadline exceeds.
func waitForBufferSignal(src io.Reader, sink io.Writer,
	signal string, deadline time.Duration) error {

	var err error
	errChan := make(chan error)

	go func() {
		stdoutRdr := bufio.NewReader(src)
		for {
			line, err := stdoutRdr.ReadString('\n')
			if err != nil {
				errChan <- err
				break
			}
			if strings.Contains(line, signal) {
				errChan <- nil
				break
			}
			_, err = sink.Write([]byte(line + "\n"))
			if err != nil {
				errChan <- err
				break
			}
		}
	}()

	select {
	case err = <-errChan:
		// Error from goroutine
	case <-time.After(deadline):
		err = errors.New("Input deadline exceeded")
	}
	return err
}

// Performs the same operation as io.Copy while at the same time prining
// the number of bytes written at any time.
func recvSnapshot(dst io.Writer, src io.Reader) (int64, error) {
	buf := make([]byte, 1024*1024*32)
	var written int64
	for {
		nr, err := src.Read(buf)
		if err == io.EOF {
			break
		} else if err != nil {
			return written, errors.Wrap(err,
				"Error receiving snapshot from device")
		}
		nw, err := dst.Write(buf[:nr])
		if err != nil {
			return written, errors.Wrap(err,
				"Error storing snapshot locally")
		} else if nw < nr {
			return written, io.ErrShortWrite
		}
		written += int64(nw)
	}
	return written, nil
}

func removeOnPanic(filename string) {
	if r := recover(); r != nil {
		err := os.Remove(filename)
		if err != nil {
			switch v := r.(type) {
			case string:
				err = errors.Wrap(errors.New(v), err.Error())
				panic(err)
			case error:
				err = errors.Wrap(v, err.Error())
				panic(err)
			default:
				panic(r)
			}
		}
		panic(r)
	}
}
