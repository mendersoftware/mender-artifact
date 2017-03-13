// Copyright 2017 Mender Software AS
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
	"bytes"
	"compress/gzip"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"

	"github.com/mendersoftware/mender-artifact/artifact"
	"github.com/pkg/errors"
)

// Rootfs handles updates of type 'rootfs-image'. The parser can be
// initialized setting `W` (io.Writer the update data gets written to), or
// `DataFunc` (user provided callback that handlers the update data stream).
type Rootfs struct {
	version int
	update  *artifact.File

	InstallHandler func(io.Reader, *artifact.File) error

	no int
}

func NewRootfsV1(updFile string) *Rootfs {
	uf := &artifact.File{
		Name: updFile,
	}
	return &Rootfs{
		update:  uf,
		version: 1,
	}
}

func NewRootfsV2(updFile string) *Rootfs {
	uf := &artifact.File{
		Name: updFile,
	}
	return &Rootfs{
		update:  uf,
		version: 2,
	}
}

// TODO:
func NewRootfsInstaller() *Rootfs {
	return &Rootfs{
		update: new(artifact.File),
	}
}

func (rp *Rootfs) Copy() artifact.Installer {
	return &Rootfs{
		version:        rp.version,
		update:         new(artifact.File),
		InstallHandler: rp.InstallHandler,
	}
}

func (rp *Rootfs) SetFromHeader(r io.Reader, path string) error {
	switch {
	case filepath.Base(path) == "files":

		files, err := parseFiles(r)
		if err != nil {
			return err
		}
		rp.update.Name = files.FileList[0]
	case filepath.Base(path) == "type-info":
		// TODO:

	case filepath.Base(path) == "meta-data":
		// TODO:
	case match(artifact.HeaderDirectory+"/*/checksums/*", path):
		buf := bytes.NewBuffer(nil)
		if _, err := io.Copy(buf, r); err != nil {
			return errors.Wrapf(err, "update: error reading checksum")
		}
		rp.update.Checksum = buf.Bytes()
	case match(artifact.HeaderDirectory+"/*/signatres/*", path):
	case match(artifact.HeaderDirectory+"/*/scripts/*/*", path):

	default:
		return errors.Errorf("update: unsupported file: %v", path)
	}
	return nil
}

func (rfs *Rootfs) Install(r io.Reader, info os.FileInfo) error {
	// we have only one update file in rootfs-image type
	rfs.update.Date = info.ModTime()
	rfs.update.Size = info.Size()

	// check checksum
	ch := artifact.NewReaderChecksum(r)

	if rfs.InstallHandler != nil {
		if err := rfs.InstallHandler(ch, rfs.update); err != nil {
			return errors.Wrap(err, "update: can not install")
		}
		checksum := ch.Checksum()
		if bytes.Compare(rfs.update.Checksum, checksum) != 0 {
			return errors.Errorf("update: invalid data file [%s] checksum (%s) -> (%s)",
				rfs.update.Name, rfs.update.Checksum, checksum)
		}
	}
	return nil
}

func (rfs *Rootfs) GetUpdateFiles() [](*artifact.File) {
	return [](*artifact.File){rfs.update}
}

func (rfs *Rootfs) GetType() string {
	return "rootfs-image"
}

func (rfs *Rootfs) ComposeHeader(tw *tar.Writer) error {

	path := updateHeaderPath(rfs.no)

	// first store files
	if err := writeFiles(tw, []string{filepath.Base(rfs.update.Name)},
		path); err != nil {
		return err
	}

	// store type-info
	if err := writeTypeInfo(tw, "rootfs-image", path); err != nil {
		return err
	}

	if rfs.version == 1 {
		// store checksums
		if err := writeChecksums(tw, [](*artifact.File){rfs.update},
			filepath.Join(path, "checksums")); err != nil {
			return err
		}
	}
	return nil
}

func (rfs *Rootfs) ComposeData(tw *tar.Writer) error {
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

		fw := artifact.NewWriterFile(tarw)

		if err := fw.WriteHeader(rfs.update.Name,
			filepath.Base(rfs.update.Name)); err != nil {
			return errors.Wrapf(err, "update: can not tar temp data header: %v", rfs.update)
		}
		df, err := os.Open(rfs.update.Name)
		if err != nil {
			return errors.Wrapf(err, "update: can not open data file: %v", rfs.update)
		}
		if _, err := io.Copy(fw, df); err != nil {
			return errors.Wrapf(err, "update: can not store temp data file: %v", rfs.update)
		}
		return nil
	}()

	if err != nil {
		return err
	}

	dfw := artifact.NewWriterFile(tw)
	if err = dfw.WriteHeader(f.Name(), updateDataPath(rfs.no)); err != nil {
		return errors.Wrapf(err, "update: can not tar data header: %v", rfs.update)
	}

	if _, err = f.Seek(0, 0); err != nil {
		return errors.Wrapf(err, "update: can not read data file: %v", rfs.update)
	}
	if _, err := io.Copy(dfw, f); err != nil {
		return errors.Wrapf(err, "update: can not store data file: %v", rfs.update)
	}

	return nil
}