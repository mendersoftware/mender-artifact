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
	"context"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"

	"github.com/EcoG-io/mender-artifact/areader"
	"github.com/EcoG-io/mender-artifact/artifact"
	"github.com/EcoG-io/mender-artifact/artifact/gcp"
	"github.com/EcoG-io/mender-artifact/artifact/vault"
	"github.com/EcoG-io/mender-artifact/awriter"
	"github.com/EcoG-io/mender-artifact/handlers"

	"github.com/pkg/errors"
	"github.com/urfave/cli"
)

type unpackedArtifact struct {
	origPath  string
	unpackDir string
	ar        *areader.Reader
	scripts   []string
	files     []string

	// Args needed to reconstruct the artifact
	writeArgs *awriter.WriteArtifactArgs
}

type writeUpdateStorer struct {
	// Dir to store files in
	dir string
	// Files that are stored. Will be filled in while storing
	names []string
}

func (w *writeUpdateStorer) Initialize(artifactHeaders,
	artifactAugmentedHeaders artifact.HeaderInfoer,
	payloadHeaders handlers.ArtifactUpdateHeaders) error {

	if artifactAugmentedHeaders != nil {
		return errors.New("Modifying augmented artifacts is not supported")
	}

	return nil
}

func (w *writeUpdateStorer) PrepareStoreUpdate() error {
	return nil
}

func (w *writeUpdateStorer) StoreUpdate(r io.Reader, info os.FileInfo) error {
	fullpath := filepath.Join(w.dir, info.Name())
	w.names = append(w.names, fullpath)
	fd, err := os.OpenFile(fullpath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0644)
	if err != nil {
		return err
	}
	_, err = io.Copy(fd, r)
	return err
}

func (w *writeUpdateStorer) FinishStoreUpdate() error {
	return nil
}

func (w *writeUpdateStorer) NewUpdateStorer(
	updateType *string,
	payloadNum int,
) (handlers.UpdateStorer, error) {
	if payloadNum != 0 {
		return nil, errors.New("More than one payload or update file is not supported")
	}
	return w, nil
}

func scripts(scripts []string) (*artifact.Scripts, error) {
	scr := artifact.Scripts{}
	for _, scriptArg := range scripts {
		statInfo, err := os.Stat(scriptArg)
		if err != nil {
			return nil, errors.Wrapf(err, "can not stat script file: %s", scriptArg)
		}

		// Read either a directory, or add the script file directly.
		if statInfo.IsDir() {
			fileList, err := ioutil.ReadDir(scriptArg)
			if err != nil {
				return nil, errors.Wrapf(err, "can not list directory contents of: %s", scriptArg)
			}
			for _, nameInfo := range fileList {
				if err := scr.Add(filepath.Join(scriptArg, nameInfo.Name())); err != nil {
					return nil, err
				}
			}
		} else {
			if err := scr.Add(scriptArg); err != nil {
				return nil, err
			}
		}
	}
	return &scr, nil
}

type SigningKey interface {
	artifact.Signer
	artifact.Verifier
}

func getKey(c *cli.Context) (SigningKey, error) {
	var chosenOptions []string
	possibleOptions := []string{"key", "gcp-kms-key", "vault-transit-key", "key-pkcs11"}
	for _, optName := range possibleOptions {
		if c.String(optName) == "" {
			continue
		}
		chosenOptions = append(chosenOptions, optName)
	}
	if len(chosenOptions) == 0 {
		return nil, nil
	} else if len(chosenOptions) > 1 {
		return nil, fmt.Errorf("too many signing keys given: %v", chosenOptions)
	}
	switch chosenOption := chosenOptions[0]; chosenOption {
	case "key":
		key, err := ioutil.ReadFile(c.String("key"))
		if err != nil {
			return nil, errors.Wrap(err, "Error reading key file")
		}

		// The "key" flag can either be public or private depending on the
		// command name. Explicitly map each command's name to which one it
		// should be, so we return the correct key type.
		publicKeyCommands := map[string]bool{
			"validate": true,
			"read":     true,
		}
		privateKeyCommands := map[string]bool{
			"rootfs-image":       true,
			"module-image":       true,
			"bootstrap-artifact": true,
			"sign":               true,
			"modify":             true,
			"copy":               true,
		}
		if publicKeyCommands[c.Command.Name] {
			return artifact.NewPKIVerifier(key)
		}
		if privateKeyCommands[c.Command.Name] {
			return artifact.NewPKISigner(key)
		}
		return nil, fmt.Errorf("unsupported command %q with %q flag, "+
			"please add command to allowlist", c.Command.Name, "key")
	case "gcp-kms-key":
		return gcp.NewKMSSigner(context.TODO(), c.String("gcp-kms-key"))
	case "vault-transit-key":
		return vault.NewVaultSigner(c.String("vault-transit-key"))
	case "key-pkcs11":
		return artifact.NewPKCS11Signer(c.String("key-pkcs11"))
	default:
		return nil, fmt.Errorf("unsupported signing key type %q", chosenOption)
	}
}

