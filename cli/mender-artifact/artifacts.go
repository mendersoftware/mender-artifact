// Copyright 2017 Northern.tech AS
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
	"encoding/json"
	"encoding/pem"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"

	"github.com/mendersoftware/mender-artifact/areader"
	"github.com/mendersoftware/mender-artifact/artifact"
	"github.com/mendersoftware/mender-artifact/awriter"
	"github.com/mendersoftware/mender-artifact/handlers"

	"github.com/pkg/errors"
	"github.com/urfave/cli"
)

// Version of the mender-artifact CLI tool
var Version = "unknown"

// LatestFormatVersion is the latest version of the format, which is
// also what we default to.
const LatestFormatVersion = 2

func version(c *cli.Context) int {
	version := c.Int("version")
	return version
}

func artifactWriter(f *os.File, c *cli.Context,
	ver int) (*awriter.Writer, error) {
	if len(c.String("key")) != 0 {
		if ver == 1 {
			// check if we are having correct version
			return nil, errors.New("can not use signed artifact with version 1")
		}
		privateKey, err := getKey(c.String("key"))
		if err != nil {
			return nil, err
		}
		return awriter.NewWriterSigned(f, artifact.NewSigner(privateKey)), nil
	}
	return awriter.NewWriter(f), nil
}

func scripts(scripts []string) (*artifact.Scripts, error) {
	scr := artifact.Scripts{}
	for _, scriptArg := range scripts {
		statInfo, err := os.Stat(scriptArg)
		if err != nil {
			return nil, errors.Wrapf(err, "can not stat script file: %s", scriptArg)
		}

		// Read either a directory, or add the script file directly.
		if statInfo.IsDir() {
			fileList, err := ioutil.ReadDir(scriptArg)
			if err != nil {
				return nil, errors.Wrapf(err, "can not list directory contents of: %s", scriptArg)
			}
			for _, nameInfo := range fileList {
				if err := scr.Add(filepath.Join(scriptArg, nameInfo.Name())); err != nil {
					return nil, err
				}
			}
		} else {
			if err := scr.Add(scriptArg); err != nil {
				return nil, err
			}
		}
	}
	return &scr, nil
}

func writeArtifact(c *cli.Context) error {
	// set default name
	name := "artifact.mender"
	if len(c.String("output-path")) > 0 {
		name = c.String("output-path")
	}
	version := version(c)

	var h *handlers.Rootfs
	switch version {
	case 1:
		h = handlers.NewRootfsV1(c.String("update"))
	case 2:
		h = handlers.NewRootfsV2(c.String("update"))
	default:
		return cli.NewExitError("unsupported artifact version", 1)
	}

	upd := &awriter.Updates{
		U: []handlers.Composer{h},
	}

	f, err := os.Create(name + ".tmp")
	if err != nil {
		return cli.NewExitError("can not create artifact file", 1)
	}
	defer func() {
		f.Close()
		// in case of success `.tmp` suffix will be removed and below
		// will not remove valid artifact
		os.Remove(name + ".tmp")
	}()

	aw, err := artifactWriter(f, c, version)
	if err != nil {
		return cli.NewExitError(err.Error(), 1)
	}

	scr, err := scripts(c.StringSlice("script"))
	if err != nil {
		return cli.NewExitError(err.Error(), 1)
	} else if len(scr.Get()) != 0 && version == 1 {
		// check if we are having correct version
		return cli.NewExitError("can not use scripts artifact with version 1", 1)
	}

	err = aw.WriteArtifact("mender", version,
		c.StringSlice("device-type"), c.String("artifact-name"), upd, scr)
	if err != nil {
		return cli.NewExitError(err.Error(), 1)
	}

	f.Close()
	err = os.Rename(name+".tmp", name)
	if err != nil {
		return cli.NewExitError(err.Error(), 1)
	}
	return nil
}

