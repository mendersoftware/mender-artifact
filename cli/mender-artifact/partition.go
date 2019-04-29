// Copyright 2019 Northern.tech AS
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

	"github.com/mendersoftware/mender-artifact/artifact"
	"github.com/mendersoftware/mender-artifact/utils"
	"github.com/pkg/errors"
)

const (
	fat = iota
	ext
	unsupported

	// empty placeholder, so that we can write virtualPartitionFile.Open()
	// as the API call (better semantics).
	virtualPartitionFile vFile = 1
)

// V(irtual)P(artition)File mimicks a file in an Artifact or on an sdimg.
type VPFile interface {
	io.ReadWriteCloser
	Delete(recursive bool) error
}

type partition struct {
	offset string
	size   string
	path   string
	name   string
}

type vFile int

// Open is a utility function that parses an input image and path
// and returns a V(irtual)P(artition)File.
func (v vFile) Open(comp artifact.Compressor, imgpath string) (VPFile, error) {
	imgname, fpath, err := parseImgPath(imgpath)
	if err != nil {
		return nil, err
	}
	modcands, isArtifact, err := getCandidatesForModify(imgname)
	if err != nil {
		return nil, err
	}
	for i := 0; i < len(modcands); i++ {
		modcands[i].name = imgname
	}
	if isArtifact {
		return newArtifactExtFile(comp, fpath, modcands[0])
	}
	return newSDImgFile(fpath, modcands)

}

// parseImgPath parses cli input of the form
// path/to/[sdimg,mender]:/path/inside/img/file
// into path/to/[sdimg,mender] and path/inside/img/file
func parseImgPath(imgpath string) (imgname, fpath string, err error) {
	paths := strings.SplitN(imgpath, ":", 2)
	if len(paths) != 2 {
		return "", "", errors.New("failed to parse image path")
	}
	if len(paths[1]) == 0 {
		return "", "", errors.New("please enter a path into the image")
	}
	return paths[0], paths[1], nil
}

// imgFilesystemtype returns the filesystem type of a partition.
// Currently only distinguishes ext from fat.
func imgFilesystemType(imgpath string) (int, error) {
	bin, err := utils.GetBinaryPath("blkid")
	if err != nil {
		return unsupported, fmt.Errorf("`blkid` binary not found on the system")
	}
	cmd := exec.Command(bin, "-s", "TYPE", imgpath)
	buf := bytes.NewBuffer(nil)
	cmd.Stdout = buf
	if err := cmd.Run(); err != nil {
		return unsupported, errors.Wrap(err, "imgFilesystemType: blkid command failed")
	}
	if strings.Contains(buf.String(), `TYPE="vfat"`) {
		return fat, nil
	} else if strings.Contains(buf.String(), `TYPE="ext`) {
		return ext, nil
	}
	return unsupported, nil
}

// sdimgFile is a virtual file for files on an sdimg.
// It can write and read to and from all partitions (boot,rootfsa,roofsb,data),
// where if a file is located on the rootfs, a write will be duplicated to both
// partitions.
type sdimgFile []VPFile

func newSDImgFile(fpath string, modcands []partition) (sdimgFile, error) {
	var delPartition []partition
	defer func() {
		for _, part := range delPartition {
			os.Remove(part.path)
		}
	}()
	if len(modcands) < 4 {
		return nil, fmt.Errorf("newSDImgFile: %d partitions found, 4 needed", len(modcands))
	}
	// Only return the data partition.
	if strings.HasPrefix(fpath, "/data") {
		// The data dir is not a directory in the data partition
		fpath = strings.TrimPrefix(fpath, "/data")
		delPartition = append(delPartition, modcands[0:3]...)
		ext, err := newExtFile(fpath, modcands[3]) // Data partition
		return sdimgFile{ext}, err
	}
	reg := regexp.MustCompile("/(uboot|boot/(efi|grub))")
	// Only return the boot-partition.
	if reg.MatchString(fpath) {
		// /uboot, /boot/efi, /boot/grup are not directories on the boot partition.
		fpath = reg.ReplaceAllString(fpath, "")
		// Since boot partitions can be either fat or ext,
		// return a readWriteCloser dependent upon the underlying filesystem type.
		fstype, err := imgFilesystemType(modcands[0].path)
		if err != nil {
			return nil, errors.Wrap(err, "partition: error reading file-system type on the boot partition")
		}
		switch fstype {
		case fat:
			delPartition = append(delPartition, modcands[1:]...)
			ff, err := newFatFile(fpath, modcands[0]) // Boot partition.
			return sdimgFile{ff}, err
		case ext:
			delPartition = append(delPartition, modcands[1:]...)
			extf, err := newExtFile(fpath, modcands[0])
			return sdimgFile{extf}, err
		case unsupported:
			return nil, errors.New("partition: error reading file-system type on the boot partition")

		}
	}
	delPartition = append(delPartition, modcands[0])
	delPartition = append(delPartition, modcands[2:]...)
	rootfsa, err := newExtFile(fpath, modcands[1]) // rootfsa
	if err != nil {
		return sdimgFile{rootfsa}, err
	}
	delPartition = []partition{modcands[0], modcands[3]}
	rootfsb, err := newExtFile(fpath, modcands[2]) // rootfsb
	return sdimgFile{rootfsa, rootfsb}, err
}

// Write forces a write from the underlying writers sdimgFile wraps.
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
	// A read from the first partition wrapped should suffice in all cases.
	return p[0].Read(b)
}

