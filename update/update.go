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
	"encoding/json"
	"fmt"
	"io"
	"path/filepath"
	"time"

	"github.com/mendersoftware/mender-artifact/archiver"
	"github.com/mendersoftware/mender-artifact/metadata"
	"github.com/pkg/errors"
)

const (
	headerDirectory = "headers"
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
}

type Updates struct {
	no  int
	upd []Composer
}

func NewUpdates() *Updates {
	return &Updates{upd: make([]Composer, 1)}
}

func (u *Updates) Add(update Composer) error {
	// TODO: set num for given composer
	u.upd = append(u.upd, update)
	return nil
}

func (u *Updates) Next() (Composer, error) {
	if len(u.upd) > u.no {
		defer func() { u.no++ }()
		return u.upd[u.no], nil
	}
	return nil, io.EOF
}

func (u *Updates) Reset() {
	u.no = 0
}

type Decomposer interface {
}

// Rootfs handles updates of type 'rootfs-image'. The parser can be
// initialized setting `W` (io.Writer the update data gets written to), or
// `DataFunc` (user provided callback that handlers the update data stream).
type Rootfs struct {
	version int
	update  *File

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

func (rfs *Rootfs) GetUpdateFiles() ([](*File), error) {
	return nil, nil
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
	if err := sa.WriteHeader(fs, "files"); err != nil {
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
	if err := w.WriteHeader(info, "type-info"); err != nil {
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
			filepath.Base(f.Name)+".sha256sum"); err != nil {
			return errors.Wrapf(err, "update: can not tar checksum for %v", f)
		}
		if n, err := w.Write(f.Checksum); err != nil || n != len(f.Checksum) {
			return errors.Wrapf(err, "update: can not store checksum for: %v", f)
		}
	}
	return nil
}

func (rfs *Rootfs) tarPath() string {
	return filepath.Join(headerDirectory, fmt.Sprintf("%04d", rfs.no))
}

func (rfs *Rootfs) Compose(tw *tar.Writer) error {

	// first store files
	if err := writeFiles(tw, []string{filepath.Base(rfs.update.Name)},
		rfs.tarPath()); err != nil {
		return err
	}

	// store type-info
	if err := writeTypeInfo(tw, "rootfs-image", rfs.tarPath()); err != nil {
		return err
	}

	if rfs.version == 1 {
		// store checksums
		if err := writeChecksums(tw, [](*File){rfs.update},
			filepath.Join(rfs.tarPath(), "checksums")); err != nil {
			return err
		}
	}
	return nil
}
