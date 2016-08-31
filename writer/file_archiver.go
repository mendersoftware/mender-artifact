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

// implements ReadArchiver interface
type fileArchiver struct {
	path string
	name string
	file *os.File
}

func NewFileArchiver(path, name string) *fileArchiver {
	f, err := os.Open(path)
	if err != nil {
		return nil
	}
	return &fileArchiver{file: f, name: name, path: path}
}

func (f fileArchiver) Read(p []byte) (n int, err error) {
	return f.file.Read(p)
}

func (f fileArchiver) Close() error {
	return f.file.Close()
}

func (f fileArchiver) GetHeader() (*tar.Header, error) {
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
