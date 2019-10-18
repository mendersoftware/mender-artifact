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
	"strconv"
	"strings"
	"syscall"

	"github.com/mendersoftware/mender-artifact/areader"
	"github.com/mendersoftware/mender-artifact/artifact"
	"github.com/mendersoftware/mender-artifact/utils"
	"github.com/pkg/errors"
)

const (
	fat = iota
	ext
	unsupported

	// empty placeholder, so that we can write virtualImage.Open()
	// as the API call (better semantics).
	virtualImage vImage = 1
)

var errFsTypeUnsupported = errors.New("mender-artifact can only modify ext4 and vfat payloads")

type VPImage interface {
	io.Closer
	Open(fpath string) (VPFile, error)
}

// V(irtual)P(artition)File mimicks a file in an Artifact or on an sdimg.
type VPFile interface {
	io.ReadWriteCloser
	Delete(recursive bool) error
	CopyTo(hostFile string) error
	CopyFrom(hostFile string) error
}

type partition struct {
	offset string
	size   string
	path   string
}

type ModImageBase struct {
	path string
}

type ModImageArtifact struct {
	ModImageBase
	*unpackedArtifact
	comp artifact.Compressor
	key  []byte
}

type ModImageSdimg struct {
	ModImageBase
	candidates []partition
}

type ModImageRaw struct {
	ModImageBase
}

type vImage int

type vImageAndFile struct {
	image VPImage
	file  VPFile
}

// Open is a utility function that parses an input image and path
// and returns a V(irtual)P(artition)File.
func (v vImage) Open(comp artifact.Compressor, key []byte, imgname string) (VPImage, error) {
	// first we need to check  if we are having artifact or image file
	art, err := os.Open(imgname)
	if err != nil {
		return nil, errors.Wrap(err, "can not open artifact")
	}
	defer art.Close()

	aReader := areader.NewReader(art)
	err = aReader.ReadArtifact()
	if err == nil {
		// we have VALID artifact,

		// First check if it is a module-image type
		inst := aReader.GetHandlers()

		if len(inst) > 1 {
			return nil, errors.New("Modifying artifacts with more than one payload is not supported")
		}

		unpackedArtifact, err := unpackArtifact(imgname)
		if err != nil {
			return nil, errors.Wrap(err, "can not process artifact")
		}

		return &ModImageArtifact{
			ModImageBase: ModImageBase{
				path: imgname,
			},
			unpackedArtifact: unpackedArtifact,
			comp:             comp,
			key:              key,
		}, nil
	} else {
		return processSdimg(imgname)
	}
}

// Shortcut to use an image with one file. This is inefficient if you are going
// to write more than one file, since it writes out the entire image
// afterwards. In that case use VPImage and VPFile instead.
func (v vImage) OpenFile(comp artifact.Compressor, key []byte, imgAndPath string) (VPFile, error) {
	imagepath, filepath, err := parseImgPath(imgAndPath)
	if err != nil {
		return nil, err
	}

	image, err := v.Open(comp, key, imagepath)
	if err != nil {
		return nil, err
	}

	file, err := image.Open(filepath)
	if err != nil {
		image.Close()
		return nil, err
	}

	return &vImageAndFile{
		image: image,
		file:  file,
	}, nil
}

// Shortcut to open a file in the image, write into it, and close it again.
func CopyIntoImage(hostFile string, image VPImage, imageFile string) error {
	imageFd, err := image.Open(imageFile)
	if err != nil {
		return err
	}

	err = imageFd.CopyTo(hostFile)
	if err != nil {
		imageFd.Close()
		return err
	}
	return imageFd.Close()
}

// Shortcut to open a file in the image, read from it, and close it again.
func CopyFromImage(image VPImage, imageFile string, hostFile string) error {
	imageFd, err := image.Open(imageFile)
	if err != nil {
		return err
	}

	err = imageFd.CopyFrom(hostFile)
	if err != nil {
		imageFd.Close()
		return err
	}
	return imageFd.Close()
}

func (v *vImageAndFile) Read(buf []byte) (int, error) {
	return v.file.Read(buf)
}

