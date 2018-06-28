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
	"bytes"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/pkg/errors"
)

type partition struct {
	offset string
	size   string
	path   string
	name   string
}

// sdimgFile is a wrapper around partitionFile, so that
// a write is duplicated to both sdimgFile' files during a write
type sdimgFile []io.ReadWriteCloser

func newSDImgFile(fpath string, modcands []partition) (sdimgFile, error) {
	if len(modcands) < 4 {
		return nil, fmt.Errorf("newSDImgFile: %d partitions found, 4 needed", len(modcands))
	}
	if strings.HasPrefix(fpath, "/data") {
		// The data dir is not a directory in the data partition
		fpath = strings.TrimPrefix(fpath, "/data")
		ext, err := newExtFile("", fpath, modcands[3]) // Data partition
		if err != nil {
			return nil, err
		}
		return sdimgFile{ext}, nil
	}
	reg := regexp.MustCompile("/(uboot|boot/(efi|grub))")
	// Only return the boot-partition.
	if reg.MatchString(fpath) {
		// Map /uboot /boot/efi /boot/grub to the boot partition.
		fpath = reg.ReplaceAllString(fpath, "")
		// Check if the file is on an ext or fat file-system.
		fstype, err := imgFilesystemType(modcands[0].path)
		if err != nil {
			return nil, errors.Wrap(err, "partition: error reading file-system type on the boot partition")
		}
		tmpf, err := ioutil.TempFile("", "mendertmp")
		if err != nil {
			return nil, err
		}
		// Handle ext and fat independently
		switch fstype {
		case fat:
			return sdimgFile{
				&fatFile{
					partition:     modcands[0],
					imageFilePath: fpath,
					tmpf:          tmpf,
				},
			}, nil
		case ext:
			return sdimgFile{
				&extFile{
					partition:     modcands[0], // boot partition
					imagefilepath: fpath,
					tmpf:          tmpf,
				},
			}, nil
		case unsupported:
			return nil, errors.New("partition: error reading file-system type on the boot partition")

		}
	}
	rootfsa, err := newExtFile("", fpath, modcands[1])
	if err != nil {
		return nil, err
	}
	rootfsb, err := newExtFile("", fpath, modcands[2])
	if err != nil {
		return nil, err
	}
	// return a virtual partition read/writer, wrapping both rootfsA and B partitions
	ps := sdimgFile{
		rootfsa,
		rootfsb,
	}
	return ps, nil
}