func unpackArtifact(name string) (ua *unpackedArtifact, err error) {
	ua = &unpackedArtifact{
		origPath: name,
	}

	f, err := os.Open(name)
	if err != nil {
		return nil, errors.Wrapf(err, "Can not open: %s", name)
	}
	defer f.Close()

	aReader := areader.NewReader(f)
	ua.ar = aReader

	tmpdir, err := ioutil.TempDir("", "mender-artifact")
	if err != nil {
		return nil, err
	}
	ua.unpackDir = tmpdir
	defer func() {
		if err != nil {
			os.RemoveAll(tmpdir)
		}
	}()

	sDir := filepath.Join(tmpdir, "scripts")
	err = os.Mkdir(sDir, 0755)
	if err != nil {
		return nil, err
	}
	storeScripts := func(r io.Reader, info os.FileInfo) error {
		sLocation := filepath.Join(sDir, info.Name())
		ua.scripts = append(ua.scripts, sLocation)
		f, fileErr := os.OpenFile(sLocation, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0755)
		if fileErr != nil {
			return errors.Wrapf(fileErr,
				"can not create script file: %v", sLocation)
		}
		defer f.Close()

		_, err = io.Copy(f, r)
		if err != nil {
			return errors.Wrapf(err,
				"can not write script file: %v", sLocation)
		}
		return nil
	}
	aReader.ScriptsReadCallback = storeScripts

	err = aReader.ReadArtifactHeaders()
	if err != nil {
		return nil, err
	}

	fDir := filepath.Join(tmpdir, "files")
	err = os.Mkdir(fDir, 0755)
	if err != nil {
		return nil, err
	}

	updateStore := &writeUpdateStorer{
		dir: fDir,
	}
	inst := aReader.GetHandlers()
	if len(inst) == 1 {
		inst[0].SetUpdateStorerProducer(updateStore)
	} else if len(inst) > 1 {
		return nil, errors.New("More than one payload not supported")
	}

	if err := aReader.ReadArtifactData(); err != nil {
		return nil, err
	}

	ua.files = updateStore.names

	updType := inst[0].GetUpdateType()
	if updType == nil {
		return nil, errors.New("nil update type is not allowed")
	}
	if len(inst) > 0 &&
		*inst[0].GetUpdateType() == "rootfs-image" &&
		len(ua.files) != 1 {

		return nil, errors.New("rootfs-image artifacts with more than one file not supported")
	}

	ua.writeArgs, err = reconstructArtifactWriteData(ua)
	return ua, err
}

func reconstructPayloadWriteData(
	info *artifact.Info,
	inst map[int]handlers.Installer,
) (upd *awriter.Updates,
	typeInfoV3 *artifact.TypeInfoV3,
	augTypeInfoV3 *artifact.TypeInfoV3,
	metaData map[string]interface{},
	augMetaData map[string]interface{},
	err error) {

	if len(inst) > 1 {
		err = errors.New("More than one payload not supported")
		return
	} else if len(inst) == 1 {
		var updateType *string
		upd = &awriter.Updates{}

		switch info.Version {
		case 1:
			err = errors.New("Mender-Artifact version 1 no longer supported")
			return
		case 2:
			updateType = inst[0].GetUpdateType()
			if updateType == nil {
				err = errors.New("nil update type is not allowed")
				return
			}
			upd.Updates = []handlers.Composer{handlers.NewRootfsV2(*updateType)}
		case 3:
			// Even rootfs images will be written using ModuleImage, which
			// is a superset
			var updType *string
			updType = inst[0].GetUpdateOriginalType()
			if *updType != "" {
				// If augmented artifact.
				upd.Augments = []handlers.Composer{handlers.NewModuleImage(*updType)}
				augTypeInfoV3 = &artifact.TypeInfoV3{
					Type:             updType,
					ArtifactDepends:  inst[0].GetUpdateOriginalDepends(),
					ArtifactProvides: inst[0].GetUpdateOriginalProvides(),
				}
				augMetaData = inst[0].GetUpdateOriginalMetaData()
			}

			updateType = inst[0].GetUpdateType()
			if updateType == nil {
				err = errors.New("nil update type is not allowed")
				return
			}
			upd.Updates = []handlers.Composer{handlers.NewModuleImage(*updateType)}

		default:
			err = errors.Errorf("unsupported artifact version: %d", info.Version)
			return
		}

		var uDepends artifact.TypeInfoDepends
		var uProvides artifact.TypeInfoProvides

		if uDepends, err = inst[0].GetUpdateDepends(); err != nil {
			return
		}
		if uProvides, err = inst[0].GetUpdateProvides(); err != nil {
			return
		}
		typeInfoV3 = &artifact.TypeInfoV3{
			Type:                   updateType,
			ArtifactDepends:        uDepends,
			ArtifactProvides:       uProvides,
			ClearsArtifactProvides: inst[0].GetUpdateOriginalClearsProvides(),
		}

		if metaData, err = inst[0].GetUpdateMetaData(); err != nil {
			return
		}
	}

	return
}