// Read reads a file from an sdimg.
func (p sdimgFile) Delete(recursive bool) (err error) {
	for _, part := range p {
		err = part.Delete(recursive)
		if err != nil {
			return err
		}
	}
	return nil
}

// Close closes the underlying closers.
func (p sdimgFile) Close() (err error) {
	if p == nil {
		return nil
	}
	for _, part := range p {
		err = part.Close()
		if err != nil {
			return err
		}
	}
	return nil
}

// artifactExtFile is a wrapper for a reader and writer to the underlying
// file in a mender artifact with an ext file system.
type artifactExtFile struct {
	extFile
	comp artifact.Compressor
}

func newArtifactExtFile(comp artifact.Compressor, fpath string, p partition) (af *artifactExtFile, err error) {
	tmpf, err := ioutil.TempFile("", "mendertmp-artifactextfile")
	// Cleanup resources in case of error.
	af = &artifactExtFile{
		extFile{
			partition:     p,
			imagefilepath: fpath,
			tmpf:          tmpf,
		},
		comp,
	}
	if err != nil {
		return af, err
	}
	reg := regexp.MustCompile("/(uboot|boot/(efi|grub))")
	if reg.MatchString(fpath) {
		return af, errors.New("newArtifactExtFile: A mender artifact does not contain a boot partition, only a rootfs")
	}
	if strings.HasPrefix(fpath, "/data") {
		return af, errors.New("newArtifactExtFile: A mender artifact does not contain a data partition, only a rootfs")
	}
	return af, nil
}

func (a *artifactExtFile) Close() (err error) {
	if a == nil {
		return nil
	}
	if a.repack {
		err = repackArtifact(a.comp, a.name, a.path, "")
	}
	if a.tmpf != nil {
		a.tmpf.Close()
		os.Remove(a.tmpf.Name())
	}
	os.Remove(a.path)
	return err
}

// extFile wraps partition and implements ReadWriteCloser
type extFile struct {
	partition
	imagefilepath string
	repack        bool     // True if a write has been done
	tmpf          *os.File // Used as a buffer for multiple write operations
}

func newExtFile(imagefilepath string, p partition) (e *extFile, err error) {
	tmpf, err := ioutil.TempFile("", "mendertmp-extfile")
	// Cleanup resources in case of error.
	e = &extFile{
		partition:     p,
		imagefilepath: imagefilepath,
		tmpf:          tmpf,
	}
	return e, err
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
	defer os.RemoveAll(str) // ignore error removing tmp-dir
	if err != nil {
		return 0, errors.Wrap(err, "extFile: ReadError: debugfsCopyFile failed")
	}
	data, err := ioutil.ReadFile(filepath.Join(str, filepath.Base(ef.imagefilepath)))
	if err != nil {
		return 0, errors.Wrapf(err, "extFile: ReadError: ioutil.Readfile failed to read file: %s", filepath.Join(str, filepath.Base(ef.imagefilepath)))
	}
	return copy(b, data), io.EOF
}

func (ef *extFile) Delete(recursive bool) (err error) {
	err = debugfsRemoveFileOrDir(ef.imagefilepath, ef.path, recursive)
	if err != nil {
		return err
	}
	ef.repack = true
	return nil
}

// Close closes the temporary file held by partitionFile path, and repacks the partition if needed.
func (ef *extFile) Close() (err error) {
	if ef == nil {
		return nil
	}
	if ef.repack {
		part := []partition{ef.partition}
		err = repackSdimg(part, ef.name)
	}
	if ef.tmpf != nil {
		// Ignore tmp-errors
		ef.tmpf.Close()
		os.Remove(ef.tmpf.Name())
	}
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

func newFatFile(imageFilePath string, partition partition) (*fatFile, error) {
	tmpf, err := ioutil.TempFile("", "mendertmp-fatfile")
	ff := &fatFile{
		partition:     partition,
		imageFilePath: imageFilePath,
		tmpf:          tmpf,
	}
	return ff, err
}

// Read Dump the file contents to stdout, and capture, using MTools' mtype
func (f *fatFile) Read(b []byte) (n int, err error) {
	cmd := exec.Command("mtype", "-i", f.path, "::"+f.imageFilePath)
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
	cmd := exec.Command("mcopy", "-n", "-i", f.path, f.tmpf.Name(), "::"+f.imageFilePath)
	data := bytes.NewBuffer(nil)
	cmd.Stdout = data
	if err = cmd.Run(); err != nil {
		return 0, errors.Wrap(err, "fatFile: Write: MTools execution failed")
	}
	f.repack = true
	return len(b), nil
}

func (f *fatFile) Delete(recursive bool) (err error) {
	isDir := filepath.Dir(f.imageFilePath) == strings.TrimRight(f.imageFilePath, "/")
	var deleteCmd string
	if isDir {
		deleteCmd = "mdeltree"
	} else {
		deleteCmd = "mdel"
	}
	cmd := exec.Command(deleteCmd, "-i", f.path, "::"+f.imageFilePath)
	if err = cmd.Run(); err != nil {
		return errors.Wrap(err, "fatFile: Delete: execution failed: "+deleteCmd)
	}
	f.repack = true
	return nil
}

func (f *fatFile) Close() (err error) {
	if f == nil {
		return nil
	}
	if f.repack {
		p := []partition{f.partition}
		err = repackSdimg(p, f.name)
	}
	if f.tmpf != nil {
		f.tmpf.Close()
		os.Remove(f.tmpf.Name())
	}
	os.Remove(f.path) // Ignore error for tmp-dir
	return err
}
