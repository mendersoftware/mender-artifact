// Copyright 2023 Northern.tech AS
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

package cli

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

	"github.com/pkg/errors"

	"github.com/mendersoftware/mender-artifact/areader"
	"github.com/mendersoftware/mender-artifact/artifact"
	"github.com/mendersoftware/mender-artifact/utils"
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
var errBlkidNotFound = errors.New("`blkid` binary not found on the system")

type VPImage interface {
	io.Closer
	Open(fpath string) (VPFile, error)
	OpenDir(fpath string) (VPDir, error)
	dirtyImage()
}

// V(irtual)P(artition)File mimicks a file in an Artifact or on an sdimg.
type VPFile interface {
	io.ReadWriteCloser
	Delete(recursive bool) error
	CopyTo(hostFile string) error
	CopyFrom(hostFile string) error
}

// VPDir V(irtual)P(artition)Dir mimics a directory in an Artifact or on an sdimg.
type VPDir interface {
	io.Closer
	Create() error
}

type partition struct {
	offset string
	size   string
	path   string
}

type ModImageBase struct {
	path  string
	dirty bool
}

type ModImageArtifact struct {
	ModImageBase
	*unpackedArtifact
	comp artifact.Compressor
	key  SigningKey
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

type vImageAndDir struct {
	image VPImage
	dir   VPDir
}

// Open is a utility function that parses an input image and returns a
// V(irtual)P(artition)Image.
func (v vImage) Open(
	key SigningKey,
	imgname string,
	overrideCompressor ...artifact.Compressor,
) (VPImage, error) {
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
			return nil, errors.New(
				"Modifying artifacts with more than one payload is not supported",
			)
		}

		unpackedArtifact, err := unpackArtifact(imgname)
		if err != nil {
			return nil, errors.Wrap(err, "can not process artifact")
		}

		var comp artifact.Compressor
		if len(overrideCompressor) == 1 {
			comp = overrideCompressor[0]
		} else {
			comp = unpackedArtifact.ar.Compressor()
		}

		return &ModImageArtifact{
			ModImageBase: ModImageBase{
				path:  imgname,
				dirty: false,
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
func (v vImage) OpenFile(key SigningKey, imgAndPath string) (VPFile, error) {
	imagepath, filepath, err := parseImgPath(imgAndPath)
	if err != nil {
		return nil, err
	}

	image, err := v.Open(key, imagepath)
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

// Shortcut to use an image with one directory.
func (v vImage) OpenDir(key SigningKey, imgAndPath string) (VPDir, error) {
	imagepath, dirpath, err := parseImgPath(imgAndPath)
	if err != nil {
		return nil, err
	}

	image, err := v.Open(key, imagepath)
	if err != nil {
		return nil, err
	}

	dir, err := image.OpenDir(dirpath)
	if err != nil {
		image.Close()
		return nil, err
	}

	return &vImageAndDir{
		image: image,
		dir:   dir,
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
	v.image.dirtyImage()
	return v.file.Write(buf)
}

func (v *vImageAndFile) Delete(recursive bool) error {
	v.image.dirtyImage()
	return v.file.Delete(recursive)
}

func (v *vImageAndFile) CopyTo(hostFile string) error {
	v.image.dirtyImage()
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

func (v *vImageAndDir) Create() error {
	v.image.dirtyImage()
	return v.dir.Create()
}

func (v *vImageAndDir) Close() error {
	dirErr := v.dir.Close()
	imageErr := v.image.Close()

	if dirErr != nil {
		return dirErr
	} else {
		return imageErr
	}
}

// Opens a file inside the image(s) represented by the ModImageArtifact
func (i *ModImageArtifact) Open(fpath string) (VPFile, error) {
	return newArtifactExtFile(i, i.comp, fpath, i.files[0])
}

// Opens a dir inside the image(s) represented by the ModImageArtifact
func (i *ModImageArtifact) OpenDir(fpath string) (VPDir, error) {
	return newArtifactExtDir(i, i.comp, fpath, i.files[0])
}

// Closes and repacks the artifact or sdimg.
func (i *ModImageArtifact) Close() error {
	if i.unpackDir != "" {
		defer os.RemoveAll(i.unpackDir)
	}
	if i.dirty {
		return repackArtifact(i.comp, i.key, i.unpackedArtifact)
	}
	return nil
}

func (i *ModImageArtifact) dirtyImage() {
	i.dirty = true
}

// Opens a file inside the image(s) represented by the ModImageSdimg
func (i *ModImageSdimg) Open(fpath string) (VPFile, error) {
	return newSDImgFile(i, fpath, i.candidates)
}

func (i *ModImageSdimg) OpenDir(fpath string) (VPDir, error) {
	return newSDImgDir(i, fpath, i.candidates)
}

func (i *ModImageSdimg) Close() error {
	for _, cand := range i.candidates {
		if cand.path != "" && cand.path != i.path {
			defer os.RemoveAll(cand.path)
		}
	}
	if i.dirty {
		return repackSdimg(i.candidates, i.path)
	}
	return nil
}

func (i *ModImageSdimg) dirtyImage() {
	i.dirty = true
}

func (i *ModImageRaw) Open(fpath string) (VPFile, error) {
	return newExtFile(i.path, fpath)
}

func (i *ModImageRaw) OpenDir(fpath string) (VPDir, error) {
	return newExtDir(i.path, fpath)
}

func (i *ModImageRaw) Close() error {
	return nil
}

func (i *ModImageRaw) dirtyImage() {
	i.dirty = true
}

// parseImgPath parses cli input of the form
// path/to/[sdimg,mender]:/path/inside/img/file
// into path/to/[sdimg,mender] and path/inside/img/file
func parseImgPath(imgpath string) (imgname, fpath string, err error) {
	paths := strings.SplitN(imgpath, ":", 2)
	if len(paths) != 2 {
		return "", "", fmt.Errorf("failed to parse image path %q", imgpath)
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
		return unsupported, errBlkidNotFound
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
//	0      No errors
//	1      Filesystem errors corrected
//	2      System should be rebooted
//	4      Filesystem errors left uncorrected
//	8      Operational error
//	16     Usage or syntax error
//	32     Checking canceled by user request
//	128    Shared-library error
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

// sdimgDir is a virtual directory for files on an sdimg.
type sdimgDir []VPDir

func isSparsePartition(part partition) bool {
	// NOTE: Basically just checking for a filesystem
	_, err := debugfsExecuteCommand("stat /", part.path)
	return err != nil
}

// filterSparsePartitions returns partitions with data from an array of partitions
func filterSparsePartitions(parts []partition) []partition {
	ps := []partition{}
	for _, part := range parts {
		if isSparsePartition(part) {
			continue
		}
		ps = append(ps, part)
	}
	return ps
}

// getFilesystems extracts only the partitions we want to modify.
// for {data,/[u]boot} this is one partition.
// for rootfs{a,b}, this is the two partitions (unless one of them is unpopulated,
// then only the one with data is returned)
func getFilesystems(fpath string, modcands []partition) ([]partition, string) {

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
		filesystems = append(filesystems, filterSparsePartitions(modcands[1:3])...)
	}

	return filesystems, fpath
}

func newSDImgFile(image *ModImageSdimg, fpath string, modcands []partition) (sdimgFile, error) {
	if len(modcands) < 4 {
		return nil, fmt.Errorf("newSDImgFile: %d partitions found, 4 needed", len(modcands))
	}

	filesystems, pfpath := getFilesystems(fpath, modcands)

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
			f, err = newFatFile(fs.path, pfpath)
		case ext:
			f, err = newExtFile(fs.path, pfpath)
		case unsupported:
			err = errors.New("partition: unsupported filesystem")

		}
		if err != nil {
			sdimgFile.Close()
			return nil, err
		}
		sdimgFile = append(sdimgFile, f)
	}
	return sdimgFile, nil
}

func newSDImgDir(image *ModImageSdimg, fpath string, modcands []partition) (sdimgDir, error) {
	if len(modcands) < 4 {
		return nil, fmt.Errorf("newSDImgDir: %d partitions found, 4 needed", len(modcands))
	}

	filesystems, pfpath := getFilesystems(fpath, modcands)

	// Since boot partitions can be either fat or ext, return a
	// Closer dependent upon the underlying filesystem type.
	var sdimgDir sdimgDir
	for _, fs := range filesystems {
		fstype, err := imgFilesystemType(fs.path)
		if err != nil {
			return nil, errors.Wrap(err, "partition: error reading file-system type on partition")
		}
		var d VPDir
		switch fstype {
		case fat:
			d, err = newFatDir(fs.path, pfpath)
		case ext:
			d, err = newExtDir(fs.path, pfpath)
		case unsupported:
			err = errors.New("partition: unsupported filesystem")

		}
		if err != nil {
			sdimgDir.Close()
			return nil, err
		}
		sdimgDir = append(sdimgDir, d)
	}
	return sdimgDir, nil
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

func (p sdimgDir) Create() (err error) {
	for _, part := range p {
		err := part.Create()
		if err != nil {
			return err
		}
	}
	return nil
}

// Close closes the underlying closers.
func (p sdimgDir) Close() (err error) {
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

func newArtifactExtFile(
	image *ModImageArtifact,
	comp artifact.Compressor,
	fpath,
	imgpath string,
) (VPFile, error) {
	reg := regexp.MustCompile("/(uboot|boot/(efi|grub))")
	if reg.MatchString(fpath) {
		return nil, errors.New(
			"newArtifactExtFile: A mender artifact does not contain a boot partition," +
				" only a rootfs",
		)
	}
	if strings.HasPrefix(fpath, "/data") {
		return nil, errors.New(
			"newArtifactExtFile: A mender artifact does not contain a data partition," +
				" only a rootfs",
		)
	}

	return newExtFile(imgpath, fpath)
}

func newArtifactExtDir(
	image *ModImageArtifact,
	comp artifact.Compressor,
	fpath string,
	imgpath string,
) (VPDir, error) {
	reg := regexp.MustCompile("/(uboot|boot/(efi|grub))")
	if reg.MatchString(fpath) {
		return nil, errors.New(
			"newArtifactExtDir: A mender artifact does not contain a boot partition, only a rootfs",
		)
	}
	if strings.HasPrefix(fpath, "/data") {
		return nil, errors.New(
			"newArtifactExtDir: A mender artifact does not contain a data partition, only a rootfs",
		)
	}

	return newExtDir(imgpath, fpath)
}

// extFile wraps partition and implements ReadWriteCloser
type extFile struct {
	imagePath     string
	imageFilePath string
	flush         bool     // True if Close() needs to copy the file to the image
	tmpf          *os.File // Used as a buffer for multiple write operations
}

// extDir wraps partition
type extDir struct {
	imagePath     string
	imageFilePath string
}

func newExtFile(imagePath, imageFilePath string) (e *extFile, err error) {
	if err := runFsck(imagePath, "ext4"); err != nil {
		return nil, err
	}

	// Check that the given directory exists.
	_, err = debugfsExecuteCommand(fmt.Sprintf("cd %s", filepath.Dir(imageFilePath)), imagePath)
	if err != nil {
		return nil, fmt.Errorf(
			"The directory: %s does not exist in the image", filepath.Dir(imageFilePath),
		)
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

func newExtDir(imagePath, imageFilePath string) (e *extDir, err error) {
	if err := runFsck(imagePath, "ext4"); err != nil {
		return nil, err
	}

	// Cleanup resources in case of error.
	e = &extDir{
		imagePath:     imagePath,
		imageFilePath: imageFilePath,
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
		return 0, errors.Wrapf(
			err,
			"extFile: ReadError: ioutil.Readfile failed to read file: %s",
			filepath.Join(str, filepath.Base(ef.imageFilePath)),
		)
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
		return fmt.Errorf(
			"Could not extract the filemode information from the file: %s\n",
			ef.imageFilePath,
		)
	}
	mode, err := strconv.ParseInt(m[1], 8, 32)
	if err != nil {
		return fmt.Errorf(
			"Failed to extract the file permissions for the file: %s\nerr: %s",
			ef.imageFilePath,
			err,
		)
	}
	_, err = debugfsExecuteCommand(
		fmt.Sprintf("dump %s %s\nclose", ef.imageFilePath, hostFile),
		ef.imagePath,
	)
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
		defer func() {
			// Ignore tmp-errors
			ef.tmpf.Close()
			os.Remove(ef.tmpf.Name())
		}()
		if ef.flush {
			err = debugfsReplaceFile(ef.imageFilePath, ef.tmpf.Name(), ef.imagePath)
			if err != nil {
				return err
			}
		}
	}
	return err
}

func (ed *extDir) Create() error {
	err := debugfsMakeDir(ed.imageFilePath, ed.imagePath)
	return err
}

// Close closes the temporary file held by partitionFile path.
func (ed *extDir) Close() (err error) {
	if ed == nil {
		return nil
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

type fatDir struct {
	imagePath     string
	imageFilePath string
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

func newFatDir(imagePath, imageFilePath string) (fd *fatDir, err error) {
	if err := runFsck(imagePath, "vfat"); err != nil {
		return nil, err
	}

	fd = &fatDir{
		imagePath:     imagePath,
		imageFilePath: imageFilePath,
	}
	return fd, err
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
	if err != nil {
		return n, err
	}
	f.flush = true
	return n, nil
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
		defer func() {
			f.tmpf.Close()
			os.Remove(f.tmpf.Name())
		}()
		if f.flush {
			cmd := exec.Command(
				"mcopy",
				"-n",
				"-i",
				f.imagePath,
				f.tmpf.Name(),
				"::"+f.imageFilePath,
			)
			data := bytes.NewBuffer(nil)
			cmd.Stdout = data
			if err = cmd.Run(); err != nil {
				return errors.Wrap(err, "fatFile: Write: MTools execution failed")
			}
		}
	}
	return err
}

func (fd *fatDir) Create() (err error) {
	cmd := exec.Command("mmd", fd.imageFilePath)
	data := bytes.NewBuffer(nil)
	cmd.Stdout = data
	if err := cmd.Run(); err != nil {
		return errors.Wrap(err, "fatDir: Create: MTools execution failed")
	}
	return err
}

func (fd *fatDir) Close() (err error) {
	if fd == nil {
		return nil
	}
	os.Remove(fd.imagePath)
	return err
}

func processSdimg(image string) (VPImage, error) {
	bin, err := utils.GetBinaryPath("parted")
	if err != nil {
		return nil, errors.Wrap(err, "`parted` binary not found on the system")
	}
	out, err := exec.Command(bin, image, "unit s", "print").Output()
	if err != nil {
		return nil, errors.Wrap(err, "can not execute `parted` command or image is broken; "+
			"make sure parted is available in your system and is in the $PATH")
	}

	reg := regexp.MustCompile(
		`(?m)^[[:blank:]][0-9]+[[:blank:]]+([0-9]+)s[[:blank:]]+[0-9]+s[[:blank:]]+([0-9]+)s`,
	)
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
				path:  image,
				dirty: false,
			},
			candidates: partitions,
		}, nil
		// if we have single ext file there is no need to mount it

	} else if len(partitionMatch) == 1 && partitionMatch[0][1] == "0" {
		// For one partition match which has an offset of zero, we
		// assume it is a raw filesystem image.
		return &ModImageRaw{
			ModImageBase: ModImageBase{
				path:  image,
				dirty: false,
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