func (v *vImageAndFile) Write(buf []byte) (int, error) {
	return v.file.Write(buf)
}

func (v *vImageAndFile) Delete(recursive bool) error {
	return v.file.Delete(recursive)
}

func (v *vImageAndFile) CopyTo(hostFile string) error {
	return v.file.CopyTo(hostFile)
}

func (v *vImageAndFile) CopyFrom(hostFile string) error {
	return v.file.CopyFrom(hostFile)
}

func (v *vImageAndFile) Close() error {
	fileErr := v.file.Close()
	imageErr := v.image.Close()

	if fileErr != nil {
		return fileErr
	} else {
		return imageErr
	}
}

// Opens a file inside the image(s) represented by the ModImageArtifact
func (i *ModImageArtifact) Open(fpath string) (VPFile, error) {
	return newArtifactExtFile(i, i.comp, fpath, i.files[0])
}

// Opens a file inside the image(s) represented by the ModImageSdimg
func (i *ModImageSdimg) Open(fpath string) (VPFile, error) {
	return newSDImgFile(i, fpath, i.candidates)
}

// Closes and repacks the artifact or sdimg.
func (i *ModImageArtifact) Close() error {
	return repackArtifact(i.comp, i.key, i.unpackedArtifact)
}

func (i *ModImageSdimg) Close() error {
	return repackSdimg(i.candidates, i.path)
}

func (i *ModImageRaw) Open(fpath string) (VPFile, error) {
	return newExtFile(i.path, fpath)
}

