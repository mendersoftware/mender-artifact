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
		return errors.New("invalid arguments")
	}

	he := &parser.HeaderElems{
		Metadata: []byte(`{"deviceType": "` + c.String("device-type") + `", "imageId": "` + c.String("image-id") + `"}`),
	}

	ud := parser.UpdateData{
		P:         &parser.RootfsParser{},
		DataFiles: []string{c.String("update")},
		Type:      "rootfs-image",
		Data:      he,
	}

	aw := awriter.NewWriter("mender", 1)
	return aw.WriteKnown([]parser.UpdateData{ud}, "mender.tar.gz")
}

func readArtifact(c *cli.Context) error {
	return nil
}

func main() {

	app := cli.NewApp()
	//app. = "Mender artifact read/writer"
	//app.Copyright =
	//app.Usage = "asdffasdfafsd"
	//app.UsageText = "asdfa asdf "
	app.Version = "0.1"

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
	}

	app.Commands = []cli.Command{
		{
			Name:     "write",
			Usage:    "Writes artifact file",
			Category: "write",
			Subcommands: []cli.Command{
				writeRootfs,
			},
		},
		{
			Name:     "read",
			Action:   readArtifact,
			Usage:    "Reads artifact file",
			Category: "read",
		},
	}

	app.Run(os.Args)
}