func read(ar *areader.Reader, verify areader.SignatureVerifyFn,
	readScripts areader.ScriptsReadFn) (*areader.Reader, error) {

	if ar == nil {
		return nil, errors.New("can not read artifact file")
	}

	if verify != nil {
		ar.VerifySignatureCallback = verify
	}
	if readScripts != nil {
		ar.ScriptsReadCallback = readScripts
	}

	if err := ar.ReadArtifact(); err != nil {
		return nil, err
	}

	return ar, nil
}

func readArtifact(c *cli.Context) error {
	if c.NArg() == 0 {
		return cli.NewExitError("Nothing specified, nothing read. \nMaybe you wanted"+
			" to say 'artifacts read <pathspec>'?", 1)
	}

	f, err := os.Open(c.Args().First())
	if err != nil {
		return cli.NewExitError("Can not open '"+c.Args().First()+"' file.", 1)
	}
	defer f.Close()

	var verifyCallback areader.SignatureVerifyFn

	if len(c.String("key")) != 0 {
		key, keyErr := getKey(c.String("key"))
		if keyErr != nil {
			return cli.NewExitError(keyErr.Error(), 1)
		}
		s := artifact.NewVerifier(key)
		verifyCallback = s.Verify
	}

	// if key is not provided just continue reading artifact returning
	// info that signature can not be verified
	sigInfo := "no signature"
	ver := func(message, sig []byte) error {
		sigInfo = "signed but no key for verification provided; " +
			"please use `-k` option for providing verification key"
		if verifyCallback != nil {
			err = verifyCallback(message, sig)
			if err != nil {
				sigInfo = "signed; verification using provided key failed"
			} else {
				sigInfo = "signed and verified correctly"
			}
		}
		return nil
	}

	var scripts []string
	readScripts := func(r io.Reader, info os.FileInfo) error {
		scripts = append(scripts, info.Name())
		return nil
	}

	ar := areader.NewReader(f)
	r, err := read(ar, ver, readScripts)
	if err != nil {
		return cli.NewExitError(err.Error(), 1)
	}

	inst := r.GetHandlers()
	info := r.GetInfo()

	fmt.Printf("Mender artifact:\n")
	fmt.Printf("  Name: %s\n", r.GetArtifactName())
	fmt.Printf("  Format: %s\n", info.Format)
	fmt.Printf("  Version: %d\n", info.Version)
	fmt.Printf("  Signature: %s\n", sigInfo)
	fmt.Printf("  Compatible devices: '%s'\n", r.GetCompatibleDevices())
	if len(scripts) > 0 {
		fmt.Printf("  State scripts:\n")
	}
	for _, scr := range scripts {
		fmt.Printf("    %s\n", scr)
	}
	fmt.Printf("\nUpdates:\n")

	for k, p := range inst {
		fmt.Printf("  %04d:\n", k)
		fmt.Printf("    Type:   %s\n", p.GetType())
		for _, f := range p.GetUpdateFiles() {
			fmt.Printf("    Files:\n")
			fmt.Printf("      name:     %s\n", f.Name)
			fmt.Printf("      size:     %d\n", f.Size)
			fmt.Printf("      modified: %s\n", f.Date)
			fmt.Printf("      checksum: %s\n", f.Checksum)
		}
	}
	return nil
}

func getKey(path string) ([]byte, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("Invalid key path: %s", path)
	}
	defer f.Close()

	key := bytes.NewBuffer(nil)
	if _, err := io.Copy(key, f); err != nil {
		return nil, fmt.Errorf("Error reading key: %s", path)
	}
	return key.Bytes(), nil
}

type artifactError struct {
	err          error
	badSignature bool
}

func (ae *artifactError) Error() string {
	return ae.err.Error()
}

