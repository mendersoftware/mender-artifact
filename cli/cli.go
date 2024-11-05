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
	"fmt"
	"sort"
	"strings"

	"github.com/mendersoftware/mender-artifact/artifact"

	"github.com/urfave/cli"
)

const (
	errArtifactInvalidParameters = iota
	errArtifactUnsupportedVersion
	errArtifactCreate
	errArtifactOpen
	errArtifactInvalid
	errArtifactUnsupportedFeature
	errSystemError
)

const (
	clearsProvidesFlag           = "clears-provides"
	deleteClearsProvidesFlag     = "delete-clears-provides"
	noDefaultSoftwareVersionFlag = "no-default-software-version"
	noDefaultClearsProvidesFlag  = "no-default-clears-provides"
	softwareNameFlag             = "software-name"
	softwareVersionFlag          = "software-version"
	softwareFilesystemFlag       = "software-filesystem"
)

// Version of the mender-artifact CLI tool
var Version = "unknown"

// LatestFormatVersion is the latest version of the format, which is
// also what we default to.
const LatestFormatVersion = 3

// Copied from urfave/cli/template.go
// with the addition of the NOTE on the `global` `--compression flag`
//
//nolint:lll
var menderAppHelpTemplate = `NAME:
   {{.Name}}{{if .Usage}} - {{.Usage}}{{end}}

USAGE:
   {{if .UsageText}}{{.UsageText}}{{else}}{{.HelpName}} {{if .VisibleFlags}}[global options]{{end}}{{if .Commands}} command [command options]{{end}} {{if .ArgsUsage}}{{.ArgsUsage}}{{else}}[arguments...]{{end}}{{end}}{{if .Version}}{{if not .HideVersion}}

VERSION:
   {{.Version}}{{end}}{{end}}{{if .Description}}

DESCRIPTION:
   {{.Description}}{{end}}{{if len .Authors}}

AUTHOR{{with $length := len .Authors}}{{if ne 1 $length}}S{{end}}{{end}}:
   {{range $index, $author := .Authors}}{{if $index}}
   {{end}}{{$author}}{{end}}{{end}}{{if .VisibleCommands}}

COMMANDS:{{range .VisibleCategories}}{{if .Name}}

   {{.Name}}:{{range .VisibleCommands}}
     {{join .Names ", "}}{{"\t"}}{{.Usage}}{{end}}{{else}}{{range .VisibleCommands}}
   {{join .Names ", "}}{{"\t"}}{{.Usage}}{{end}}{{end}}{{end}}{{end}}{{if .VisibleFlags}}

GLOBAL OPTIONS:
   {{range $index, $option := .VisibleFlags}}{{if $index}}
   {{end}}{{$option}}{{end}}{{end}}
   NOTE:
       For the commands <write>, <modify>, the '--compression' flag functions as
       a global option
`

func applyCompressionInCommand(c *cli.Context) error {
	// Let --compression argument work after command as well. Latest one
	// applies.
	if c.String("compression") != "" {
		_ = c.GlobalSet("compression", c.String("compression"))
	}
	return nil
}

func Run(args []string) error {
	return getCliContext().Run(args)
}