func (i *ModImageRaw) Close() error {
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

// From the fsck man page:
// The exit code returned by fsck is the sum of the following conditions:
//
//              0      No errors
//              1      Filesystem errors corrected
//              2      System should be rebooted
//              4      Filesystem errors left uncorrected
//              8      Operational error
//              16     Usage or syntax error
//              32     Checking canceled by user request
//              128    Shared-library error
func runFsck(image, fstype string) error {
	bin, err := utils.GetBinaryPath("fsck." + fstype)
	if err != nil {
		return errors.Wrap(err, "fsck command not found")
	}
	cmd := exec.Command(bin, "-a", image)
	if err := cmd.Run(); err != nil {
		// try to get the exit code
		if exitError, ok := err.(*exec.ExitError); ok {
			ws := exitError.Sys().(syscall.WaitStatus)
			if ws.ExitStatus() == 0 || ws.ExitStatus() == 1 {
				return nil
			}
			if ws.ExitStatus() == 8 {
				return errFsTypeUnsupported
			}
			return errors.Wrap(err, "fsck error")
		}
		return errors.New("fsck returned unparsed error")
	}
	return nil
}

// sdimgFile is a virtual file for files on an sdimg.
// It can write and read to and from all partitions (boot,rootfsa,roofsb,data),
// where if a file is located on the rootfs, a write will be duplicated to both
// partitions.
type sdimgFile []VPFile

func newSDImgFile(image *ModImageSdimg, fpath string, modcands []partition) (sdimgFile, error) {
	if len(modcands) < 4 {
		return nil, fmt.Errorf("newSDImgFile: %d partitions found, 4 needed", len(modcands))
	}

	reg := regexp.MustCompile("/(uboot|boot/(efi|grub))[/]")

	var filesystems []partition
	if strings.HasPrefix(fpath, "/data") {
		// The data dir is not a directory in the data partition
		fpath = strings.TrimPrefix(fpath, "/data/")
		filesystems = append(filesystems, modcands[3])
	} else if reg.MatchString(fpath) {
		// /uboot, /boot/efi, /boot/grub are not directories on the boot partition.
		fpath = reg.ReplaceAllString(fpath, "")
		filesystems = append(filesystems, modcands[0])
	} else {
		filesystems = append(filesystems, modcands[1:3]...)
	}

	// Since boot partitions can be either fat or ext, return a
	// readWriteCloser dependent upon the underlying filesystem type.
	var sdimgFile sdimgFile
	for _, fs := range filesystems {
		fstype, err := imgFilesystemType(fs.path)
		if err != nil {
			return nil, errors.Wrap(err, "partition: error reading file-system type on partition")
		}
		var f VPFile
		switch fstype {
		case fat:
			f, err = newFatFile(fs.path, fpath)
		case ext:
			f, err = newExtFile(fs.path, fpath)
		case unsupported:
			return nil, errors.New("partition: unsupported filesystem")

		}
		if err != nil {
			return nil, err
		}
		sdimgFile = append(sdimgFile, f)
	}
	return sdimgFile, nil
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
	if len(p) == 0 {
		return 0, errors.New("No partition set to read from")
	}
	return p[0].Read(b)
}

func (p sdimgFile) CopyTo(hostFile string) error {
	for _, part := range p {
		err := part.CopyTo(hostFile)
		if err != nil {
			return err
		}
	}
	return nil
}

func (p sdimgFile) CopyFrom(hostFile string) error {
	if len(p) == 0 {
		return errors.New("No partition set to copy from")
	}
	return p[0].CopyFrom(hostFile)
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

func newArtifactExtFile(image *ModImageArtifact, comp artifact.Compressor, fpath, imgpath string) (VPFile, error) {
	reg := regexp.MustCompile("/(uboot|boot/(efi|grub))")
	if reg.MatchString(fpath) {
		return nil, errors.New("newArtifactExtFile: A mender artifact does not contain a boot partition, only a rootfs")
	}
	if strings.HasPrefix(fpath, "/data") {
		return nil, errors.New("newArtifactExtFile: A mender artifact does not contain a data partition, only a rootfs")
	}

	return newExtFile(imgpath, fpath)
}

// extFile wraps partition and implements ReadWriteCloser
type extFile struct {
	imagePath     string
	imageFilePath string
	flush         bool     // True if Close() needs to copy the file to the image
	tmpf          *os.File // Used as a buffer for multiple write operations
}

func newExtFile(imagePath, imageFilePath string) (e *extFile, err error) {
	if err := runFsck(imagePath, "ext4"); err != nil {
		return nil, err
	}

	// Check that the given directory exists.
	_, err = debugfsExecuteCommand(fmt.Sprintf("cd %s", filepath.Dir(imageFilePath)), imagePath)
	if err != nil {
		return nil, fmt.Errorf("The directory: %s does not exist in the image", filepath.Dir(imageFilePath))
	}
	tmpf, err := ioutil.TempFile("", "mendertmp-extfile")
	// Cleanup resources in case of error.
	e = &extFile{
		imagePath:     imagePath,
		imageFilePath: imageFilePath,
		tmpf:          tmpf,
	}
	return e, err
}

// Write reads all bytes from b into the partitionFile using debugfs.
func (ef *extFile) Write(b []byte) (int, error) {
	n, err := ef.tmpf.Write(b)
	ef.flush = true
	return n, err
}

// Read reads all bytes from the filepath on the partition image into b
func (ef *extFile) Read(b []byte) (int, error) {
	str, err := debugfsCopyFile(ef.imageFilePath, ef.imagePath)
	defer os.RemoveAll(str) // ignore error removing tmp-dir
	if err != nil {
		return 0, errors.Wrap(err, "extFile: ReadError: debugfsCopyFile failed")
	}
	data, err := ioutil.ReadFile(filepath.Join(str, filepath.Base(ef.imageFilePath)))
	if err != nil {
		return 0, errors.Wrapf(err, "extFile: ReadError: ioutil.Readfile failed to read file: %s", filepath.Join(str, filepath.Base(ef.imageFilePath)))
	}
	return copy(b, data), io.EOF
}

func (ef *extFile) CopyTo(hostFile string) error {
	if err := debugfsReplaceFile(ef.imageFilePath, hostFile, ef.imagePath); err != nil {
		return err
	}
	return nil
}

func (ef *extFile) CopyFrom(hostFile string) error {
	// Get the file permissions
	d, err := debugfsExecuteCommand(fmt.Sprintf("stat %s", ef.imageFilePath), ef.imagePath)
	if err != nil {
		if strings.Contains(err.Error(), "File not found by ext2_lookup") {
			return fmt.Errorf("The file: %s does not exist in the image", ef.imageFilePath)
		}
		return err
	}
	// Extract the Mode: oooo octal code
	reg := regexp.MustCompile(`Mode: +(0[0-9]{3})`)
	m := reg.FindStringSubmatch(d.String())
	if m == nil || len(m) != 2 {
		return fmt.Errorf("Could not extract the filemode information from the file: %s\n", ef.imageFilePath)
	}
	mode, err := strconv.ParseInt(m[1], 8, 32)
	if err != nil {
		return fmt.Errorf("Failed to extract the file permissions for the file: %s\nerr: %s", ef.imageFilePath, err)
	}
	_, err = debugfsExecuteCommand(fmt.Sprintf("dump %s %s\nclose", ef.imageFilePath, hostFile), ef.imagePath)
	if err != nil {
		if strings.Contains(err.Error(), "File not found by ext2_lookup") {
			return fmt.Errorf("The file: %s does not exist in the image", ef.imageFilePath)
		}
		return err
	}
	if err = os.Chmod(hostFile, os.FileMode(mode)); err != nil {
		return err
	}
	return nil
}

func (ef *extFile) Delete(recursive bool) (err error) {
	err = debugfsRemoveFileOrDir(ef.imageFilePath, ef.imagePath, recursive)
	if err != nil {
		return err
	}
	return nil
}

// Close closes the temporary file held by partitionFile path.
func (ef *extFile) Close() (err error) {
	if ef == nil {
		return nil
	}
	if ef.tmpf != nil {
		if ef.flush {
			err = debugfsReplaceFile(ef.imageFilePath, ef.tmpf.Name(), ef.imagePath)
			if err != nil {
				return err
			}
		}
		// Ignore tmp-errors
		ef.tmpf.Close()
		os.Remove(ef.tmpf.Name())
	}
	return err
}

// fatFile wraps a partition struct with a reader/writer for fat filesystems
type fatFile struct {
	imagePath     string
	imageFilePath string // The local filesystem path to the image
	flush         bool
	tmpf          *os.File
}

func newFatFile(imagePath, imageFilePath string) (*fatFile, error) {
	if err := runFsck(imagePath, "vfat"); err != nil {
		return nil, err
	}

	tmpf, err := ioutil.TempFile("", "mendertmp-fatfile")
	ff := &fatFile{
		imagePath:     imagePath,
		imageFilePath: imageFilePath,
		tmpf:          tmpf,
	}
	return ff, err
}

// Read Dump the file contents to stdout, and capture, using MTools' mtype
func (f *fatFile) Read(b []byte) (n int, err error) {
	cmd := exec.Command("mtype", "-n", "-i", f.imagePath, "::"+f.imageFilePath)
	dbuf := bytes.NewBuffer(nil)
	cmd.Stdout = dbuf // capture Stdout
	if err = cmd.Run(); err != nil {
		return 0, errors.Wrap(err, "fatPartitionFile: Read: MTools mtype dump failed")
	}
	return copy(b, dbuf.Bytes()), io.EOF
}

// Write Writes to the underlying fat image, using MTools' mcopy
func (f *fatFile) Write(b []byte) (n int, err error) {
	n, err = f.tmpf.Write(b)
	f.flush = true
	return len(b), nil
}

func (f *fatFile) CopyTo(hostFile string) error {
	cmd := exec.Command("mcopy", "-oi", f.imagePath, hostFile, "::"+f.imageFilePath)
	data := bytes.NewBuffer(nil)
	cmd.Stdout = data
	if err := cmd.Run(); err != nil {
		return errors.Wrap(err, "fatFile: Write: MTools execution failed")
	}
	return nil
}

func (f *fatFile) CopyFrom(hostFile string) error {
	cmd := exec.Command("mcopy", "-n", "-i", f.imagePath, "::"+f.imageFilePath, hostFile)
	dbuf := bytes.NewBuffer(nil)
	cmd.Stdout = dbuf // capture Stdout
	if err := cmd.Run(); err != nil {
		return errors.Wrap(err, "fatPartitionFile: Read: MTools mcopy failed")
	}
	return nil
}

func (f *fatFile) Delete(recursive bool) (err error) {
	isDir := filepath.Dir(f.imageFilePath) == strings.TrimRight(f.imageFilePath, "/")
	var deleteCmd string
	if isDir {
		deleteCmd = "mdeltree"
	} else {
		deleteCmd = "mdel"
	}
	cmd := exec.Command(deleteCmd, "-i", f.imagePath, "::"+f.imageFilePath)
	if err = cmd.Run(); err != nil {
		return errors.Wrap(err, "fatFile: Delete: execution failed: "+deleteCmd)
	}
	return nil
}

func (f *fatFile) Close() (err error) {
	if f == nil {
		return nil
	}
	if f.tmpf != nil {
		if f.flush {
			cmd := exec.Command("mcopy", "-n", "-i", f.imagePath, f.tmpf.Name(), "::"+f.imageFilePath)
			data := bytes.NewBuffer(nil)
			cmd.Stdout = data
			if err = cmd.Run(); err != nil {
				return errors.Wrap(err, "fatFile: Write: MTools execution failed")
			}
		}
		f.tmpf.Close()
		os.Remove(f.tmpf.Name())
	}
	return err
}

func processSdimg(image string) (VPImage, error) {
	bin, err := utils.GetBinaryPath("parted")
	if err != nil {
		return nil, fmt.Errorf("`parted` binary not found on the system")
	}
	out, err := exec.Command(bin, image, "unit s", "print").Output()
	if err != nil {
		return nil, errors.Wrap(err, "can not execute `parted` command or image is broken; "+
			"make sure parted is available in your system and is in the $PATH")
	}


	reg := regexp.MustCompile(`(?m)^[[:blank:]][0-9]+[[:blank:]]+([0-9]+)s[[:blank:]]+[0-9]+s[[:blank:]]+([0-9]+)s`)
	partitionMatch := reg.FindAllStringSubmatch(string(out), -1)

	if len(partitionMatch) == 4 {
		partitions := make([]partition, 0)
		// we will have three groups per each entry in the partition table
		for i := 0; i < 4; i++ {
			single := partitionMatch[i]
			partitions = append(partitions, partition{offset: single[1], size: single[2]})
		}
		if partitions, err = extractFromSdimg(partitions, image); err != nil {
			return nil, err
		}
		return &ModImageSdimg{
			ModImageBase: ModImageBase{
				path: image,
			},
			candidates: partitions,
		}, nil
		// if we have single ext file there is no need to mount it

	} else if len(partitionMatch) == 1 {
		return &ModImageRaw{
			ModImageBase: ModImageBase{
				path: image,
			},
		}, nil
	}
	return nil, fmt.Errorf("invalid partition table: %s", string(out))
}

func extractFromSdimg(partitions []partition, image string) ([]partition, error) {
	for i, part := range partitions {
		tmp, err := ioutil.TempFile("", "mender-modify-image")
		if err != nil {
			return nil, errors.Wrap(err, "can not create temp file for storing image")
		}
		if err = tmp.Close(); err != nil {
			return nil, errors.Wrapf(err, "can not close temporary file: %s", tmp.Name())
		}
		cmd := exec.Command("dd", "if="+image, "of="+tmp.Name(),
			"skip="+part.offset, "count="+part.size)
		if err = cmd.Run(); err != nil {
			return nil, errors.Wrap(err, "can not extract image from sdimg")
		}
		partitions[i].path = tmp.Name()
	}
	return partitions, nil
}

func repackSdimg(partitions []partition, image string) error {
	for _, part := range partitions {
		if err := exec.Command("dd", "if="+part.path, "of="+image,
			"seek="+part.offset, "count="+part.size,
			"conv=notrunc").Run(); err != nil {
			return errors.Wrap(err, "can not copy image back to sdimg")
		}
	}
	return nil
}