func checkIfValid(artifactPath string, key []byte) *artifactError {
	verifyCallback := func(message, sig []byte) error {
		return errors.New("artifact is signed but no verification key was provided")
	}

	if key != nil {
		s := artifact.NewVerifier(key)
		verifyCallback = s.Verify
	}

	// do not return error immediately if we can not validate signature;
	// just continue checking consistency and return info if
	// signature verification failed
	var validationError error
	ver := func(message, sig []byte) error {
		if verifyCallback != nil {
			if err := verifyCallback(message, sig); err != nil {
				validationError = err
			}
		}
		return nil
	}

	f, err := os.Open(artifactPath)
	if err != nil {
		return &artifactError{err: err}
	}
	defer f.Close()

	ar := areader.NewReader(f)
	_, err = read(ar, ver, nil)
	if err != nil {
		return &artifactError{err: err}
	}

	if validationError != nil {
		return &artifactError{
			err: fmt.Errorf("artifact file '%s' formatted correctly, "+
				"but error validating signature: %s", artifactPath, validationError),
			badSignature: true,
		}
	}
	return nil
}

func validateArtifact(c *cli.Context) error {
	if c.NArg() == 0 {
		return cli.NewExitError("Nothing specified, nothing validated. \nMaybe you wanted"+
			" to say 'artifacts validate <pathspec>'?", 1)
	}

	var key []byte
	var err error
	if c.String("key") != "" {
		key, err = getKey(c.String("key"))
		if err != nil {
			return cli.NewExitError("Can not read key: "+err.Error(), 1)
		}
	}

	if err := checkIfValid(c.Args().First(), key); err != nil {
		return cli.NewExitError(err.Error(), 1)
	}

	fmt.Printf("Artifact file '%s' validated successfully\n", c.Args().First())
	return nil
}

func signExisting(c *cli.Context) error {
	if c.NArg() == 0 {
		return cli.NewExitError("Nothing specified, nothing signed. \nMaybe you wanted"+
			" to say 'artifacts sign <pathspec>'?", 1)
	}

	if len(c.String("key")) == 0 {
		return cli.NewExitError("Missing signing key; "+
			"please use `-k` parameter for providing one", 1)
	}

	privateKey, err := getKey(c.String("key"))
	if err != nil {
		return cli.NewExitError("Can not use signing key provided: "+err.Error(), 1)
	}

	f, err := os.Open(c.Args().First())
	if err != nil {
		return errors.Wrapf(err, "Can not open: %s", c.Args().First())
	}
	defer f.Close()

	tFile, err := ioutil.TempFile(filepath.Dir(c.Args().First()), "mender-artifact")
	if err != nil {
		return errors.Wrap(err,
			"Can not create temporary file for storing artifact")
	}
	defer os.Remove(tFile.Name())
	defer tFile.Close()

	reader, err := repack(c.Args().First(), f, tFile, privateKey, "", "")
	if err != nil {
		return err
	}

	switch ver := reader.GetInfo().Version; ver {
	case 1:
		return cli.NewExitError("Can not sign v1 artifact", 1)
	case 2:
		if reader.IsSigned && !c.Bool("force") {
			return cli.NewExitError("Trying to sign already signed artifact; "+
				"please use force option", 1)
		}
	default:
		return cli.NewExitError("Unsupported version of artifact file: "+string(ver), 1)
	}

	if err = tFile.Close(); err != nil {
		return err
	}

	name := c.Args().First()
	if len(c.String("output-path")) > 0 {
		name = c.String("output-path")
	}

	err = os.Rename(tFile.Name(), name)
	if err != nil {
		return cli.NewExitError("Can not store signed artifact: "+err.Error(), 1)
	}
	return nil
}

func unpackArtifact(name string) (string, error) {
	f, err := os.Open(name)
	if err != nil {
		return "", errors.Wrapf(err, "Can not open: %s", name)
	}
	defer f.Close()

	// initialize raw reader and writer
	aReader := areader.NewReader(f)
	rootfs := handlers.NewRootfsInstaller()

	tmp, err := ioutil.TempFile(filepath.Dir(name), "mender-artifact")
	if err != nil {
		return "", err
	}
	defer tmp.Close()

	rootfs.InstallHandler = func(r io.Reader, df *handlers.DataFile) error {
		_, err = io.Copy(tmp, r)
		return err
	}

	if err = aReader.RegisterHandler(rootfs); err != nil {
		return "", errors.Wrap(err, "failed to register install handler")
	}

	err = aReader.ReadArtifact()
	if err != nil {
		return "", err
	}
	return tmp.Name(), nil
}

