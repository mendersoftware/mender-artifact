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
	"os"

	"github.com/mendersoftware/artifacts/parser"
	"github.com/mendersoftware/artifacts/writer"

	"github.com/urfave/cli"
)

func wrieArtifact(c *cli.Context) error {
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
	if len(c.String("name")) > 0 {
		name = c.String("name")
	}

	aw := awriter.NewWriter("mender", 1)
	return aw.WriteKnown([]parser.UpdateData{ud}, name)
}

func readArtifact(c *cli.Context) error {
	return nil
}

func main() {
	app := cli.NewApp()
	app.Name = "artifact"
	app.Usage = "Mender artifact read/writer"
	app.UsageText = "artifacts [--version][--help] <command> [<args>]"
	app.Version = "0.1"

	app.Author = "mender.io"
	app.Email = "contact@mender.io"

	writeRootfs := cli.Command{
		Name:   "rootfs-image",
		Action: wrieArtifact,
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
			Name:  "name, n",
			Usage: "Name of the artifact file",
		},
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
	}

	app.Run(os.Args)
}
