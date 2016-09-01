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

package writer

import (
	"archive/tar"
	"errors"
	"os"
)

// FileArchiver implements ReadArchiver interface
type FileArchiver struct {
	path string
	name string
	file *os.File
}

// NewFileArchiver creates fileArchiver used for storing plain files
// inside tar archive.
// path is the absolute path to the file that will be archived and
// name is the relatve path inside the archive (see tar.Header.Name)
func NewFileArchiver(path, name string) *FileArchiver {
	return &FileArchiver{name: name, path: path}
}

// Open is opening file for reading before storing it into archive.
// It is not returning open file descriptor, but rather it is setting
// FileArchiver file field to be an open descriptor.
func (f *FileArchiver) Open() error {
	fd, err := os.Open(f.path)
	if err != nil {
		return err
	}
	f.file = fd
	return nil
}

func (f *FileArchiver) Read(p []byte) (n int, err error) {
	if f.file == nil {
		return 0, errors.New("attempt to read from closed file")
	}
	return f.file.Read(p)
}

// Close is a path of ReadArchiver interface
func (f *FileArchiver) Close() error {
	if f.file != nil {
		return f.file.Close()
	}
	return errors.New("file already closed")
}

// GetHeader is a path of ReadArchiver interface. It returns tar.Header which
// is then writtem as a part of archive header.
func (f *FileArchiver) GetHeader() (*tar.Header, error) {
	info, err := os.Stat(f.path)
	if err != nil {
		return nil, err
	}
	hdr, err := tar.FileInfoHeader(info, "")
	if err != nil {
		return nil, err
	}
	hdr.Name = f.name
	return hdr, nil
}
