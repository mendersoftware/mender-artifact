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
	"crypto/sha256"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/mendersoftware/mender-artifact/artifact"
	"github.com/mendersoftware/mender-artifact/awriter"
	"github.com/mendersoftware/mender-artifact/handlers"
	"github.com/pkg/errors"
	"github.com/urfave/cli"
)

func validateInput(c *cli.Context) error {
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

func getRootfsChecksum(rootfs string) (string, error) {
	partf, err := os.OpenFile(rootfs, os.O_RDONLY, os.ModeDevice)
	if err != nil {
		return "", errors.Wrapf(err, "getRootfsChecksum: failed to open rootfs: %s", rootfs)
	}
	h := sha256.New()
	if _, err = io.Copy(h, partf); err != nil {
		return "", errors.Wrap(err, "getRootfsChecksum: failed to calculate the checksum of the inactive partition")
	}
	checksumHex := fmt.Sprintf("%x", h.Sum(nil))
	return checksumHex, nil
}

func validateDeltaInput(c *cli.Context) error {
	if len(c.StringSlice("device-type")) == 0 ||
		c.String("artifact-name") == "" ||
		c.String("newrootfs") == "" ||
		c.String("oldrootfs") == "" {
		return cli.NewExitError("must provide `device-type`, `artifact-name`, `old/newrootfs`", 1)
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

func writeDelta(c *cli.Context) error {
	if err := validateDeltaInput(c); err != nil {
		Log.Error(err.Error())
		return err
	}

	// set the default name
	name := "artifact.mender"
	if len(c.String("output-path")) > 0 {
		name = c.String("output-path")
	}
	version := c.Int("version")

	Log.Debugf("creating arifact [%s], version: %d", name, version)

	newRootfs := c.String("newrootfs")
	oldRootfs := c.String("oldrootfs")
	// Get the checksums for our rootfs'
	newRootfsChecksum, err := getRootfsChecksum(newRootfs)
	oldRootfsChecksum, err := getRootfsChecksum(oldRootfs)
	deltaFile := newRootfs

	h := handlers.NewDelta(deltaFile, newRootfsChecksum, oldRootfsChecksum)

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

	err = aw.WriteArtifact(
		&awriter.WriteArtifactArgs{
			Format:  "mender",
			Version: version,
			Devices: c.StringSlice("device-type"),
			Name:    c.String("artifact-name"),
			Updates: upd,
			Scripts: scr,
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

	Log.Debugf("creating arifact [%s], version: %d", name, version)

	var h *handlers.Rootfs
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

	err = aw.WriteArtifact(
		&awriter.WriteArtifactArgs{
			Format:  "mender",
			Version: version,
			Devices: c.StringSlice("device-type"),
			Name:    c.String("artifact-name"),
			Updates: upd,
			Scripts: scr,
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