// Write writes a file to both sdimg partitions.
func (p sdimgFile) Write(b []byte) (int, error) {
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
func (p sdimgFile) Read(b []byte) (int, error) {
	// the partitons should be equal, so only read out of the first one
	return p[0].Read(b)
}

// Close closes the partitions held in the parittions array
// and closes them in turn.
func (p sdimgFile) Close() (err error) {
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

// NewPartitionFile is a utility function that parses an input image and path
// and returns one of the underlying file reader/writers.
func NewPartitionFile(imgpath, key string) (io.ReadWriteCloser, error) {
	imgname, fpath, err := parseImgPath(imgpath)
	if err != nil {
		return nil, err
	}
	modcands, isArtifact, err := getCandidatesForModify(imgname, []byte(key))
	if err != nil {
		return nil, err
	}
	for i := 0; i < len(modcands); i++ {
		modcands[i].name = imgname
	}
	if isArtifact {
		return newArtifactExtFile(key, fpath, modcands[0])
	}
	return newSDImgFile(fpath, modcands)
}

const (
	fat = iota
	ext
	unsupported
)

// imgFilesystemtype returns the filesystem type of a partition.
// Currently only distinguishes ext from fat.
func imgFilesystemType(imgpath string) (int, error) {
	cmd := exec.Command("file", imgpath)
	data := bytes.NewBuffer(nil)
	cmd.Stdout = data
	err := cmd.Run()
	if err != nil {
		return unsupported, err
	}
	s := data.String()
	if strings.Contains(s, "DOS") {
		return fat, nil
	} else if strings.Contains(s, "ext") {
		return ext, nil
	}
	return unsupported, nil
}

// artifactExtFile is a wrapper for a reader and writer to the underlying
// file in a mender artifact with an ext file system.
type artifactExtFile struct {
	extFile
}

func newArtifactExtFile(key, fpath string, p partition) (*artifactExtFile, error) {
	tmpf, err := ioutil.TempFile("", "mendertmp")
	if err != nil {
		return nil, err
	}
	// Strip the possible xtra boot paths
	reg := regexp.MustCompile("/(uboot|boot/(efi|grub))")
	// Only return the boot-partition.
	if reg.MatchString(fpath) {
		return nil, errors.New("newArtifactExtFile: A mender artifact does not contain a boot partition, only a rootfs")
	}
	return &artifactExtFile{
		extFile{
			partition:     p,
			key:           key,
			imagefilepath: fpath,
			tmpf:          tmpf,
		},
	}, nil
}

func (a *artifactExtFile) Close() error {
	os.Remove(a.tmpf.Name())
	if a.repack {
		return repackArtifact(a.name, a.path,
			a.key, filepath.Base(a.name))
	}
	return nil
}

// extFile wraps partition and implements ReadWriteCloser
type extFile struct {
	partition
	key           string
	imagefilepath string
	repack        bool     // True if a write has been done
	tmpf          *os.File // Used as a buffer for multiple write operations
}

func newExtFile(key, imagefilepath string, p partition) (*extFile, error) {
	tmpf, err := ioutil.TempFile("", "mendertmp")
	if err != nil {
		return nil, err
	}
	return &extFile{
		partition:     p,
		key:           key,
		imagefilepath: imagefilepath,
		tmpf:          tmpf,
	}, nil
}

// Write reads all bytes from b into the partitionFile using debugfs.
func (ef *extFile) Write(b []byte) (int, error) {
	var err error
	if _, err = ef.tmpf.WriteAt(b, 0); err != nil {
		return 0, err
	}
	ef.tmpf.Sync()

	err = debugfsReplaceFile(ef.imagefilepath, ef.tmpf.Name(), ef.path)
	if err != nil {
		return 0, err
	}
	ef.repack = true
	return len(b), nil
}

// Read reads all bytes from the filepath on the partition image into b
func (ef *extFile) Read(b []byte) (int, error) {
	str, err := debugfsCopyFile(ef.imagefilepath, ef.path)
	if err != nil {
		return 0, errors.Wrap(err, "extFile: ReadError: debugfsCopyFile failed")
	}
	data, err := ioutil.ReadFile(filepath.Join(str, filepath.Base(ef.imagefilepath)))
	if err != nil {
		return 0, errors.Wrapf(err, "extFile: ReadError: ioutil.Readfile failed to read file: %s", filepath.Join(str, filepath.Base(ef.imagefilepath)))
	}
	defer os.RemoveAll(str) // ignore error removing tmp-dir
	return copy(b, data), io.EOF
}

// Close closes the temporary file held by partitionFile path, and repacks the partition if needed.
func (ef *extFile) Close() (err error) {
	if ef.repack {
		part := []partition{ef.partition}
		err = repackSdimg(part, ef.name)
	}
	ef.tmpf.Close()    // Ignore
	os.Remove(ef.path) // ignore error for tmp-dir
	return err
}

// fatFile wraps a partition struct with a reader/writer for fat filesystems
type fatFile struct {
	partition
	imageFilePath string // The local filesystem path to the image
	repack        bool
	tmpf          *os.File
}

func newFatFile(imagefilepath string, p partition) (*fatFile, error) {
	tmpf, err := ioutil.TempFile("", "mendertmp")
	if err != nil {
		return nil, err
	}
	return &fatFile{
		partition:     p,
		imageFilePath: imagefilepath,
		tmpf:          tmpf,
	}, nil
}

// Read Dump the file contents to stdout, and capture, using MTools' mtype
func (f *fatFile) Read(b []byte) (n int, err error) {
	cmd := exec.Command("mtype", "-i", f.path, "::/"+f.imageFilePath)
	dbuf := bytes.NewBuffer(nil)
	cmd.Stdout = dbuf // capture Stdout
	if err = cmd.Run(); err != nil {
		return 0, errors.Wrap(err, "fatPartitionFile: Read: MTools mtype dump failed")
	}
	return copy(b, dbuf.Bytes()), io.EOF
}

// Write Writes to the underlying fat image, using MTools' mcopy
func (f *fatFile) Write(b []byte) (n int, err error) {
	if _, err := f.tmpf.WriteAt(b, 0); err != nil {
		return 0, errors.Wrap(err, "fatFile: Write: Failed to write to tmpfile")
	}
	if err = f.tmpf.Sync(); err != nil {
		return 0, errors.Wrap(err, "fatFile: Write: Failed to sync tmpfile")
	}
	// Use MTools to write to the fat-partition
	cmd := exec.Command("mcopy", "-i", f.path, f.tmpf.Name(), "::/"+f.imageFilePath)
	data := bytes.NewBuffer(nil)
	cmd.Stdout = data
	if err = cmd.Run(); err != nil {
		return 0, errors.Wrap(err, "fatFile: Write: MTools execution failed")
	}
	f.repack = true
	return len(b), nil
}

func (f *fatFile) Close() (err error) {
	if f.repack {
		p := []partition{f.partition}
		err = repackSdimg(p, f.name)
	}
	f.tmpf.Close()
	os.Remove(f.path) // Ignore error for tmp-dir
	return err
}
