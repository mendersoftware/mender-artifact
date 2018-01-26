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

func getKey(keyPath string) ([]byte, error) {
	if keyPath == "" {
		return nil, nil
	}

	key, err := ioutil.ReadFile(keyPath)
	if err != nil {
		return nil, errors.Wrap(err, "Error reading key file")
	}
	return key, nil
}

type artifactError struct {
	err          error
	badSignature bool
}

func (ae *artifactError) Error() string {
	return ae.err.Error()
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