func repack(artifactName string, from io.Reader, to io.Writer, key []byte,
	newName string, dataFile string) (*areader.Reader, error) {
	sDir, err := ioutil.TempDir(filepath.Dir(artifactName), "mender-repack")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(sDir)

	storeScripts := func(r io.Reader, info os.FileInfo) error {
		sLocation := filepath.Join(sDir, info.Name())
		f, fileErr := os.OpenFile(sLocation, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0755)
		if fileErr != nil {
			return errors.Wrapf(fileErr,
				"can not create script file: %v", sLocation)
		}
		defer f.Close()

		_, err = io.Copy(f, r)
		if err != nil {
			return errors.Wrapf(err,
				"can not write script file: %v", sLocation)
		}
		f.Sync()
		return nil
	}

	verify := func(message, sig []byte) error {
		return nil
	}

	data := dataFile
	ar := areader.NewReader(from)

	if dataFile == "" {
		tmpData, tmpErr := ioutil.TempFile("", "mender-repack")
		if tmpErr != nil {
			return nil, tmpErr
		}
		defer os.Remove(tmpData.Name())
		defer tmpData.Close()

		rootfs := handlers.NewRootfsInstaller()
		rootfs.InstallHandler = func(r io.Reader, df *handlers.DataFile) error {
			_, err = io.Copy(tmpData, r)
			return err
		}

		data = tmpData.Name()
		ar.RegisterHandler(rootfs)
	}

	r, err := read(ar, verify, storeScripts)
	if err != nil {
		return nil, err
	}

	info := r.GetInfo()

	// now once arifact is read we need to
	var h *handlers.Rootfs
	switch info.Version {
	case 1:
		h = handlers.NewRootfsV1(data)
	case 2:
		h = handlers.NewRootfsV2(data)
	default:
		return nil, errors.Errorf("unsupported artifact version: %d", info.Version)
	}

	upd := &awriter.Updates{
		U: []handlers.Composer{h},
	}
	scr, err := scripts([]string{sDir})
	if err != nil {
		return nil, err
	}

	aWriter := awriter.NewWriter(to)
	if key != nil {
		aWriter = awriter.NewWriterSigned(to, artifact.NewSigner(key))
	}

	name := ar.GetArtifactName()
	if newName != "" {
		name = newName
	}
	err = aWriter.WriteArtifact(info.Format, info.Version,
		ar.GetCompatibleDevices(), name, upd, scr)

	return ar, err
}

// oblivious to whether the file exists beforehand
func modifyName(name, image string) error {
	data := fmt.Sprintf("artifact_name=%s", name)
	tmpNameFile, err := ioutil.TempFile("", "mender-name")
	if err != nil {
		return err
	}
	defer os.Remove(tmpNameFile.Name())
	defer tmpNameFile.Close()

	if _, err = tmpNameFile.WriteString(data); err != nil {
		return err
	}

	if err = tmpNameFile.Close(); err != nil {
		return err
	}

	return debugfsReplaceFile("/etc/mender/artifact_info",
		tmpNameFile.Name(), image)
}

func modifyServerCert(newCert, image string) error {
	_, err := os.Stat(newCert)
	if err != nil {
		return errors.Wrap(err, "invalid server certificate")
	}
	return debugfsReplaceFile("/etc/mender/server.crt", newCert, image)
}

func modifyVerificationKey(newKey, image string) error {
	_, err := os.Stat(newKey)
	if err != nil {
		return errors.Wrapf(err, "invalid verification key")
	}
	return debugfsReplaceFile("/etc/mender/artifact-verify-key.pem", newKey, image)
}

