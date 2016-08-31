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
	f, err := os.Open(path)
	if err != nil {
		return nil
	}
	return &FileArchiver{file: f, name: name, path: path}
}

func (f FileArchiver) Read(p []byte) (n int, err error) {
	return f.file.Read(p)
}

// Close is a path of ReadArchiver interface
func (f FileArchiver) Close() error {
	return f.file.Close()
}

// GetHeader is a path of ReadArchiver interface. It returns tar.Header which
// is then writtem as a part of archive header.
func (f FileArchiver) GetHeader() (*tar.Header, error) {
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
