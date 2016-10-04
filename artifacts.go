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
	"errors"
	"fmt"
	"io/ioutil"
	"os"

	"github.com/mendersoftware/artifacts/parser"
	"github.com/mendersoftware/artifacts/reader"
	"github.com/mendersoftware/artifacts/writer"

	"github.com/urfave/cli"
)

func writeArtifact(c *cli.Context) error {
	if len(c.String("device-type")) == 0 || len(c.String("image-id")) == 0 ||
		len(c.String("update")) == 0 {
		return errors.New("must provide `device-type`, `image-id` and `update`")
	}

	he := &parser.HeaderElems{
		Metadata: []byte(`{"deviceType": "` + c.String("device-type") +
			`", "imageId": "` + c.String("image-id") + `"}`),
	}

	ud := parser.UpdateData{
		P:         &parser.RootfsParser{},
		DataFiles: []string{c.String("update")},
		Type:      "rootfs-image",
		Data:      he,
	}

	name := "mender.tar.gz"
	if len(c.String("output-path")) > 0 {
		name = c.String("output-path")
	}

	aw := awriter.NewWriter("mender", 1)
	return aw.WriteKnown([]parser.UpdateData{ud}, name)
}

func read(aPath string) (parser.Workers, error) {
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
	defer ar.Close()

	p := parser.RootfsParser{
		W: ioutil.Discard, // don't store update anywhere
	}
	ar.Register(&p)

	return ar.Read()
}

func readArtifact(c *cli.Context) error {
	if c.NArg() == 0 {
		return errors.New("Nothing specified, nothing validated. \nMaybe you wanted" +
			"to say 'artifacts validate <pathspec>'?")
	}
	parsers, err := read(c.Args().First())
	if err != nil {
		return err
	}

	fmt.Printf("Read Mender artifact file containing %d updates\n", len(parsers))

	for k, p := range parsers {
		fmt.Printf("details of update: %s\n", k)
		fmt.Printf("  update type: '%s'\n", p.GetUpdateType().Type)
		fmt.Printf("  supported device: '%s'\n", p.GetDeviceType())
		if ri, ok := p.(*parser.RootfsParser); ok {
			fmt.Printf("  image id: '%s'\n", ri.GetImageID())
		}

		for _, f := range p.GetUpdateFiles() {
			fmt.Printf("  update files: name: '%s', size: %d bytes, modified: %s\n",
				f.Name, f.Size, f.Date)
		}
	}
	return nil
}

func validateArtifact(c *cli.Context) error {
	if c.NArg() == 0 {
		return errors.New("Nothing specified, nothing validated. \nMaybe you wanted" +
			"to say 'artifacts validate <pathspec>'?")
	}

	_, err := read(c.Args().First())
	if err != nil {
		return err
	}

	fmt.Println("Artifact file '" + c.Args().First() + "' validated successfully")
	return nil
}

func run() error {
	app := cli.NewApp()
	app.Name = "artifact"
	app.Usage = "Mender artifact read/writer"
	app.UsageText = "artifacts [--version][--help] <command> [<args>]"
	app.Version = "0.1"

	app.Author = "mender.io"
	app.Email = "contact@mender.io"

	writeRootfs := cli.Command{
		Name:   "rootfs-image",
		Action: writeArtifact,
	}

	writeRootfs.Flags = []cli.Flag{
		cli.StringFlag{
			Name:  "update, u",
			Usage: "Update `FILE`",
		},
		cli.StringFlag{
			Name:  "device-type, t",
			Usage: "Type of device supported by the update",
		},
		cli.StringFlag{
			Name:  "image-id, i",
			Usage: "Yocto id of the update image",
		},
		cli.StringFlag{
			Name:  "output-path, o",
			Usage: "Full path to output artifact file",
		},
	}

	validate := cli.Command{
		Name:        "validate",
		Usage:       "Validates artifact file",
		Action:      validateArtifact,
		UsageText:   "atrifacts validate [options] <pathspec>",
		Description: "This command validates artifact file provided by pathspec.",
	}

	app.Commands = []cli.Command{
		{
			Name:  "write",
			Usage: "Writes artifact file",
			Subcommands: []cli.Command{
				writeRootfs,
			},
		},
		{
			Name:  "read",
			Usage: "Reads artifact file",
			Subcommands: []cli.Command{
				cli.Command{
					Name:   "artifact",
					Action: readArtifact,
				},
				cli.Command{
					Name:   "type",
					Action: readArtifact,
				},
			},
		},
		validate,
	}
	return app.Run(os.Args)
}

func main() {
	run()
}