func reconstructArtifactWriteData(ua *unpackedArtifact) (*awriter.WriteArtifactArgs, error) {
	info := ua.ar.GetInfo()
	inst := ua.ar.GetHandlers()

	upd, typeInfoV3, augTypeInfoV3, metaData, augMetaData, err := reconstructPayloadWriteData(
		&info,
		inst,
	)
	if err != nil {
		return nil, err
	}

	if len(inst) == 1 {
		dataFiles := make([]*handlers.DataFile, 0, len(ua.files))
		for _, file := range ua.files {
			dataFiles = append(dataFiles, &handlers.DataFile{Name: file})
		}
		err := upd.Updates[0].SetUpdateFiles(dataFiles)
		if err != nil {
			return nil, errors.Wrap(err, "Cannot assign payload files")
		}
	} else if len(inst) > 1 {
		return nil, errors.New("Multiple payloads not supported")
	}

	scr, err := scripts(ua.scripts)
	if err != nil {
		return nil, err
	}

	name := ua.ar.GetArtifactName()

	args := &awriter.WriteArtifactArgs{
		Format:            info.Format,
		Version:           info.Version,
		Devices:           ua.ar.GetCompatibleDevices(),
		Name:              name,
		Updates:           upd,
		Scripts:           scr,
		Provides:          ua.ar.GetArtifactProvides(),
		Depends:           ua.ar.GetArtifactDepends(),
		TypeInfoV3:        typeInfoV3,
		MetaData:          metaData,
		AugmentTypeInfoV3: augTypeInfoV3,
		AugmentMetaData:   augMetaData,
	}

	return args, nil
}

func repack(comp artifact.Compressor, ua *unpackedArtifact, to io.Writer, key SigningKey) error {
	aWriter := awriter.NewWriter(to, comp)
	if key != nil {
		aWriter = awriter.NewWriterSigned(to, comp, key)
	}

	// for rootfs-images: Update rootfs-image.checksum provide if there is one.
	_, hasChecksumProvide := ua.writeArgs.TypeInfoV3.ArtifactProvides["rootfs-image.checksum"]
	// for rootfs-images: Update legacy rootfs_image_checksum provide if there is one.
	_, hasLegacyChecksumProvide := ua.writeArgs.TypeInfoV3.ArtifactProvides["rootfs_image_checksum"]
	if *ua.writeArgs.TypeInfoV3.Type == "rootfs-image" && (hasChecksumProvide ||
		hasLegacyChecksumProvide) {
		if len(ua.files) != 1 {
			return errors.New("Only rootfs-image Artifacts with one file are supported")
		}
		err := writeRootfsImageChecksum(
			ua.files[0],
			ua.writeArgs.TypeInfoV3,
			hasLegacyChecksumProvide,
		)
		if err != nil {
			return err
		}
	}

	return aWriter.WriteArtifact(ua.writeArgs)
}

func repackArtifact(comp artifact.Compressor, key SigningKey, ua *unpackedArtifact) error {
	tmp, err := ioutil.TempFile(filepath.Dir(ua.origPath), "mender-artifact")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	defer tmp.Close()

	if err = repack(comp, ua, tmp, key); err != nil {
		return err
	}

	tmp.Close()

	return os.Rename(tmp.Name(), ua.origPath)
}
