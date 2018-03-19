// Copyright 2018 Northern.tech AS
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

package main

import (
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"

	"github.com/pkg/errors"
)

// PartitionReadWriteClosePacker wraps io.ReadWriteCloser with a Repack method
type PartitionReadWriteClosePacker interface {
	io.ReadWriteCloser
	Repack() error
}

type partition struct {
	offset string
	size   string
	path   string
	name   string
}

// partitions is a wrapper around partitionFile, so that
// a write is duplicated to both partitions' files during a write
type partitions []partitionFile

// Write writes a file to both sdimg partitions.
func (p partitions) Write(b []byte) (int, error) {
	for _, part := range p {
		n, err := part.Write(b)
		if err != nil {
			return n, err
		}
		if n != len(b) {
			return n, io.ErrShortWrite
		}
	}
	return len(b), nil
}

// Read reads a file from an sdimg.
func (p partitions) Read(b []byte) (int, error) {
	// the partitons should be equal, so only read out of the first one
	return p[0].Read(b)
}

// Repack repacks the sdimg.
func (p partitions) Repack() error {
	// make modified images part of sdimg again
	var ps []partition
	for _, pf := range p {
		ps = append(ps, pf.partition)
	}
	return repackSdimg(ps, ps[0].name)
}

// Close closes the partitions held in the parittions array
// and closes them in turn.
func (p partitions) Close() (err error) {
	for _, part := range p {
		err = part.Close()
		if err != nil {
			return err
		}
	}
	return nil
}

// parseImgPath parses cli input of the form
// path/to/[sdimg,mender]:/path/inside/img/file
// into path/to/[sdimg,mender] and path/inside/img/file
func parseImgPath(imgpath string) (imgname, fpath string, err error) {
	paths := strings.SplitN(imgpath, ":", 2)

	if len(paths) != 2 {
		return "", "", errors.New("failed to parse image path")
	}

	if len(paths[0]) < 2 {
		return "", "", errors.New("invalid image or artifact path given")
	}

	if len(paths[1]) == 0 {
		return "", "", errors.New("please enter a path into the image")
	}

	return paths[0], paths[1], nil
}

// NewPartitionFile returns an io.ReadWriteCloser in the form of either a partition file,
// or an array of partitionfiles. Both implementing Read Write and Close.
func NewPartitionFile(imgpath, key string) (PartitionReadWriteClosePacker, error) {
	imgname, fpath, err := parseImgPath(imgpath)
	if err != nil {
		return nil, err
	}

	modcands, isArtifact, err := getCandidatesForModify(imgname, []byte(key))
	if err != nil {
		return nil, err
	}

	modcands[0].name = imgname
	if isArtifact {
		pf := &partitionFile{
			key:           key,
			partition:     modcands[0],
			imagefilepath: fpath,
		}
		return pf, nil
	}
	modcands[1].name = imgname
	var ps partitions = []partitionFile{
		partitionFile{
			partition:     modcands[0],
			imagefilepath: fpath,
		},
		partitionFile{
			partition:     modcands[1],
			imagefilepath: fpath,
		},
	}

	return ps, nil
}

// NewPartitionReader returns a reader for a file located inside
// an image or a mender artifact.
func NewPartitionReader(imgpath, key string) (io.ReadCloser, error) {
	return NewPartitionFile(imgpath, key)
}

// PartitionPacker has the functionality to repack an image or artifact.
type PartitionPacker interface {
	Repack() error
}

// NewPartitionWritePacker returns a writer for files located inside
// an image or a mender artifact, and writes to it.
func NewPartitionWritePacker(imgpath, key string) (io.WriteCloser, error) {
	return NewPartitionFile(imgpath, key)
}

// partitionFile wraps partition and implements ReadWriteCloser
type partitionFile struct {
	partition
	key           string
	imagefilepath string
}

// Write reads all bytes from b into the partitionFile using debugfs.
func (p *partitionFile) Write(b []byte) (int, error) {
	f, err := ioutil.TempFile("", "mendertmp")

	// ignore tempfile os-cleanup errors
	defer f.Close()
	defer os.Remove(f.Name())

	if err != nil {
		return 0, err
	}
	if _, err := f.WriteAt(b, 0); err != nil {
		return 0, err
	}

	err = debugfsReplaceFile(p.imagefilepath, f.Name(), p.path)
	if err != nil {
		return 0, err
	}

	return len(b), nil
}

// Read reads all bytes from the filepath on the partition image into b
func (p *partitionFile) Read(b []byte) (int, error) {
	str, err := debugfsCopyFile(p.imagefilepath, p.path)
	if err != nil {
		return 0, errors.Wrap(err, "ReadError: debugfsCopyFile failed")
	}
	data, err := ioutil.ReadFile(filepath.Join(str, filepath.Base(p.imagefilepath)))
	if err != nil {
		return 0, errors.Wrapf(err, "ReadError: ioutil.Readfile failed to read file: %s", filepath.Join(str, filepath.Base(p.imagefilepath)))
	}
	defer os.RemoveAll(str) // ignore error removing tmp-dir
	return copy(b, data), io.EOF
}

// Close removes the temporary file held by partitionFile path.
func (p *partitionFile) Close() error {
	if p != nil {
		os.Remove(p.path) // ignore error for tmp-dir
	}
	return nil
}

// Repack repacks the artifact or image.
func (p *partitionFile) Repack() error {
	err := repackArtifact(p.name, p.path,
		p.key, filepath.Base(p.name))
	os.Remove(p.path) // ignore error, file exists in /tmp only
	return err
}