func modifyMenderConfVar(confKey, confValue, image string) error {
	confFile := "/etc/mender/mender.conf"
	dir, err := debugfsCopyFile(confFile, image)
	if err != nil {
		return err
	}
	defer os.RemoveAll(dir)

	raw, err := ioutil.ReadFile(filepath.Join(dir, filepath.Base(confFile)))
	if err != nil {
		return err
	}

	var rawData interface{}
	if err = json.Unmarshal(raw, &rawData); err != nil {
		return err
	}
	rawData.(map[string]interface{})[confKey] = confValue

	data, err := json.Marshal(&rawData)
	if err != nil {
		return err
	}

	if err = ioutil.WriteFile(filepath.Join(dir, filepath.Base(confFile)), data, 0755); err != nil {
		return err
	}

	return debugfsReplaceFile(confFile, filepath.Join(dir,
		filepath.Base(confFile)), image)
}

func repackArtifact(artifact, rootfs, key, newName string) error {
	art, err := os.Open(artifact)
	if err != nil {
		return err
	}
	defer art.Close()

	tmp, err := ioutil.TempFile(filepath.Dir(artifact), "mender-artifact")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	defer tmp.Close()

	var privateKey []byte
	if key != "" {
		privateKey, err = getKey(key)
		if err != nil {

			return cli.NewExitError(fmt.Sprintf("Can not use signing key provided: %s", err.Error()), 1)
		}
	}

	if _, err = repack(artifact, art, tmp, privateKey, newName, rootfs); err != nil {
		return err
	}

	return os.Rename(tmp.Name(), artifact)
}

func modifyExisting(c *cli.Context, image string) error {
	if c.String("name") != "" {
		if err := modifyName(c.String("name"), image); err != nil {
			return err
		}
	}

	if c.String("server-uri") != "" {
		if err := modifyMenderConfVar("ServerURL",
			c.String("server-uri"), image); err != nil {
			return err
		}
	}

	if c.String("server-cert") != "" {
		if err := modifyServerCert(c.String("server-cert"), image); err != nil {
			return err
		}
	}

	if c.String("verification-key") != "" {
		if err := modifyVerificationKey(c.String("verification-key"), image); err != nil {
			return err
		}
	}

	if c.String("tenant-token") != "" {
		if err := modifyMenderConfVar("TenantToken",
			c.String("tenant-token"), image); err != nil {
			return err
		}
	}

	return nil
}

type partition struct {
	offset string
	size   string
	path   string
}

func processSdimg(image string) ([]partition, error) {
	out, err := exec.Command("parted", image, "unit s", "print").Output()
	if err != nil {
		return nil, errors.Wrap(err, "can not execute `parted` command or image is broken; "+
			"make sure parted is available in your system and is in the $PATH")
	}

	partitions := make([]partition, 0)

	reg := regexp.MustCompile(`(?m)^[[:blank:]][0-9]+[[:blank:]]+([0-9]+)s[[:blank:]]+[0-9]+s[[:blank:]]+([0-9]+)s`)
	partitionMatch := reg.FindAllStringSubmatch(string(out), -1)

	// IMPORTANT: we are assuming standard Mender formating here:
	// only 2nd and 3rd partitions are rootfs we are going to modify
	if len(partitionMatch) == 4 {
		// we will have three groups per each entry in the partition table
		for i := 1; i < 3; i++ {
			single := partitionMatch[i]
			partitions = append(partitions, partition{offset: single[1], size: single[2]})
		}
		if err = extractFromSdimg(partitions, image); err != nil {
			return nil, err
		}
		return partitions, nil
		// if we have single ext file there is no need to mount it
	} else if len(partitionMatch) == 1 {
		return []partition{{path: image}}, nil
	}
	return nil, fmt.Errorf("invalid partition table: %s", string(out))
}

