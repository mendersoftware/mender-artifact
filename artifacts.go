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
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/godbus/dbus"
	"github.com/mendersoftware/mender-artifact/areader"
	"github.com/mendersoftware/mender-artifact/artifact"
	"github.com/mendersoftware/mender-artifact/awriter"
	"github.com/mendersoftware/mender-artifact/handlers"

	"github.com/pkg/errors"
	"github.com/urfave/cli"
)

// Version of the mender-artifact CLI tool
var Version = "unknown"

// Latest version of the format, which is also what we default to.
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

func scripts(c *cli.Context) (*artifact.Scripts, error) {
	scr := artifact.Scripts{}
	for _, scriptArg := range c.StringSlice("script") {
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

	scr, err := scripts(c)
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

func read(aPath string, verify areader.SignatureVerifyFn,
	readScripts areader.ScriptsReadFn) (*areader.Reader, error) {
	_, err := os.Stat(aPath)
	if err != nil {
		return nil, errors.New("Pathspec '" + aPath +
			"' does not match any files.")
	}

	f, err := os.Open(aPath)
	if err != nil {
		return nil, errors.New("Can not open '" + aPath + "' file.")
	}
	defer f.Close()

	ar := areader.NewReader(f)
	if ar == nil {
		return nil, errors.New("Can not read artifact file.")
	}

	if verify != nil {
		ar.VerifySignatureCallback = verify
	}
	if readScripts != nil {
		ar.ScriptsReadCallback = readScripts
	}

	if err = ar.ReadArtifact(); err != nil {
		return nil, err
	}

	return ar, nil
}

func readArtifact(c *cli.Context) error {
	if c.NArg() == 0 {
		return cli.NewExitError("Nothing specified, nothing read. \nMaybe you wanted"+
			" to say 'artifacts read <pathspec>'?", 1)
	}

	var verifyCallback areader.SignatureVerifyFn

	if len(c.String("key")) != 0 {
		key, err := getKey(c.String("key"))
		if err != nil {
			return cli.NewExitError(err.Error(), 1)
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
			err := verifyCallback(message, sig)
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

	r, err := read(c.Args().First(), ver, readScripts)
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

func validateArtifact(c *cli.Context) error {
	if c.NArg() == 0 {
		return cli.NewExitError("Nothing specified, nothing validated. \nMaybe you wanted"+
			" to say 'artifacts validate <pathspec>'?", 1)
	}

	var verifyCallback areader.SignatureVerifyFn

	if len(c.String("key")) != 0 {
		key, err := getKey(c.String("key"))
		if err != nil {
			return cli.NewExitError(err.Error(), 1)
		}
		s := artifact.NewVerifier(key)
		verifyCallback = s.Verify
	}

	// do not return error immediately if we can not validate signature;
	// just continue checking consistency and return info if
	// signature verification failed
	valid := true
	ver := func(message, sig []byte) error {
		if verifyCallback != nil {
			if err := verifyCallback(message, sig); err != nil {
				valid = false
			}
		}
		return nil
	}

	_, err := read(c.Args().First(), ver, nil)
	if err != nil {
		return cli.NewExitError(err.Error(), 1)
	}

	if !valid {
		return cli.NewExitError("Artifact file '"+c.Args().First()+
			"' formatted correctly, but error validating signature.", 1)
	} else {
		fmt.Println("Artifact file '" + c.Args().First() + "' validated successfully")
	}
	return nil
}

func createSignature(raw *artifact.Raw, w *awriter.Writer, key []byte) error {
	s := artifact.NewSigner(key)
	buf := bytes.NewBuffer(nil)
	_, err := io.Copy(buf, raw.Data)
	if err != nil {
		return errors.Wrap(err, "Can not copy manifest data for signing")
	}
	signed, sErr := s.Sign(buf.Bytes())
	if sErr != nil {
		return sErr
	}

	// first write orifinal "manifest" file
	if err :=
		w.WriteRaw(artifact.NewRaw("manifest", raw.Size, buf)); err != nil {
		return err
	}
	// then, write "manifest.sig"
	if err :=
		w.WriteRaw(
			artifact.NewRaw("manifest.sig",
				int64(len(signed)), bytes.NewBuffer(signed))); err != nil {
		return err
	}
	return nil
}

func processHeader(r *areader.Reader, w *awriter.Writer,
	key []byte, force bool) error {
	// simple list with the header element name and the flag uses as
	// indicator if element is optional or required
	artifactHeaderElems := []struct {
		name     string
		required bool
	}{
		{"manifest", true},
		{"manifest.sig", false},
		{"header.", true},
	}

	getNext := true
	var raw *artifact.Raw = nil
	var err error = nil

	// read header elements first
	for _, elem := range artifactHeaderElems {
		// get element form the artifact
		if getNext {
			raw, err = r.ReadRaw()
			if err != nil {
				return err
			}
		}
		getNext = true

		// check if we are not having element out of order
		if elem.required && !strings.HasPrefix(raw.Name, elem.name) {
			return errors.Errorf("Invalid artifact, should contain '%s' "+
				"file , but contains '%s'", elem.name, raw.Name)
		} else if !elem.required && !strings.HasPrefix(raw.Name, elem.name) {
			// we have missing optional element; move on to the next one
			getNext = false
			continue
		}

		// check if we are having "manifest" file, which we need to sign
		if raw.Name == "manifest" {
			if err = createSignature(raw, w, key); err != nil {
				return err
			}
			continue

		} else if raw.Name == "manifest.sig" && !force {
			// we are re-signing the artifact; return error by default
			return errors.New("Trying to sign already signed artifact; " +
				"please use force option")
		} else if raw.Name == "manifest.sig" && force {
			// just continue here as new signature is already part of tmp artifact
			continue
		}

		if err = w.WriteRaw(raw); err != nil {
			return errors.Wrap(err, "Can not write artifact")
		}
	}
	return nil
}

func signArtifact(r *areader.Reader, w *awriter.Writer,
	rawVer *artifact.Raw, key []byte, force bool) error {
	// first we need to store version in new artifact we are trying to sign
	if err := w.WriteRaw(rawVer); err != nil {
		return err
	}

	if err := processHeader(r, w, key, force); err != nil {
		return err
	}

	// read the rest of the artifact
	for {
		raw, err := r.ReadRaw()
		if err != nil && errors.Cause(err) == io.EOF {
			break
		} else if err != nil {
			return err
		}
		if err = w.WriteRaw(raw); err != nil {
			return err
		}
	}
	return nil
}

func writeTemp(aName string, key []byte, force bool) (string, error) {

	f, err := os.Open(aName)
	if err != nil {
		return "", errors.Wrapf(err, "Can not open: %s", aName)
	}
	defer f.Close()

	// initialize raw reader and writer
	aReader := areader.NewReader(f)
	ver, data, err := aReader.ReadRawVersion()
	if err != nil {
		return "", err
	}

	// we are supporting only v2 signing for now
	switch ver {
	case 1:
		return "", errors.New("Can not sign v1 artifact")
	case 2:
		tFile, err := ioutil.TempFile("", "mender-artifact")
		if err != nil {
			return "", errors.Wrap(err,
				"Can not create temporary file for storing artifact")
		}
		aWriter := awriter.NewWriterRaw(tFile)
		defer aWriter.CloseRaw()

		err = signArtifact(aReader, aWriter, data, key, force)
		if err != nil {
			os.Remove(tFile.Name())
			return "", err
		}

		tFile.Close()
		return tFile.Name(), nil
	default:
		return "", errors.New("Unsupported version of artifact file: " + string(ver))
	}
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

	tmp, err := writeTemp(c.Args().First(), privateKey, c.Bool("force"))
	if err != nil {
		return cli.NewExitError("Can not read/write artifact: "+err.Error(), 1)
	}

	name := c.Args().First()
	if len(c.String("output-path")) > 0 {
		name = c.String("output-path")
	}

	err = os.Rename(tmp, name)
	if err != nil {
		os.Remove(tmp)
		return cli.NewExitError("Can not store signed artifact: "+err.Error(), 1)
	}
	return nil
}

func getDevices(msg *dbus.Signal) error {
	if strings.HasSuffix(msg.Name, "InterfacesAdded") {
		for _, v := range msg.Body {
			if data, ok := v.(dbus.ObjectPath); ok {
				if strings.Contains(string(data), "block_device") {
					// TODO:
					fmt.Printf("dev: %v\n", string(data))
				}
			}
		}
	}
	return nil
}

func modifyArtifact(c *cli.Context) error {
	if c.NArg() == 0 {
		return cli.NewExitError("Nothing specified, nothing will be modified. \n"+
			"Maybe you wanted to say 'artifacts read <pathspec>'?", 1)
	}

	// start connection with dbus
	conn, err := dbus.SystemBus()
	if err != nil {
		return cli.NewExitError("Failed to connect to dbus: "+err.Error(), 1)
	}
	defer conn.Close()

	if !conn.SupportsUnixFDs() {
		return cli.NewExitError("Connection does not support unix fsd; exiting", 1)
	}

	file, err := os.OpenFile(c.Args().First(), os.O_RDWR, 0)
	if err != nil {
		return cli.NewExitError("Can not open file: "+err.Error(), 1)
	}
	defer file.Close()

	var wg sync.WaitGroup
	wg.Add(1)

	ch := make(chan *dbus.Signal)

	if err = registerDeviceCallback(conn, ch, &wg, getDevices); err != nil {
		return cli.NewExitError("Can not register dbus callback: "+err.Error(), 1)
	}

	// wait for the callback to be completed
	defer func() {
		close(ch)
		wg.Wait()
	}()

	loopDevice, err := mountFile(conn, dbus.UnixFD(file.Fd()))
	if err != nil {
		return cli.NewExitError("Can not loop mount file: "+err.Error(), 1)
	}
	// TODO:
	fmt.Printf("Loop device mounted: %s\n", loopDevice)

	prop, err := getDeviceProperties(conn, loopDevice,
		[]string{"IdUUID", "ReadOnly", "MountPoints"})
	if err != nil {
		return cli.NewExitError("Can not get device properties: "+err.Error(), 1)
	}
	// TODO
	fmt.Printf("Received device properties: %v\n", prop)

	return nil
}

func run() error {
	app := cli.NewApp()
	app.Name = "mender-artifact"
	app.Usage = "Mender artifact read/writer"
	app.UsageText = "mender-artifact [--version][--help] <command> [<args>]"
	app.Version = Version

	app.Author = "mender.io"
	app.Email = "contact@mender.io"

	//
	// write
	//
	writeRootfs := cli.Command{
		Name: "rootfs-image",
		Action: func(c *cli.Context) error {
			if len(c.StringSlice("device-type")) == 0 ||
				len(c.String("artifact-name")) == 0 ||
				len(c.String("update")) == 0 {
				return cli.NewExitError("must provide `device-type`, `artifact-name` and `update`", 1)
			}
			if len(strings.Fields(c.String("artifact-name"))) > 1 { // check for whitespace in artifact-name
				return cli.NewExitError("whitespace is not allowed in the artifact-name", 1)
			}
			return writeArtifact(c)
		},
	}

	writeRootfs.Flags = []cli.Flag{
		cli.StringFlag{
			Name:  "update, u",
			Usage: "Update `FILE`.",
		},
		cli.StringSliceFlag{
			Name: "device-type, t",
			Usage: "Type of device(s) supported by the update. You can specify multiple " +
				"compatible devices providing this parameter multiple times.",
		},
		cli.StringFlag{
			Name:  "artifact-name, n",
			Usage: "Name of the artifact",
		},
		cli.StringFlag{
			Name:  "output-path, o",
			Usage: "Full path to output artifact file.",
		},
		cli.IntFlag{
			Name:  "version, v",
			Usage: "Version of the artifact.",
			Value: LatestFormatVersion,
		},
		cli.StringFlag{
			Name:  "key, k",
			Usage: "Full path to the private key that will be used to sign the artifact.",
		},
		cli.StringSliceFlag{
			Name: "script, s",
			Usage: "Full path to the state script(s). You can specify multiple " +
				"scripts providing this parameter multiple times.",
		},
	}

	write := cli.Command{
		Name:  "write",
		Usage: "Writes artifact file.",
		Subcommands: []cli.Command{
			writeRootfs,
		},
	}

	key := cli.StringFlag{
		Name: "key, k",
		Usage: "Full path to the public key that will be used to verify " +
			"the artifact signature.",
	}

	//
	// validate
	//
	validate := cli.Command{
		Name:        "validate",
		Usage:       "Validates artifact file.",
		Action:      validateArtifact,
		UsageText:   "mender-artifact validate [options] <pathspec>",
		Description: "This command validates artifact file provided by pathspec.",
	}
	validate.Flags = []cli.Flag{
		key,
	}

	//
	// read
	//
	read := cli.Command{
		Name:        "read",
		Usage:       "Reads artifact file.",
		Action:      readArtifact,
		UsageText:   "mender-artifact read [options] <pathspec>",
		Description: "This command validates artifact file provided by pathspec.",
	}

	read.Flags = []cli.Flag{
		key,
	}

	//
	// sign
	//
	sign := cli.Command{
		Name:        "sign",
		Usage:       "Signs existing artifact file.",
		Action:      signExisting,
		UsageText:   "mender-artifact sign [options] <pathspec>",
		Description: "This command signs artifact file provided by pathspec.",
	}
	sign.Flags = []cli.Flag{
		key,
		cli.StringFlag{
			Name: "output-path, o",
			Usage: "Full path to output signed artifact file; " +
				"if none is provided existing artifact will be replaced with signed one",
		},
		cli.BoolFlag{
			Name:  "force, f",
			Usage: "Force creating new signature if the artifact is already signed",
		},
	}

	//
	// modify existing
	//
	modify := cli.Command{
		Name:        "modify",
		Usage:       "Modifies image or artifact file.",
		Action:      modifyArtifact,
		UsageText:   "mender-artifact modify [options] <pathspec>",
		Description: "This command modifies existing image or artifact file provided by pathspec.",
	}

	app.Commands = []cli.Command{
		write,
		read,
		validate,
		sign,
		modify,
	}
	return app.Run(os.Args)
}

func main() {
	if err := run(); err != nil {
		os.Exit(1)
	}
}
