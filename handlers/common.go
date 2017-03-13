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
	"encoding/json"
	"fmt"
	"io"
	"path/filepath"

	"github.com/mendersoftware/mender-artifact/artifact"
	"github.com/pkg/errors"
)

func updateHeaderPath(no int) string {
	return filepath.Join(artifact.HeaderDirectory, fmt.Sprintf("%04d", no))
}

func updateDataPath(no int) string {
	return filepath.Join(artifact.DataDirectory, fmt.Sprintf("%04d.tar.gz", no))
}

func parseFiles(r io.Reader) (*artifact.Files, error) {
	files := new(artifact.Files)
	if _, err := io.Copy(files, r); err != nil {
		return nil, errors.Wrapf(err, "update: error reading files")
	}
	return files, nil
}

func match(pattern, name string) bool {
	match, err := filepath.Match(pattern, name)
	if err != nil {
		return false
	}
	return match
}

func writeFiles(tw *tar.Writer, updFiles []string, dir string) error {
	files := new(artifact.Files)
	for _, u := range updFiles {
		files.FileList = append(files.FileList, u)
	}
	fs := artifact.ToStream(files)
	sa := artifact.NewWriterStream(tw)
	if err := sa.WriteHeader(fs, filepath.Join(dir, "files")); err != nil {
		return errors.Wrapf(err, "writer: can not tar files")
	}
	if n, err := sa.Write(fs); err != nil || n != len(fs) {
		return errors.New("writer: can not store files")
	}
	return nil
}

func writeTypeInfo(tw *tar.Writer, updateType string, dir string) error {
	tInfo := artifact.TypeInfo{Type: updateType}
	info, err := json.Marshal(&tInfo)
	if err != nil {
		return errors.Wrapf(err, "update: can not create type-info")
	}

	w := artifact.NewWriterStream(tw)
	if err := w.WriteHeader(info, filepath.Join(dir, "type-info")); err != nil {
		return errors.Wrapf(err, "update: can not tar type-info")
	}
	if n, err := w.Write(info); err != nil || n != len(info) {
		return errors.New("update: can not store type-info")
	}
	return nil
}

func writeChecksums(tw *tar.Writer, files [](*artifact.File), dir string) error {
	for _, f := range files {
		w := artifact.NewWriterStream(tw)
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
