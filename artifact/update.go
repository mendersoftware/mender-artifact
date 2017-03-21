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
type DataFile struct {
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
	GetUpdateFiles() [](*DataFile)
	GetType() string
	ComposeHeader(tw *tar.Writer, no int) error
	ComposeData(tw *tar.Writer, no int) error
}

type Updates struct {
	U []Composer
}

type Installer interface {
	GetUpdateFiles() [](*DataFile)
	GetType() string
	Copy() Installer
	ReadHeader(r io.Reader, path string) error
	Install(r io.Reader, info *os.FileInfo) error
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

func getDataFile(i Installer, name string) *DataFile {
	for _, file := range i.GetUpdateFiles() {
		if name == file.Name {
			return file
		}
	}
	return nil
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
		if err == io.EOF {
			break
		} else if err != nil {
			return errors.Wrap(err, "update: error reading update file header")
		}

		// check checksum
		ch := NewReaderChecksum(tar)

		df := getDataFile(i, hdr.Name)
		if df == nil {
			return errors.Errorf("update: can not find data file: %s", hdr.Name)
		}

		// fill in needed data
		info := hdr.FileInfo()
		df.Size = info.Size()
		df.Date = info.ModTime()

		// we need to have a checksum either in manifest file (v2 artifact)
		// or it needs to be pre-filled after reading header
		if manifest != nil {
			df.Checksum, err = manifest.Get(filepath.Join(UpdatePath(no), hdr.FileInfo().Name()))
			if err != nil {
				return errors.Wrapf(err, "update: checksum missing")
			}
		}

		if df.Checksum == nil {
			return errors.Errorf("update: checksum missing for file: %s", hdr.Name)
		}

		if err := i.Install(ch, &info); err != nil {
			return errors.Wrapf(err, "update: can not install update: %v", hdr)
		}

		checksum := ch.Checksum()
		if bytes.Compare(df.Checksum, checksum) != 0 {
			return errors.Errorf("update: invalid data file checksum: %s; "+
				"actual:[%s], expected:[%s]",
				info.Name(), checksum, df.Checksum)
		}
	}
	return nil
}
