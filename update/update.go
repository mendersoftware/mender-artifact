// Copyright 2016 Mender Software AS
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

package update

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/mendersoftware/mender-artifact/archiver"
	"github.com/mendersoftware/mender-artifact/metadata"
	"github.com/pkg/errors"
)

const (
	headerDirectory = "headers"
	dataDirectory   = "data"
)

// UpdateFile represents the minimum set of attributes each update file
// must contain. Some of those might be empty though for specific update types.
type File struct {
	// name of the update file
	Name string
	// size of the update file
	Size int64
	// last modification time
	Date time.Time
	// checksum of the update file
	Checksum []byte
}

type Composer interface {
	GetUpdateFiles() [](*File)
	GetType() string
	ComposeHeader(tw *tar.Writer) error
	ComposeData(tw *tar.Writer) error
}

type Updates struct {
	U []Composer
}

type Installer interface {
	GetUpdateFiles() [](*File)
	GetType() string
	Copy() Installer
	SetFromHeader(r io.Reader, name string) error
	Install(r io.Reader, f os.FileInfo) error
}

// Rootfs handles updates of type 'rootfs-image'. The parser can be
// initialized setting `W` (io.Writer the update data gets written to), or
// `DataFunc` (user provided callback that handlers the update data stream).
type Rootfs struct {
	version int
	update  *File

	InstallHandler func(io.Reader, *File) error

	no int
}

func NewRootfsV1(updFile string) *Rootfs {
	uf := &File{
		Name: updFile,
	}
	return &Rootfs{
		update:  uf,
		version: 1,
	}
}

func NewRootfsV2(updFile string) *Rootfs {
	uf := &File{
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
		update: new(File),
	}
}

func (rp *Rootfs) Copy() Installer {
	return &Rootfs{
		version: rp.version,
		update:  new(File),
	}
}

func parseFiles(r io.Reader) (*metadata.Files, error) {
	files := new(metadata.Files)
	if _, err := io.Copy(files, r); err != nil {
		return nil, errors.Wrapf(err, "update: error reading files")
	}
	return files, nil
}

func (rp *Rootfs) SetFromHeader(r io.Reader, path string) error {
	switch {
	case filepath.Base(path) == "files":

		files, err := parseFiles(r)
		if err != nil {
			return err
		}
		// TODO:
		rp.update.Name = files.FileList[0]
	case filepath.Base(path) == "type-info":
		// TODO:

	case filepath.Base(path) == "meta-data":
		// TODO:
	case strings.HasPrefix(path, "checksums"):
		buf := bytes.NewBuffer(nil)
		if _, err := io.Copy(buf, r); err != nil {
			return errors.Wrapf(err, "update: error reading checksum")
		}
		rp.update.Checksum = buf.Bytes()
	case strings.HasPrefix(path, "signatures"):
	case strings.HasPrefix(path, "scripts"):

	default:
		return errors.Errorf("update: unsupported file: %v", path)
	}
	return nil
}

func ReadAndInstall(r io.Reader, i Installer) error {
	// each data file is stored in tar.gz format
	gz, err := gzip.NewReader(r)
	if err != nil {
		return errors.Wrapf(err, "update: can not open gz for reading data")
	}
	defer gz.Close()

	tar := tar.NewReader(gz)

	for {
		hdr, err := tar.Next()
		if err != nil {
			return errors.Wrapf(err, "update: error reading archive")
		}
		if err == io.EOF {
			break
		} else if err != nil {
			return errors.Wrap(err, "update: error reading update file header")
		}
		if err := i.Install(r, hdr.FileInfo()); i != nil {
			return errors.Wrapf(err, "update: can not install update: %v", hdr)
		}
	}
	return nil
}

func (rfs *Rootfs) Install(r io.Reader, info os.FileInfo) error {
	// we have only one update file in rootfs-image type
	rfs.update.Date = info.ModTime()
	rfs.update.Size = info.Size()

	// TODO: check checksum

	if rfs.InstallHandler != nil {
		return rfs.InstallHandler(r, rfs.update)
	}
	return nil
}

func (rfs *Rootfs) GetUpdateFiles() [](*File) {
	return [](*File){rfs.update}
}

func (rfs *Rootfs) GetType() string {
	return "rootfs-image"
}

func writeFiles(tw *tar.Writer, updFiles []string, dir string) error {
	files := new(metadata.Files)
	for _, u := range updFiles {
		files.FileList = append(files.FileList, u)
	}
	fs := archiver.ToStream(files)
	sa := archiver.NewWriterStream(tw)
	if err := sa.WriteHeader(fs, filepath.Join(dir, "files")); err != nil {
		return errors.Wrapf(err, "writer: can not tar files")
	}
	if n, err := sa.Write(fs); err != nil || n != len(fs) {
		return errors.New("writer: can not store files")
	}
	return nil
}

func writeTypeInfo(tw *tar.Writer, updateType string, dir string) error {
	tInfo := metadata.TypeInfo{Type: updateType}
	info, err := json.Marshal(&tInfo)
	if err != nil {
		return errors.Wrapf(err, "update: can not create type-info")
	}

	w := archiver.NewWriterStream(tw)
	if err := w.WriteHeader(info, filepath.Join(dir, "type-info")); err != nil {
		return errors.Wrapf(err, "update: can not tar type-info")
	}
	if n, err := w.Write(info); err != nil || n != len(info) {
		return errors.New("update: can not store type-info")
	}
	return nil
}

func writeChecksums(tw *tar.Writer, files [](*File), dir string) error {
	for _, f := range files {
		w := archiver.NewWriterStream(tw)
		if err := w.WriteHeader(f.Checksum,
			filepath.Join(dir, filepath.Base(f.Name)+".sha256sum")); err != nil {
			return errors.Wrapf(err, "update: can not tar checksum for %v", f)
		}
		if n, err := w.Write(f.Checksum); err != nil || n != len(f.Checksum) {
			return errors.Wrapf(err, "update: can not store checksum for: %v", f)
		}
	}
	return nil
}

func (rfs *Rootfs) updateHeaderPath() string {
	return filepath.Join(headerDirectory, fmt.Sprintf("%04d", rfs.no))
}

func (rfs *Rootfs) updateDataPath() string {
	return filepath.Join(dataDirectory, fmt.Sprintf("%04d.tar.gz", rfs.no))
}

func (rfs *Rootfs) ComposeHeader(tw *tar.Writer) error {

	path := rfs.updateHeaderPath()

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
		if err := writeChecksums(tw, [](*File){rfs.update},
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

		fw := archiver.NewWriterFile(tarw)

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

	dfw := archiver.NewWriterFile(tw)
	if err = dfw.WriteHeader(f.Name(), rfs.updateDataPath()); err != nil {
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
