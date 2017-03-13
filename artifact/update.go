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
	"compress/gzip"
	"io"
	"os"
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
	SetFromHeader(r io.Reader, path string) error
	Install(r io.Reader, f os.FileInfo) error
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

		if err := i.Install(tar, hdr.FileInfo()); i != nil {
			return errors.Wrapf(err, "update: can not install update: %v", hdr)
		}
	}
	return nil
}
