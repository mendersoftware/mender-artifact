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

package artifact

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/pkg/errors"
)

const (
	HeaderDirectory = "headers"
	DataDirectory   = "data"
)

// File represents the minimum set of attributes each update file
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
	ComposeHeader(tw *tar.Writer, no int) error
	ComposeData(tw *tar.Writer, no int) error
}

type Updates struct {
	U []Composer
}

type Installer interface {
	GetUpdateFiles() [](*File)
	GetType() string
	Copy() Installer
	ReadHeader(r io.Reader, path string) error
	Install(r io.Reader, info *FileInfoChecksum) error
}

type FileInfoChecksum struct {
	os.FileInfo
	Checksum []byte
}

func UpdatePath(no int) string {
	return fmt.Sprintf("%04d", no)
}

func UpdateHeaderPath(no int) string {
	return filepath.Join(HeaderDirectory, fmt.Sprintf("%04d", no))
}

func UpdateDataPath(no int) string {
	return filepath.Join(DataDirectory, fmt.Sprintf("%04d.tar.gz", no))
}

func ReadAndInstall(r io.Reader, i Installer,
	manifest *ChecksumStore, no int) error {
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

		// check checksum
		ch := NewReaderChecksum(tar)
		var check []byte

		if manifest != nil {
			check, err = manifest.Get(filepath.Join(UpdatePath(no), hdr.FileInfo().Name()))
			if err != nil {
				return errors.Wrapf(err, "update: checksum missing")
			}
		}

		info := &FileInfoChecksum{
			FileInfo: hdr.FileInfo(),
			Checksum: check,
		}

		if err := i.Install(ch, info); i != nil {
			return errors.Wrapf(err, "update: can not install update: %v", hdr)
		}

		checksum := ch.Checksum()
		if bytes.Compare(info.Checksum, checksum) != 0 {
			return errors.Errorf("update: invalid data file [%s] checksum (%s) -> (%s)",
				info.Name(), info.Checksum, checksum)
		}
	}
	return nil
}
