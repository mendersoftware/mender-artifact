// Copyright 2018 Northern.tech AS
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
	"os"

	"github.com/urfave/cli"
)

const (
	artifactOK = iota
	errArtifactInvalidParameters
	errArtifactUnsupportedVersion
	errArtifactCreate
	errArtifactOpen
	errArtifactInvalid
)

// Version of the mender-artifact CLI tool
var Version = "unknown"

// LatestFormatVersion is the latest version of the format, which is
// also what we default to.
const LatestFormatVersion = 3

func main() {
	if err := run(); err != nil {
		os.Exit(1)
	}
}

func run() error {
	app := cli.NewApp()
	app.Name = "mender-artifact"
	app.Usage = "interface for manipulating Mender artifacts"
	app.UsageText = "mender-artifact [--version][--help] <command> [<args>]"
	app.Version = Version

	app.Author = "mender.io"
	app.Email = "contact@mender.io"

	//
	// write
	//
	writeRootfsCommand := cli.Command{
		Name:      "rootfs-image",
		Action:    writeRootfs,
		Usage:     "Writes Mender artifact containing rootfs image",
		ArgsUsage: "<image path>",
	}

	writeRootfsCommand.Flags = []cli.Flag{
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
		/////////////////////////
		// Version 3 specifics.//
		/////////////////////////
		// Hiding these for now. We are not enforcing them on the client
		// so we should not set these yet.
		//
		// cli.StringSliceFlag{
		// 	Name:  "artifact-name-depends, N",
		// 	Usage: "Sets the name(s) of the artifact(s) which this update depends upon",
		// },
		// cli.StringFlag{
		// 	Name:  "provides-group, g",
		// 	Usage: "The group the artifact provides",
		// },
		// cli.StringSliceFlag{
		// 	Name:  "depends-groups, G",
		// 	Usage: "The group(s) the artifact depends on",
		// },
		// cli.StringFlag{
		// 	Name:  "depends-rootfs-image-checksum",
		// 	Usage: "The checksum of the rootfs image which this artifact depends upon",
		// },
		// cli.StringFlag{
		// 	Name:  "provides-rootfs-image-checksum",
		// 	Usage: "The checksum of the rootfs image which this artifact provides",
		// },
	}

	//
	// Update modules: module-image
	//
	writeModuleCommand := cli.Command{
		Name:   "module-image",
		Action: writeModuleImage,
		Usage:  "Writes Mender artifact for an update module",
		UsageText: "Writes a generic Mender artifact that will be used by an update module. " +
			"This command is not meant to be used directly, but should rather be wrapped by an " +
			"update module build command, which prepares all the necessary files and headers " +
			"for that update module.",
	}

	writeModuleCommand.Flags = []cli.Flag{
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
		cli.StringSliceFlag{
			Name:  "artifact-name-depends, N",
			Usage: "Sets the name(s) of the artifact(s) which this update depends upon",
		},
		cli.StringFlag{
			Name:  "provides-group, g",
			Usage: "The group the artifact provides",
		},
		cli.StringSliceFlag{
			Name:  "depends-groups, G",
			Usage: "The group(s) the artifact depends on",
		},
		cli.StringFlag{
			Name:  "type, T",
			Usage: "Type of update. This is the same as the name of the update module",
		},
		cli.StringSliceFlag{
			Name:  "provides, p",
			Usage: "Generic `KEY:VALUE` which is added to the type-info -> artifact_provides section. Can be given multiple times",
		},
		cli.StringSliceFlag{
			Name:  "depends, d",
			Usage: "Generic `KEY:VALUE` which is added to the type-info -> artifact_depends section. Can be given multiple times",
		},
		cli.StringFlag{
			Name:  "meta-data, m",
			Usage: "The meta-data JSON `FILE` for this update module",
		},
		cli.StringSliceFlag{
			Name:  "file, f",
			Usage: "Include `FILE` in update. Can be given more than once.",
		},
		cli.StringFlag{
			Name:  "augment-type",
			Usage: "Type of augmented update. This is the same as the name of the update module",
		},
		cli.StringSliceFlag{
			Name:  "augment-provides",
			Usage: "Generic `KEY:VALUE` which is added to the augmented type-info -> artifact_provides section. Can be given multiple times",
		},
		cli.StringSliceFlag{
			Name:  "augment-depends",
			Usage: "Generic `KEY:VALUE` which is added to the augmented type-info -> artifact_depends section. Can be given multiple times",
		},
		cli.StringFlag{
			Name:  "augment-meta-data",
			Usage: "The meta-data JSON `FILE` for this update module, for the augmented section",
		},
		cli.StringSliceFlag{
			Name:  "augment-file",
			Usage: "Include `FILE` in updatein the augment section. Can be given more than once.",
		},
	}

	writeCommand := cli.Command{
		Name:  "write",
		Usage: "Writes artifact file.",
		Subcommands: []cli.Command{
			writeRootfsCommand,
			writeModuleCommand,
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
	readCommand := cli.Command{
		Name:        "read",
		Usage:       "Reads artifact file.",
		ArgsUsage:   "<artifact path>",
		Action:      readArtifact,
		Description: "This command validates artifact file provided by pathspec.",
		Flags:       []cli.Flag{key},
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

	modify.Flags = []cli.Flag{
		cli.StringFlag{
			Name:  "key, k",
			Usage: "Full path to the private key that will be used to sign the artifact after modifying.",
		},
		cli.StringFlag{
			Name:  "server-uri, u",
			Usage: "Mender server URI; the default URI will be replaced with given one.",
		},
		cli.StringFlag{
			Name: "server-cert, c",
			Usage: "Full path to the certificate file that will be used for validating " +
				"Mender server by the client.",
		},
		cli.StringFlag{
			Name: "verification-key, v",
			Usage: "Full path to the public verification key that is used by the client  " +
				"to verify the artifact.",
		},
		cli.StringFlag{
			Name:  "name, n",
			Usage: "New name of the artifact.",
		},
		cli.StringFlag{
			Name:  "tenant-token, t",
			Usage: "Full path to the tenant token that will be injected into modified file.",
		},
	}

	copy := cli.Command{
		Name:        "cp",
		Usage:       "cp <src> <dst>",
		Description: "Copies a file into or out of a mender artifact, or sdimg",
		UsageText:   "Copy from or into an artifact, or sdimg where either the <src> or <dst> has to be of the form [artifact|sdimg]:<filepath>",
		Action:      Copy,
	}

	cat := cli.Command{
		Name:        "cat",
		Usage:       "cat [artifact|sdimg|uefiimg]:<filepath>",
		Description: "Cat can output a file from a mender artifact or mender image to stdout.",
		Action:      Cat,
	}

	install := cli.Command{
		Name:        "install",
		Usage:       "install -m<permissions> <src> <dst>",
		Description: "Installs a file from the host filesystem to the artifact or sdimg.",
		Action:      Install,
	}

	install.Flags = []cli.Flag{
		cli.IntFlag{
			Name:  "mode, m",
			Usage: "Set the permission bits in the file",
		},
	}

	app.Commands = []cli.Command{
		writeCommand,
		readCommand,
		validate,
		sign,
		modify,
		copy,
		cat,
		install,
	}
	return app.Run(os.Args)
}
