// Copyright 2016 Mender Software AS
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

func writeArtifact(c *cli.Context) error {
	if len(c.StringSlice("device-type")) == 0 || len(c.String("artifact-name")) == 0 ||
		len(c.String("update")) == 0 {
		return errors.New("must provide `device-type`, `artifact-name` and `update`")
	}

	name := "mender.tar.gz"
	if len(c.String("output-path")) > 0 {
		name = c.String("output-path")
	}
	devices := c.StringSlice("device-type")

	f, err := os.Create(name)
	if err != nil {
		return errors.New("can not create artifact file")
	}
	defer f.Close()

	if len(c.String("sign")) != 0 {

		privateKey, err := getKey(c.String("sign"))
		if err != nil {
			return err
		}
		s := &artifact.DummySigner{
			PrivKey: privateKey,
		}
		u := handlers.NewRootfsV2(c.String("update"))
		upd := &artifact.Updates{
			U: []artifact.Composer{u},
		}
		aw := awriter.NewWriterSigned(f, s)
		// default version for signed artifact is 2
		ver := 2
		if c.IsSet("version") {
			ver = c.Int("version")
		}
		return aw.WriteArtifact("mender", ver,
			devices, c.String("artifact-name"), upd)
	} else {
		u := handlers.NewRootfsV1(c.String("update"))
		upd := &artifact.Updates{
			U: []artifact.Composer{u},
		}
		aw := awriter.NewWriter(f)
		return aw.WriteArtifact("mender", c.Int("version"),
			devices, c.String("artifact-name"), upd)
	}
}

func read(aPath string, verify func(message, sig []byte) error) (*areader.Reader, error) {
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

	if err = ar.ReadArtifact(); err != nil {
		return nil, err
	}

	return ar, nil
}

func readArtifact(c *cli.Context) error {
	if c.NArg() == 0 {
		return errors.New("Nothing specified, nothing read. \nMaybe you wanted" +
			" to say 'artifacts read <pathspec>'?")
	}

	var verifyCallback func(message, sig []byte) error

	if len(c.String("verify")) != 0 {
		key, err := getKey(c.String("verify"))
		if err != nil {
			return err
		}
		s := artifact.DummySigner{
			PubKey: key,
		}
		verifyCallback = s.Verify
	}

	r, err := read(c.Args().First(), verifyCallback)
	if err != nil {
		return err
	}

	inst := r.GetInstallers()
	info := r.GetInfo()

	fmt.Printf("Mender artifact:\n")
	fmt.Printf("  Name: %s\n", r.GetArtifactName())
	fmt.Printf("  Format: %s\n", info.Format)
	fmt.Printf("  Version: %d\n", info.Version)
	fmt.Printf("  Compatible devices: '%s'\n", r.GetCompatibleDevices())

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
		return nil, errors.New("Invialid key path.")
	}
	defer f.Close()

	key := bytes.NewBuffer(nil)
	if _, err := io.Copy(key, f); err != nil {
		return nil, errors.New("Error reading key.")
	}
	return key.Bytes(), nil
}

func validateArtifact(c *cli.Context) error {
	if c.NArg() == 0 {
		return errors.New("Nothing specified, nothing validated. \nMaybe you wanted" +
			" to say 'artifacts validate <pathspec>'?")
	}

	var verifyCallback func(message, sig []byte) error

	if len(c.String("verify")) != 0 {
		key, err := getKey(c.String("verify"))
		if err != nil {
			return err
		}
		s := artifact.DummySigner{
			PubKey: key,
		}
		verifyCallback = s.Verify
	}

	_, err := read(c.Args().First(), verifyCallback)
	if err != nil {
		return err
	}

	fmt.Println("Artifact file '" + c.Args().First() + "' validated successfully")
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
			Value: 1,
		},
		cli.StringFlag{
			Name:  "sign, s",
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
		Name: "verify, v",
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
