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
	"os"

	"github.com/pkg/errors"
)

type FileArchiver struct {
	*tar.Writer
}

func NewWriterFile(tw *tar.Writer) *FileArchiver {
	w := FileArchiver{
		Writer: tw,
	}
	return &w
}

func (fa *FileArchiver) WriteHeader(f string, archivePath string) error {
	info, err := os.Stat(f)
	if err != nil {
		return err
	}
	hdr, err := tar.FileInfoHeader(info, "")
	if err != nil {
		return errors.Wrapf(err, "arch: invalid file info header")
	}
	hdr.Name = archivePath
	if err = fa.Writer.WriteHeader(hdr); err != nil {
		return errors.Wrapf(err, "arch: error writing header")
	}
	return nil
}
