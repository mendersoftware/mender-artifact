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

package handlers

import (
	"archive/tar"
	"compress/gzip"
	"encoding/json"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"

	"github.com/mendersoftware/mender-artifact/artifact"
	"github.com/pkg/errors"
)

type ModuleImage struct {
	version      int
	updateType   string
	files        [](*DataFile)
	augmentFiles [](*DataFile)
	typeInfoV3   *artifact.TypeInfoV3
}

func NewModuleImage(updateType string) *ModuleImage {
	mi := ModuleImage{
		version:    3,
		updateType: updateType,
	}
	return &mi
}

func (img *ModuleImage) Copy() Installer {
	newImg := ModuleImage{
		version:      img.version,
		updateType:   img.updateType,
		files:        make([](*DataFile), len(img.files)),
		augmentFiles: make([](*DataFile), len(img.augmentFiles)),
	}
	for n, f := range img.files {
		newImg.files[n] = new(DataFile)
		*newImg.files[n] = *f
	}
	for n, f := range img.augmentFiles {
		newImg.augmentFiles[n] = new(DataFile)
		*newImg.augmentFiles[n] = *f
	}
	return &newImg
}

func (img *ModuleImage) GetType() string {
	return img.updateType
}

func (img *ModuleImage) GetUpdateFiles() [](*DataFile) {
	return img.files
}

func (img *ModuleImage) GetUpdateAugmentFiles() [](*DataFile) {
	return img.augmentFiles
}

func (img *ModuleImage) SetUpdateFiles(files [](*DataFile)) error {
	img.files = files
	return nil
}

func (img *ModuleImage) SetUpdateAugmentFiles(files [](*DataFile)) error {
	img.augmentFiles = files
	return nil
}

func (img *ModuleImage) GetUpdateAllFiles() [](*DataFile) {
	allFiles := make([](*DataFile), 0, len(img.files)+len(img.augmentFiles))
	for n := range img.files {
		allFiles = append(allFiles, img.files[n])
	}
	for n := range img.augmentFiles {
		allFiles = append(allFiles, img.augmentFiles[n])
	}
	return allFiles
}

func (img *ModuleImage) GetUpdateDepends() *artifact.TypeInfoDepends {
	return img.typeInfoV3.ArtifactDepends
}

func (img *ModuleImage) GetUpdateProvides() *artifact.TypeInfoProvides {
	return img.typeInfoV3.ArtifactProvides
}

func (img *ModuleImage) ComposeHeader(args *ComposeHeaderArgs) error {
	if img.version < 3 {
		return errors.New("artifact version < 3 in ModuleImage.ComposeHeader. This is a bug in the application")
	}

	img.typeInfoV3 = args.TypeInfoV3

	path := artifact.UpdateHeaderPath(args.No)

	if err := writeTypeInfoV3(&WriteInfoArgs{
		tarWriter:  args.TarWriter,
		dir:        path,
		typeinfov3: args.TypeInfoV3,
	}); err != nil {
		return errors.Wrap(err, "ComposeHeader: ")
	}

	if len(args.MetaData) > 0 {
		sw := artifact.NewTarWriterStream(args.TarWriter)
		data, err := json.Marshal(args.MetaData)
		if err != nil {
			return errors.Wrap(err, "MetaData field unmarshalable. This is a bug in the application")
		}
		if err = sw.Write(data, filepath.Join(path, "meta-data")); err != nil {
			return errors.Wrap(err, "update: can not store meta-data")
		}
	}
	return nil
}

func (img *ModuleImage) ComposeData(tw *tar.Writer, no int) error {
	f, ferr := ioutil.TempFile("", "data")
	if ferr != nil {
		return errors.New("update: can not create temporary data file")
	}
	defer os.Remove(f.Name())

	err := func() error {
		gz := gzip.NewWriter(f)
		defer gz.Close()

		tarw := tar.NewWriter(gz)
		defer tarw.Close()

		for _, file := range img.GetUpdateAllFiles() {
			df, err := os.Open(file.Name)
			if err != nil {
				return errors.Wrapf(err, "update: can not open data file: %s", file.Name)
			}
			fw := artifact.NewTarWriterFile(tarw)
			if err := fw.Write(df, filepath.Base(file.Name)); err != nil {
				df.Close()
				return errors.Wrapf(err,
					"update: can not write tar temp data header: %v", file)
			}
			df.Close()
		}
		return nil
	}()

	if err != nil {
		return err
	}

	if _, err = f.Seek(0, 0); err != nil {
		return errors.Wrap(err, "update: can not reset file position")
	}

	dfw := artifact.NewTarWriterFile(tw)
	if err = dfw.Write(f, artifact.UpdateDataPath(no)); err != nil {
		return errors.Wrap(err, "update: can not write tar data header")
	}
	return nil
}

func (img *ModuleImage) ReadHeader(r io.Reader, path string, version int, augmented bool) error {
	img.version = version
	switch {
	case filepath.Base(path) == "type-info",
		filepath.Base(path) == "meta-data":
		// TODO: implement when needed
	case match(artifact.HeaderDirectory+"/*/scripts/*/*", path):
		if augmented {
			return errors.New("scripts not allowed in augmented header")
		}
		// TODO: implement when needed
	default:
		return errors.Errorf("update: unsupported file: %v", path)
	}
	return nil
}

func (img *ModuleImage) Install(r io.Reader, info *os.FileInfo) error {
	print("TODO!!!\n")
	io.Copy(ioutil.Discard, r)
	return nil
}