func getCliContext() *cli.App {
	app := cli.NewApp()
	app.Name = "mender-artifact"
	app.Usage = "interface for manipulating Mender artifacts"
	app.UsageText = "mender-artifact [--version][--help] <command> [<args>]"
	app.Version = Version

	app.Author = "Northern.tech AS"
	app.Email = "contact@northern.tech"

	app.EnableBashCompletion = true

	compressors := artifact.GetRegisteredCompressorIds()

	compressionFlag := cli.StringFlag{
		Name: "compression",
		Usage: fmt.Sprintf("Compression to use for the artifact, "+
			"currently supports: %v.", strings.Join(compressors, ", ")),
	}
	globalCompressionFlag := compressionFlag
	// The global flag is the last fallback, so here we provide a default.
	globalCompressionFlag.Value = "gzip"
	globalCompressionFlag.Hidden = true

	privateKeyFlag := cli.StringFlag{
		Name: "key, k",
		Usage: "Full path to the private key that will be used to sign " +
			"the Artifact.",
	}

	gcpKMSKeyFlag := cli.StringFlag{
		Name: "gcp-kms-key",
		Usage: "Resource ID of the GCP KMS key that will be used to sign " +
			"the Artifact.",
	}

	signserverWorkerName := cli.StringFlag{
		Name: "keyfactor-signserver-worker",
		Usage: "The name of the SignServer worker that will be used to sign " +
			"the Artifact. The worker name must be associated with a Plain Signer worker " +
			"in SignServer. ",
	}

	vaultTransitKeyFlag := cli.StringFlag{
		Name: "vault-transit-key",
		Usage: "Key name of the Hashicorp Vault transit key that will be used to sign " +
			"the Artifact. VAULT_TOKEN and VAULT_MOUNT_PATH environment variables " +
			"needs to be provided. The default Hashicorp Vault URL can be overridden with " +
			"VAULT_ADDR environment variable. If key rotation is used, the key version " +
			"to sign can be specified with VAULT_KEY_VERSION environment variable.",
	}

	pkcs11Flag := cli.StringFlag{
		Name:  "key-pkcs11",
		Usage: "Use PKCS#11 interface to sign and verify artifacts",
	}

	publicKeyFlag := cli.StringFlag{
		Name: "key, k",
		Usage: "Full path to the public key that will be used to verify " +
			"the Artifact signature.",
	}

	//
	// Common Artifact flags
	//
	artifactName := cli.StringFlag{
		Name:     "artifact-name, n",
		Usage:    "Name of the artifact",
		Required: true,
	}

	artifactNameDepends := cli.StringSliceFlag{
		Name:  "artifact-name-depends, N",
		Usage: "Sets the name(s) of the artifact(s) which this update depends upon",
	}
	artifactProvidesGroup := cli.StringFlag{
		Name:  "provides-group, g",
		Usage: "The group the artifact provides",
	}
	artifactDependsGroups := cli.StringSliceFlag{
		Name:  "depends-groups, G",
		Usage: "The group(s) the artifact depends on",
	}
	artifactAddScripts := cli.StringSliceFlag{
		Name: "script, s",
		Usage: "Adds additional state script to an already existing artifact." +
			"You can specify multiple scripts providing this parameter multiple times.",
	}

	// Common Software Version flags
	softwareVersionNoDefault := cli.BoolFlag{
		Name:  noDefaultSoftwareVersionFlag,
		Usage: "Disable the software version field for compatibility with old clients",
	}
	softwareVersionValue := cli.StringFlag{
		Name:  softwareVersionFlag,
		Usage: "Value for the software version, defaults to the name of the artifact",
	}
	softwareFilesystem := cli.StringFlag{
		Name:  softwareFilesystemFlag,
		Usage: "If specified, is used instead of rootfs-image",
	}

	//
	// Common Payload flags
	//
	payloadProvides := cli.StringSliceFlag{
		Name: "provides, p",
		Usage: "Generic `KEY:VALUE` which is added to the type-info -> artifact_provides section." +
			" Can be given multiple times",
	}
	payloadDepends := cli.StringSliceFlag{
		Name: "depends, d",
		Usage: "Generic `KEY:VALUE` which is added to the type-info -> artifact_depends section." +
			" Can be given multiple times",
	}
	payloadMetaData := cli.StringFlag{
		Name:  "meta-data, m",
		Usage: "The meta-data JSON `FILE` for this payload",
	}
	clearsArtifactProvides := cli.StringSliceFlag{
		Name:  clearsProvidesFlag,
		Usage: "Add a clears_artifact_provides field to Artifact payload",
	}
	noDefaultClearsArtifactProvides := cli.BoolFlag{
		Name:  noDefaultClearsProvidesFlag,
		Usage: "Do not add any default clears_artifact_provides fields to Artifact payload",
	}

	//
	// write
	//
	writeRootfsCommand := cli.Command{
		Name:   "rootfs-image",
		Action: writeRootfs,
		Usage:  "Writes Mender artifact containing rootfs image",
	}

	writeRootfsCommand.CustomHelpTemplate = CustomSubcommandHelpTemplate

	writeRootfsCommand.Flags = []cli.Flag{
		cli.StringFlag{
			Name: "file, f",
			Usage: "Payload `FILE` path or ssh-url to device for system " +
				"snapshot (e.g. ssh://user@device:22022).",
			Required: true,
		},
		cli.StringSliceFlag{
			Name: "device-type, t",
			Usage: "Type of device(s) supported by the Artifact. You can specify multiple " +
				"compatible devices providing this parameter multiple times.",
			Required: true,
		},
		artifactName,
		cli.StringFlag{
			Name:  "output-path, o",
			Usage: "Full path to output artifact file, '-' for stdout.",
		},
		cli.IntFlag{
			Name:  "version, v",
			Usage: "Version of the artifact.",
			Value: LatestFormatVersion,
		},
		privateKeyFlag,
		gcpKMSKeyFlag,
		vaultTransitKeyFlag,
		signserverWorkerName,
		cli.StringSliceFlag{
			Name: "script, s",
			Usage: "Full path to the state script(s). You can specify multiple " +
				"scripts providing this parameter multiple times.",
		},
		cli.BoolFlag{
			Name: "legacy-rootfs-image-checksum",
			Usage: "Use the legacy key name rootfs_image_checksum to store the providese checksum" +
				" to the Artifact provides parameters instead of rootfs-image.checksum.",
		},
		cli.BoolFlag{
			Name: "no-checksum-provide",
			Usage: "Disable writing the provides checksum to the Artifact provides " +
				"parameters. This is needed in case the targeted devices do not support " +
				"provides and depends yet.",
		},
		cli.StringSliceFlag{
			Name: "ssh-args, S",
			Usage: "Arguments to pass to ssh - only applies when " +
				"creating artifact from snapshot (i.e. FILE " +
				"contains 'ssh://' schema)",
		},
		cli.BoolFlag{
			Name:  "no-progress",
			Usage: "Suppress the progressbar output",
		},
		/////////////////////////
		// Version 3 specifics.//
		/////////////////////////
		artifactNameDepends,
		artifactProvidesGroup,
		artifactDependsGroups,
		payloadDepends,
		payloadProvides,
		clearsArtifactProvides,
		noDefaultClearsArtifactProvides,
		compressionFlag,
		//////////////////////
		// Sotware versions //
		//////////////////////
		softwareVersionNoDefault,
		cli.StringFlag{
			Name: softwareNameFlag,
			Usage: "Name of the key to store the software version: rootfs-image.NAME.version," +
				" instead of rootfs-image.version",
		},
		softwareVersionValue,
		softwareFilesystem,
	}

	writeRootfsCommand.Before = applyCompressionInCommand

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

	writeModuleCommand.CustomHelpTemplate = CustomSubcommandHelpTemplate

	writeModuleCommand.Flags = []cli.Flag{
		cli.StringSliceFlag{
			Name: "device-type, t",
			Usage: "Type of device(s) supported by the Artifact. You can specify multiple " +
				"compatible devices providing this parameter multiple times.",
			Required: true,
		},
		cli.StringFlag{
			Name:  "output-path, o",
			Usage: "Full path to output artifact file, '-' for stdout.",
		},
		cli.IntFlag{
			Name:  "version, v",
			Usage: "Version of the artifact.",
			Value: LatestFormatVersion,
		},
		cli.StringSliceFlag{
			Name: "script, s",
			Usage: "Full path to the state script(s). You can specify multiple " +
				"scripts providing this parameter multiple times.",
		},
		artifactName,
		artifactNameDepends,
		artifactProvidesGroup,
		artifactDependsGroups,
		cli.StringFlag{
			Name:     "type, T",
			Usage:    "Type of payload. This is the same as the name of the update module",
			Required: true,
		},
		payloadProvides,
		payloadDepends,
		payloadMetaData,
		cli.StringSliceFlag{
			Name:  "file, f",
			Usage: "Include `FILE` in payload. Can be given more than once.",
		},
		cli.StringFlag{
			Name:  "augment-type",
			Usage: "Type of augmented payload. This is the same as the name of the update module",
		},
		cli.StringSliceFlag{
			Name: "augment-provides",
			Usage: "Generic `KEY:VALUE` which is added to the augmented type-info ->" +
				" artifact_provides section. Can be given multiple times",
		},
		cli.StringSliceFlag{
			Name: "augment-depends",
			Usage: "Generic `KEY:VALUE` which is added to the augmented type-info ->" +
				" artifact_depends section. Can be given multiple times",
		},
		cli.StringFlag{
			Name:  "augment-meta-data",
			Usage: "The meta-data JSON `FILE` for this payload, for the augmented section",
		},
		cli.StringSliceFlag{
			Name:  "augment-file",
			Usage: "Include `FILE` in payload in the augment section. Can be given more than once.",
		},
		clearsArtifactProvides,
		noDefaultClearsArtifactProvides,
		compressionFlag,
		privateKeyFlag,
		gcpKMSKeyFlag,
		vaultTransitKeyFlag,
		signserverWorkerName,
		//////////////////////
		// Sotware versions //
		//////////////////////
		softwareVersionNoDefault,
		cli.StringFlag{
			Name: softwareNameFlag,
			Usage: "Name of the key to store the software version: rootfs-image.NAME.version," +
				" instead of rootfs-image.PAYLOAD_TYPE.version",
		},
		softwareVersionValue,
		softwareFilesystem,
	}
	writeModuleCommand.Before = applyCompressionInCommand

	//
	// Write Bootstrap artifact
	//
	writeBootstrapArtifactCommand := cli.Command{
		Name:   "bootstrap-artifact",
		Action: writeBootstrapArtifact,
		Usage:  "Writes Mender bootstrap artifact containing empty payload",
	}

	writeBootstrapArtifactCommand.CustomHelpTemplate = CustomSubcommandHelpTemplate

	writeBootstrapArtifactCommand.Flags = []cli.Flag{
		cli.StringSliceFlag{
			Name: "device-type, t",
			Usage: "Type of device(s) supported by the Artifact. You can specify multiple " +
				"compatible devices providing this parameter multiple times.",
			Required: true,
		},
		artifactName,
		cli.StringFlag{
			Name:  "output-path, o",
			Usage: "Full path to output artifact file, '-' for standard output.",
		},
		cli.IntFlag{
			Name:  "version, v",
			Usage: "Version of the artifact.",
			Value: LatestFormatVersion,
		},
		cli.BoolFlag{
			Name:  "no-progress",
			Usage: "Suppress the progressbar output",
		},
		compressionFlag,
		clearsArtifactProvides,
		payloadProvides,
		payloadDepends,
		privateKeyFlag,
		gcpKMSKeyFlag,
		signserverWorkerName,
		vaultTransitKeyFlag,
		/////////////////////////
		// Version 3 specifics.//
		/////////////////////////
		artifactNameDepends,
		artifactProvidesGroup,
		artifactDependsGroups,
	}

	writeBootstrapArtifactCommand.Before = applyCompressionInCommand

	writeCommand := cli.Command{
		Name:     "write",
		Usage:    "Writes artifact file.",
		Category: "Artifact creation and validation",
		Subcommands: []cli.Command{
			writeRootfsCommand,
			writeModuleCommand,
			writeBootstrapArtifactCommand,
		},
	}

	//
	// validate
	//
	validate := cli.Command{
		Name:        "validate",
		Usage:       "Validates artifact file.",
		Category:    "Artifact creation and validation",
		Action:      validateArtifact,
		UsageText:   "mender-artifact validate [options] <pathspec>",
		Description: "This command validates artifact file provided by pathspec.",
		Flags: []cli.Flag{
			publicKeyFlag,
			gcpKMSKeyFlag,
			signserverWorkerName,
			vaultTransitKeyFlag,
			pkcs11Flag,
		},
	}

	//
	// read
	//
	readCommand := cli.Command{
		Name:        "read",
		Usage:       "Reads artifact file.",
		ArgsUsage:   "<artifact path>",
		Category:    "Artifact inspection",
		Action:      readArtifact,
		Description: "This command validates artifact file provided by pathspec.",
		Flags: []cli.Flag{
			publicKeyFlag,
			gcpKMSKeyFlag,
			signserverWorkerName,
			vaultTransitKeyFlag,
			pkcs11Flag,
			cli.BoolFlag{
				Name:  "no-progress",
				Usage: "Suppress the progressbar output",
			},
		},
	}

	//
	// sign
	//
	sign := cli.Command{

		Name:        "sign",
		Usage:       "Signs existing artifact file.",
		Category:    "Artifact modification",
		Action:      signExisting,
		UsageText:   "mender-artifact sign [options] <pathspec>",
		Description: "This command signs artifact file provided by pathspec.",
	}
	sign.Flags = []cli.Flag{
		privateKeyFlag,
		gcpKMSKeyFlag,
		signserverWorkerName,
		vaultTransitKeyFlag,
		cli.StringFlag{
			Name: "output-path, o",
			Usage: "Full path to output signed artifact file; " +
				"if none is provided existing artifact will be replaced with the signed one",
		},
		cli.BoolFlag{
			Name:  "force, f",
			Usage: "Force creating new signature if the artifact is already signed",
		},
		pkcs11Flag,
	}

	//
	// modify existing
	//
	modify := cli.Command{
		Name:      "modify",
		Usage:     "Modifies image or artifact file.",
		Category:  "Artifact modification",
		Action:    modifyArtifact,
		UsageText: "mender-artifact modify [options] <pathspec>",
		Description: "This command modifies existing image or artifact file provided by pathspec." +
			" NOTE: Currently only ext4 payloads can be modified",
	}

	modify.Flags = []cli.Flag{
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
			Usage: "Full path to the public verification key that is used by the client " +
				"to verify the artifact.",
		},
		cli.StringFlag{
			Name:  "artifact-name, n",
			Usage: "Name of the artifact",
		},
		cli.StringFlag{
			Name:  "name",
			Usage: "Deprecated. This is an alias for --artifact-name",
		},
		artifactNameDepends,
		artifactProvidesGroup,
		artifactDependsGroups,
		artifactAddScripts,
		payloadProvides,
		payloadDepends,
		payloadMetaData,
		clearsArtifactProvides,
		cli.StringSliceFlag{
			Name:  deleteClearsProvidesFlag,
			Usage: "Erase one \"Clears Provides\" filter from the Artifact.",
		},
		cli.StringFlag{
			Name:  "tenant-token, t",
			Usage: "Full path to the tenant token that will be injected into modified file.",
		},
		privateKeyFlag,
		gcpKMSKeyFlag,
		signserverWorkerName,
		vaultTransitKeyFlag,
		compressionFlag,
	}
	modify.Before = func(c *cli.Context) error {
		if c.String("name") != "" {
			_ = c.Set("artifact-name", c.String("name"))
		}
		return applyCompressionInCommand(c)
	}

	copy := cli.Command{
		Name:        "cp",
		Usage:       "cp <src> <dst>",
		Category:    "Artifact modification",
		Description: "Copies a file into or out of a mender artifact, or sdimg",
		UsageText: "Copy from or into an artifact, or sdimg where either the <src>" +
			" or <dst> has to be of the form [artifact|sdimg]:<filepath>, <src> can" +
			"come from stdin in the case that <src> is '-'",
		Action: Copy,
	}

	copy.Flags = []cli.Flag{
		compressionFlag,
		privateKeyFlag,
		gcpKMSKeyFlag,
		signserverWorkerName,
		vaultTransitKeyFlag,
	}

	cat := cli.Command{
		Name:        "cat",
		Usage:       "cat [artifact|sdimg|uefiimg]:<filepath>",
		Description: "Cat can output a file from a mender artifact or mender image to stdout.",
		Category:    "Artifact modification",
		Action:      Cat,
	}

	install := cli.Command{
		Name: "install",
		Usage: "install -m <permissions> <hostfile> [artifact|sdimg|uefiimg]:<filepath> or" +
			" install -d [artifact|sdimg|uefiimg]:<directory>",
		Description: "Installs a directory, or a file from the host filesystem, to the artifact" +
			" or sdimg.",
		Category: "Artifact modification",
		Action:   Install,
	}

	install.Flags = []cli.Flag{
		cli.IntFlag{
			Name:  "mode, m",
			Usage: "Set the permission bits in the file",
		},
		cli.BoolFlag{
			Name:  "directory, d",
			Usage: "Create a directory inside an artifact",
		},
	}

	remove := cli.Command{
		Name:        "rm",
		Usage:       "rm [artifact|sdimg|uefiimg]:<filepath>",
		Category:    "Artifact modification",
		Description: "Removes the given file or directory from an Artifact or sdimg.",
		Action:      Remove,
	}

	remove.Flags = []cli.Flag{
		cli.BoolFlag{
			Name:  "recursive, r",
			Usage: "remove directories and their contents recursively",
		},
	}

	//
	// dump
	//
	dumpCommand := cli.Command{
		Name:      "dump",
		Usage:     "Dump contents from Artifacts",
		ArgsUsage: "<Artifact>",
		Description: "Dump various raw files from the Artifact. These can be used to create a new" +
			" Artifact with the same components.",
		Category: "Artifact inspection",
		Action:   DumpCommand,
	}
	dumpCommand.Flags = []cli.Flag{
		cli.StringFlag{
			Name:  "files",
			Usage: "Dump all included files in the first payload into given folder",
		},
		cli.StringFlag{
			Name: "meta-data",
			Usage: "Dump the contents of the meta-data field in the first payload into given" +
				" folder",
		},
		cli.StringFlag{
			Name:  "scripts",
			Usage: "Dump all included state scripts into given folder",
		},
		cli.BoolFlag{
			Name: "print-cmdline",
			Usage: "Print the command line that can recreate the same Artifact with the" +
				" components being dumped. If all the components are being dumped, a nearly" +
				" identical Artifact can be created. Note that timestamps will cause the checksum" +
				" of the Artifact to be different, and signatures can not be recreated this way." +
				" The command line will only use long option names.",
		},
		cli.BoolFlag{
			Name: "print0-cmdline",
			Usage: "Same as 'print-cmdline', except that the arguments are separated by a null" +
				" character (0x00).",
		},
	}

	globalFlags := []cli.Flag{
		globalCompressionFlag,
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
		remove,
		dumpCommand,
	}
	app.Flags = append([]cli.Flag{}, globalFlags...)

	// Display all flags and commands alphabetically
	for _, cmd := range app.Commands {
		sortFlags(cmd)
	}

	app.CustomAppHelpTemplate = menderAppHelpTemplate
	return app
}

func sortFlags(c cli.Command) {
	sort.Sort(cli.FlagsByName(c.Flags))
	sort.Sort(cli.CommandsByName(c.Subcommands))
	for _, cmd := range c.Subcommands {
		sortFlags(cmd)
	}
}
