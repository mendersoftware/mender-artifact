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
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"

	"github.com/mendersoftware/mender-artifact/areader"
	"github.com/mendersoftware/mender-artifact/artifact"
	"github.com/mendersoftware/mender-artifact/awriter"
	"github.com/mendersoftware/mender-artifact/handlers"
	"github.com/mendersoftware/mender-artifact/utils"

	"github.com/pkg/errors"
)

type writeUpdateStorer struct {
	name   string
	writer io.Writer
}

func (w *writeUpdateStorer) Initialize(artifactHeaders,
	artifactAugmentedHeaders artifact.HeaderInfoer,
	payloadHeaders handlers.ArtifactUpdateHeaders) error {

	return nil
}

func (w *writeUpdateStorer) PrepareStoreUpdate() error {
	return nil
}

func (w *writeUpdateStorer) StoreUpdate(r io.Reader, info os.FileInfo) error {
	w.name = info.Name()
	_, err := io.Copy(w.writer, r)
	return err
}

func (w *writeUpdateStorer) FinishStoreUpdate() error {
	return nil
}

func (w *writeUpdateStorer) NewUpdateStorer(updateType string, payloadNum int) (handlers.UpdateStorer, error) {
	// For rootfs, which is the only type we support for artifact
	// modifications, there should only ever be one payload, with one file,
	// so our producer just returns itself.
	if payloadNum != 0 {
		return nil, errors.New("More than one payload or update file is not supported")
	}
	if updateType != "rootfs-image" {
		return nil, errors.New("Only rootfs update types supported")
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

func read(ar *areader.Reader, verify areader.SignatureVerifyFn,
	readScripts areader.ScriptsReadFn) (*areader.Reader, error) {

	if ar == nil {
		return nil, errors.New("can not read artifact file")
	}

	if verify != nil {
		ar.VerifySignatureCallback = verify
	}
	if readScripts != nil {
		ar.ScriptsReadCallback = readScripts
	}

	if err := ar.ReadArtifact(); err != nil {
		return nil, err
	}

	return ar, nil
}

func getKey(keyPath string) ([]byte, error) {
	if keyPath == "" {
		return nil, nil
	}

	key, err := ioutil.ReadFile(keyPath)
	if err != nil {
		return nil, errors.Wrap(err, "Error reading key file")
	}
	return key, nil
}

func unpackArtifact(name string) (string, error) {
	f, err := os.Open(name)
	if err != nil {
		return "", errors.Wrapf(err, "Can not open: %s", name)
	}
	defer f.Close()

	// initialize raw reader and writer
	aReader := areader.NewReader(f)
	rootfs := handlers.NewRootfsInstaller()

	tmp, err := ioutil.TempFile("", "mender-artifact")
	if err != nil {
		return "", err
	}
	defer tmp.Close()

	updateStore := &writeUpdateStorer{
		writer: tmp,
	}
	rootfs.SetUpdateStorerProducer(updateStore)

	if err = aReader.RegisterHandler(rootfs); err != nil {
		return "", errors.Wrap(err, "failed to register install handler")
	}

	err = aReader.ReadArtifact()
	if err != nil {
		return "", err
	}
	// Give the tempfile it's original name, so that the update does not change name upon a write.
	tmpfilePath := tmp.Name()
	newNamePath := filepath.Join(filepath.Dir(tmpfilePath), updateStore.name)
	if err = os.Rename(tmpfilePath, newNamePath); err != nil {
		return "", err
	}
	return newNamePath, nil
}

func repack(comp artifact.Compressor, artifactName string, from io.Reader, to io.Writer, key []byte,
	newName string, dataFile string) (*areader.Reader, error) {
	sDir, err := ioutil.TempDir(filepath.Dir(artifactName), "mender-repack")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(sDir)

	storeScripts := func(r io.Reader, info os.FileInfo) error {
		sLocation := filepath.Join(sDir, info.Name())
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
		f.Sync()
		return nil
	}

	verify := func(message, sig []byte) error {
		return nil
	}

	data := dataFile
	ar := areader.NewReader(from)

	if dataFile == "" {
		tmpData, tmpErr := ioutil.TempFile("", "mender-repack")
		if tmpErr != nil {
			return nil, tmpErr
		}
		defer os.Remove(tmpData.Name())
		defer tmpData.Close()

		rootfs := handlers.NewRootfsInstaller()
		updateStore := &writeUpdateStorer{
			writer: tmpData,
		}
		rootfs.SetUpdateStorerProducer(updateStore)

		data = tmpData.Name()
		ar.RegisterHandler(rootfs)
	}

	r, err := read(ar, verify, storeScripts)
	if err != nil {
		return nil, err
	}

	info := r.GetInfo()

	// now once arifact is read we need to
	var h *handlers.Rootfs
	switch info.Version {
	case 1:
		h = handlers.NewRootfsV1(data)
	case 2:
		h = handlers.NewRootfsV2(data)
	case 3:
		h = handlers.NewRootfsV3(data)
	default:
		return nil, errors.Errorf("unsupported artifact version: %d", info.Version)
	}

	upd := &awriter.Updates{
		Updates: []handlers.Composer{h},
	}
	scr, err := scripts([]string{sDir})
	if err != nil {
		return nil, err
	}

	aWriter := awriter.NewWriter(to, comp)
	if key != nil {
		aWriter = awriter.NewWriterSigned(to, comp, artifact.NewSigner(key))
	}

	name := ar.GetArtifactName()
	provides := ar.GetArtifactProvides()
	if newName != "" {
		name = newName
		if provides != nil {
			provides.ArtifactName = newName
		}
	}

	typeInfoV3 := artifact.TypeInfoV3{
		Type: "rootfs-image",
		// Keeping these empty for now. We will likely introduce these
		// later, when we add support for augmented artifacts.
		// ArtifactDepends:  &artifact.TypeInfoDepends{"rootfs_image_checksum": c.String("depends-rootfs-image-checksum")},
		// ArtifactProvides: &artifact.TypeInfoProvides{"rootfs_image_checksum": c.String("provides-rootfs-image-checksum")},
		ArtifactDepends:  &artifact.TypeInfoDepends{},
		ArtifactProvides: &artifact.TypeInfoProvides{},
	}

	err = aWriter.WriteArtifact(
		&awriter.WriteArtifactArgs{
			Format:   info.Format,
			Version:  info.Version,
			Devices:  ar.GetCompatibleDevices(),
			Name:     name,
			Updates:  upd,
			Scripts:  scr,
			Provides: provides,
			Depends:  ar.GetArtifactDepends(),
			TypeInfoV3: &typeInfoV3,
		})
	return ar, err
}

func repackArtifact(comp artifact.Compressor, artifact, rootfs, newName string) error {
	art, err := os.Open(artifact)
	if err != nil {
		return err
	}
	defer art.Close()

	tmp, err := ioutil.TempFile(filepath.Dir(artifact), "mender-artifact")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	defer tmp.Close()

	if _, err = repack(comp, artifact, art, tmp, nil, newName, rootfs); err != nil {
		return err
	}

	return os.Rename(tmp.Name(), artifact)
}

func processSdimg(image string) ([]partition, error) {
	bin, err := utils.GetBinaryPath("parted")
	if err != nil {
		return nil, fmt.Errorf("`parted` binary not found on the system")
	}
	out, err := exec.Command(bin, image, "unit s", "print").Output()
	if err != nil {
		return nil, errors.Wrap(err, "can not execute `parted` command or image is broken; "+
			"make sure parted is available in your system and is in the $PATH")
	}

	partitions := make([]partition, 0)

	reg := regexp.MustCompile(`(?m)^[[:blank:]][0-9]+[[:blank:]]+([0-9]+)s[[:blank:]]+[0-9]+s[[:blank:]]+([0-9]+)s`)
	partitionMatch := reg.FindAllStringSubmatch(string(out), -1)

	if len(partitionMatch) == 4 {
		// we will have three groups per each entry in the partition table
		for i := 0; i < 4; i++ {
			single := partitionMatch[i]
			partitions = append(partitions, partition{offset: single[1], size: single[2]})
		}
		if err = extractFromSdimg(partitions, image); err != nil {
			return nil, err
		}
		return partitions, nil
		// if we have single ext file there is no need to mount it
	} else if len(partitionMatch) == 1 {
		return []partition{{path: image}}, nil
	}
	return nil, fmt.Errorf("invalid partition table: %s", string(out))
}

func extractFromSdimg(partitions []partition, image string) error {
	for i, part := range partitions {
		tmp, err := ioutil.TempFile("", "mender-modify-image")
		if err != nil {
			return errors.Wrap(err, "can not create temp file for storing image")
		}
		if err = tmp.Close(); err != nil {
			return errors.Wrapf(err, "can not close temporary file: %s", tmp.Name())
		}
		cmd := exec.Command("dd", "if="+image, "of="+tmp.Name(),
			"skip="+part.offset, "count="+part.size)
		if err = cmd.Run(); err != nil {
			return errors.Wrap(err, "can not extract image from sdimg")
		}
		partitions[i].path = tmp.Name()
	}
	return nil
}

func repackSdimg(partitions []partition, image string) error {
	for _, part := range partitions {
		if err := exec.Command("dd", "if="+part.path, "of="+image,
			"seek="+part.offset, "count="+part.size,
			"conv=notrunc").Run(); err != nil {
			return errors.Wrap(err, "can not copy image back to sdimg")
		}
	}
	return nil
}

func getCandidatesForModify(path string) ([]partition, bool, error) {
	isArtifact := false
	modifyCandidates := make([]partition, 0)

	// first we need to check  if we are having artifact or image file
	art, err := os.Open(path)
	if err != nil {
		return nil, isArtifact, errors.Wrap(err, "can not open artifact")
	}
	defer art.Close()

	if err = validate(art, nil); err == nil {
		// we have VALID artifact, so we need to unpack it and store header
		isArtifact = true
		rawImage, err := unpackArtifact(path)
		if err != nil {
			return nil, isArtifact, errors.Wrap(err, "can not process artifact")
		}
		modifyCandidates = append(modifyCandidates, partition{path: rawImage})
	} else {
		parts, err := processSdimg(path)
		if err != nil {
			return nil, isArtifact, errors.Wrap(err, "can not process image file")
		}
		modifyCandidates = append(modifyCandidates, parts...)
	}
	return modifyCandidates, isArtifact, nil
}
