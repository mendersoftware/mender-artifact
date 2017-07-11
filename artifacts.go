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
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/mendersoftware/mender-artifact/areader"
	"github.com/mendersoftware/mender-artifact/artifact"
	"github.com/mendersoftware/mender-artifact/awriter"
	"github.com/mendersoftware/mender-artifact/handlers"

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
	for _, script := range c.StringSlice("script") {
		if err := scr.Add(script); err != nil {
			return nil, err
		}
	}
	return &scr, nil
}

func writeArtifact(c *cli.Context) error {
	if len(c.StringSlice("device-type")) == 0 ||
		len(c.String("artifact-name")) == 0 ||
		len(c.String("update")) == 0 {
		return cli.NewExitError("must provide `device-type`, `artifact-name` and `update`", 1)
	}

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

	// do not return error if we can not validate signature;
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
		fmt.Println("Artifact file '" + c.Args().First() +
			"' formatted correctly, but error validating signature.")
	} else {
		fmt.Println("Artifact file '" + c.Args().First() + "' validated successfully")
	}
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
		Name:   "rootfs-image",
		Action: writeArtifact,
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
	}

	write := cli.Command{
		Name:  "write",
		Usage: "Writes artifact file.",
		Subcommands: []cli.Command{
			writeRootfs,
		},
	}

	unsign := cli.StringFlag{
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
		unsign,
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
		unsign,
	}

	app.Commands = []cli.Command{
		write,
		read,
		validate,
	}
	return app.Run(os.Args)
}

func main() {
	run()
}