func extractFromSdimg(partitions []partition, image string) error {
	for i, part := range partitions {
		tmp, err := ioutil.TempFile("", "mender-modify-image")
		if err != nil {
			return errors.Wrap(err, "can not create temp file for storing image")
		}
		if err = tmp.Close(); err != nil {
			return errors.Wrapf(err, "can not close temporary file: %s", tmp.Name())
		}
		cmd := exec.Command("dd", "if="+image, "of="+tmp.Name(),
			"skip="+part.offset, "count="+part.size)
		if err = cmd.Run(); err != nil {
			return errors.Wrap(err, "can not extract image from sdimg")
		}
		partitions[i].path = tmp.Name()
	}
	return nil
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

func processModifyKey(keyPath string) ([]byte, error) {
	// extract public key from it private counterpart
	if keyPath != "" {
		priv, err := getKey(keyPath)
		if err != nil {
			return nil, errors.Wrap(err, "can not get private key")
		}
		pubKeyRaw, err := artifact.GetPublic(priv)
		if err != nil {
			return nil, errors.Wrap(err, "can not get private key public counterpart")
		}

		buf := &bytes.Buffer{}
		err = pem.Encode(buf, &pem.Block{
			Type:  "PUBLIC KEY",
			Bytes: pubKeyRaw,
		})
		if err != nil {
			return nil, errors.Wrap(err, "can not encode public key")
		}
		return buf.Bytes(), nil
	}
	return nil, nil
}

func getCandidatesForModify(path string, key []byte) ([]partition, bool, error) {
	isArtifact := false
	modifyCandidates := make([]partition, 0)

	// first we need to check  if we are having artifact or image file
	if err := checkIfValid(path, key); err == nil {
		// we have VALID artifact, so we need to unpack it and store header
		isArtifact = true
		rawImage, err := unpackArtifact(path)
		if err != nil {
			return nil, isArtifact, errors.Wrap(err, "can not process artifact")
		}
		modifyCandidates = append(modifyCandidates, partition{path: rawImage})
	} else if err.badSignature {
		return nil, isArtifact, err
	} else {
		parts, err := processSdimg(path)
		if err != nil {
			return nil, isArtifact, errors.Wrap(err, "can not process image file")
		}
		modifyCandidates = append(modifyCandidates, parts...)
	}
	return modifyCandidates, isArtifact, nil
}

func modifyArtifact(c *cli.Context) error {
	if c.NArg() == 0 {
		return cli.NewExitError("Nothing specified, nothing will be modified. \n"+
			"Maybe you wanted to say 'artifacts read <pathspec>'?", 1)
	}

	if _, err := os.Stat(c.Args().First()); err != nil && os.IsNotExist(err) {
		return cli.NewExitError("File ["+c.Args().First()+"] does not exist.", 1)
	}

	pubKey, err := processModifyKey(c.String("key"))
	if err != nil {
		return cli.NewExitError("Error processing private key: "+err.Error(), 1)
	}

	modifyCandidates, isArtifact, err :=
		getCandidatesForModify(c.Args().First(), pubKey)

	if err != nil {
		return cli.NewExitError("Error selecting images for modification: "+err.Error(), 1)
	}

	if len(modifyCandidates) > 1 || isArtifact {
		for _, mc := range modifyCandidates {
			defer os.Remove(mc.path)
		}
	}

	for _, toModify := range modifyCandidates {
		if err := modifyExisting(c, toModify.path); err != nil {
			return cli.NewExitError("Error modifying artifact["+toModify.path+"]: "+
				err.Error(), 1)
		}
	}

	if len(modifyCandidates) > 1 {
		// make modified images part of sdimg again
		if err := repackSdimg(modifyCandidates, c.Args().First()); err != nil {
			return cli.NewExitError("Can not recreate sdimg file: "+err.Error(), 1)
		}
		return nil
	}

	if isArtifact {
		// re-create the artifact
		err := repackArtifact(c.Args().First(), modifyCandidates[0].path,
			c.String("key"), c.String("name"))
		if err != nil {
			return cli.NewExitError("Can not recreate artifact: "+err.Error(), 1)
		}
	}
	return nil
}
