// Copyright 2026 Northern.tech AS
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
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/pkg/errors"
	"github.com/urfave/cli"

	"github.com/mendersoftware/mender-artifact/areader"
	"github.com/mendersoftware/mender-artifact/artifact"
	"github.com/mendersoftware/mender-artifact/awriter"
	"github.com/mendersoftware/mender-artifact/handlers"
	"github.com/mendersoftware/mender-artifact/utils"
)

const (
	// deltaPayloadType is the name of the update module which applies the
	// payload on the device.
	deltaPayloadType = "mender-binary-delta"
	// deltaAlgorithm is written to the payload meta-data so that the update
	// module knows how to apply the delta.
	deltaAlgorithm = "xdelta3"
	deltaBinary    = "xdelta3"

	standardChecksumKey = "rootfs-image.checksum"
	legacyChecksumKey   = "rootfs_image_checksum"

	decoderArgumentsFlag = "decoder-arguments"
)

// `--decoder-arguments` is meant for the xdelta3 memory options, which bound
// the memory needed to apply the delta. They are also the only options which
// are safe to splice into the encoder command line: anything else can change
// the operation mode entirely (`-c`, for example, writes the delta to stdout
// instead of the output file).
var (
	decoderArgumentRE      = regexp.MustCompile(`^-[BWPI][0-9]*$`)
	decoderArgumentValueRE = regexp.MustCompile(`^[0-9]+$`)
)

// extractedArtifact holds everything the delta Artifact needs to inherit from
// one of its two inputs.
type extractedArtifact struct {
	payloadFile  string
	checksum     string
	name         string
	group        string
	devices      []string
	nameDepends  []string
	groupDepends []string
	provides     artifact.TypeInfoProvides
	depends      artifact.TypeInfoDepends
	clears       []string
	metaData     map[string]interface{}
	scripts      []string
}

func writeDeltaImage(c *cli.Context) error {
	// Both `from` and `to` are declared as required flags, so they are
	// guaranteed to be set by the time the action runs.
	fromPath := c.String("from")
	toPath := c.String("to")

	if err := validateInput(c); err != nil {
		Log.Error(err.Error())
		return err
	}

	decoderArgs, err := deltaDecoderArguments(c)
	if err != nil {
		return cli.NewExitError(err.Error(), errArtifactInvalidParameters)
	}

	tmpDir, err := os.MkdirTemp(c.String("tmp"), "mender-delta-")
	if err != nil {
		return cli.NewExitError(
			"can not create temporary directory: "+err.Error(),
			errArtifactCreate,
		)
	}
	defer os.RemoveAll(tmpDir)

	base, err := extractRootfsArtifact(fromPath, filepath.Join(tmpDir, "base"))
	if err != nil {
		return cli.NewExitError(
			"can not read the base Artifact: "+err.Error(),
			errArtifactInvalidParameters,
		)
	}

	target, err := extractRootfsArtifact(toPath, filepath.Join(tmpDir, "target"))
	if err != nil {
		return cli.NewExitError(
			"can not read the target Artifact: "+err.Error(),
			errArtifactInvalidParameters,
		)
	}

	// Validate the checksum contract before spending time on the delta.
	if _, _, err := deltaChecksumKeys(base, target); err != nil {
		return cli.NewExitError(err.Error(), errArtifactInvalidParameters)
	}

	// `rootfs_file_size` in the meta-data is the size of the uncompressed
	// target rootfs image, not the size of the delta payload.
	info, err := os.Stat(target.payloadFile)
	if err != nil {
		return cli.NewExitError(err.Error(), errArtifactCreate)
	}

	// Name the delta payload after the target rootfs image it reconstructs.
	deltaPath := filepath.Join(tmpDir, filepath.Base(target.payloadFile)+".delta")

	fmt.Fprintln(os.Stderr, "Generating delta...")
	if err := generateDelta(
		base.payloadFile, target.payloadFile, deltaPath, decoderArgs); err != nil {
		return cli.NewExitError(err.Error(), errArtifactCreate)
	}

	return writeDeltaArtifact(c, base, target, deltaPath, info.Size(), decoderArgs)
}

// deltaDecoderArguments returns the extra xdelta3 arguments given with
// `--decoder-arguments`. Each occurrence is split on whitespace so that both
// `-D "-B10 -W20"` and `-D -B10 -D -W20` work. Only the xdelta3 memory
// options are accepted, since the same arguments are spliced into the encoder
// command line and recorded in the meta-data for the decoder.
func deltaDecoderArguments(c *cli.Context) ([]string, error) {
	var args []string
	for _, arg := range c.StringSlice(decoderArgumentsFlag) {
		args = append(args, strings.Fields(arg)...)
	}
	for i := 0; i < len(args); i++ {
		if !decoderArgumentRE.MatchString(args[i]) {
			return nil, fmt.Errorf(
				"unsupported decoder argument %q: only the xdelta3 memory "+
					"options -B, -W, -P and -I can be passed to the decoder "+
					"on the device (e.g. '-B524288000')", args[i])
		}
		if len(args[i]) == 2 {
			// A bare `-B`: the byte size has to follow as its own argument.
			if i+1 >= len(args) || !decoderArgumentValueRE.MatchString(args[i+1]) {
				return nil, fmt.Errorf(
					"decoder argument %q is missing its byte size value", args[i])
			}
			i++
		}
	}
	return args, nil
}

// rootfsExtractStore stores the payload files the same way dump does, but
// rejects Artifacts with an augmented header: their augmented fields would
// otherwise be silently dropped from the generated delta Artifact.
type rootfsExtractStore struct {
	dumpFileStore
}

func (s *rootfsExtractStore) NewUpdateStorer(
	updateType *string,
	payloadNum int,
) (handlers.UpdateStorer, error) {
	return s, nil
}

func (s *rootfsExtractStore) Initialize(artifactHeaders,
	artifactAugmentedHeaders artifact.HeaderInfoer,
	payloadHeaders handlers.ArtifactUpdateHeaders) error {

	if artifactAugmentedHeaders != nil {
		return errors.New("Artifacts with an augmented header are not supported")
	}
	return nil
}

func extractRootfsArtifact(artifactPath, extractDir string) (*extractedArtifact, error) {
	f, err := os.Open(artifactPath)
	if err != nil {
		return nil, errors.Wrap(err, "can not open Artifact")
	}
	defer f.Close()

	scriptsDir := filepath.Join(extractDir, "scripts")
	if err := os.MkdirAll(scriptsDir, 0755); err != nil {
		return nil, errors.Wrap(err, "can not create the scripts directory")
	}

	var scriptFiles []string
	ar := areader.NewReader(f)
	ar.ScriptsReadCallback = func(r io.Reader, info os.FileInfo) error {
		scriptPath := filepath.Join(scriptsDir, info.Name())
		script, err := os.OpenFile(scriptPath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0755)
		if err != nil {
			return err
		}
		defer script.Close()
		if _, err := io.Copy(script, r); err != nil {
			return err
		}
		scriptFiles = append(scriptFiles, scriptPath)
		return nil
	}

	if err := ar.ReadArtifactHeaders(); err != nil {
		return nil, errors.Wrap(err, "can not read Artifact headers")
	}

	if version := ar.GetInfo().Version; version < 3 {
		return nil, fmt.Errorf(
			"only format version 3 Artifacts carry the provides and depends "+
				"a delta update is built on, got version %d", version)
	}

	if ar.IsSigned {
		Log.Warnf("%s is signed, but the signature is not verified during "+
			"delta generation", artifactPath)
	}

	handler, err := rootfsPayloadHandler(ar)
	if err != nil {
		return nil, err
	}

	filesDir := filepath.Join(extractDir, "files")
	handler.SetUpdateStorerProducer(&rootfsExtractStore{
		dumpFileStore{fileDir: filesDir, args: &[]string{}},
	})
	if err := ar.ReadArtifactData(); err != nil {
		return nil, errors.Wrap(err, "can not read Artifact payload")
	}

	files, err := os.ReadDir(filesDir)
	if err != nil {
		return nil, errors.Wrap(err, "can not list the extracted payload")
	}
	if len(files) != 1 {
		return nil, fmt.Errorf(
			"expected exactly one rootfs image in the payload, got %d", len(files))
	}
	payloadPath := filepath.Join(filesDir, files[0].Name())

	checksum, err := sha256File(payloadPath)
	if err != nil {
		return nil, errors.Wrap(err, "can not checksum the extracted payload")
	}

	extracted := &extractedArtifact{
		payloadFile: payloadPath,
		checksum:    checksum,
		name:        ar.GetArtifactName(),
		devices:     ar.GetCompatibleDevices(),
		scripts:     scriptFiles,
	}
	inheritV3Metadata(ar, handler, extracted)

	return extracted, nil
}

func rootfsPayloadHandler(ar *areader.Reader) (handlers.Installer, error) {
	artHandlers := ar.GetHandlers()
	if len(artHandlers) != 1 {
		return nil, fmt.Errorf(
			"delta generation requires an Artifact with exactly one payload, got %d",
			len(artHandlers))
	}

	handler := artHandlers[0]
	updateType := handler.GetUpdateType()
	if updateType == nil || *updateType != "rootfs-image" {
		typeName := "none"
		if updateType != nil {
			typeName = *updateType
		}
		return nil, fmt.Errorf(
			"delta generation requires `rootfs-image` Artifacts, got %q", typeName)
	}
	return handler, nil
}

// inheritV3Metadata copies the header fields the delta Artifact carries over
// from its input, so that provides such as `rootfs-image.version` survive the
// delta update the same way they would survive a full rootfs update.
func inheritV3Metadata(
	ar *areader.Reader,
	handler handlers.Installer,
	extracted *extractedArtifact,
) {
	if provides := ar.GetArtifactProvides(); provides != nil {
		extracted.name = provides.ArtifactName
		extracted.group = provides.ArtifactGroup
	}
	if depends := ar.GetArtifactDepends(); depends != nil {
		extracted.devices = depends.CompatibleDevices
		extracted.nameDepends = depends.ArtifactName
		extracted.groupDepends = depends.ArtifactGroup
	}

	extracted.provides = artifact.TypeInfoProvides{}
	for k, v := range handler.GetUpdateOriginalProvides() {
		extracted.provides[k] = v
	}
	// Values are `interface{}` because a single depends key may hold a list of
	// accepted values, so they have to be copied as-is.
	extracted.depends = artifact.TypeInfoDepends{}
	for k, v := range handler.GetUpdateOriginalDepends() {
		extracted.depends[k] = v
	}
	extracted.clears = handler.GetUpdateOriginalClearsProvides()
	extracted.metaData = handler.GetUpdateOriginalMetaData()
}

// sha256File computes the same hex-encoded checksum
// `writeRootfsImageChecksum` stores in the rootfs-image provides, so the
// declared and the computed values are directly comparable.
func sha256File(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()

	chk := artifact.NewWriterChecksum(io.Discard)
	if _, err := io.Copy(chk, f); err != nil {
		return "", err
	}
	return string(chk.Checksum()), nil
}

func generateDelta(baseFile, targetFile, outputFile string, decoderArgs []string) error {
	xdelta3, err := utils.GetBinaryPath(deltaBinary)
	if err != nil {
		return errors.New("`xdelta3` was not found in the path, it is required to " +
			"generate delta payloads. Install it with `apt install xdelta3`, " +
			"`brew install xdelta`, or the equivalent for your platform")
	}

	// `-S lzma` pins the secondary compression instead of leaving it to the
	// local xdelta3 defaults, so the payload does not depend on how the local
	// xdelta3 was built: one without LZMA support fails here instead of
	// silently producing a much larger delta.
	//
	// `-A` disables the application header, which would otherwise embed the
	// rootfs image file names taken from inside the input Artifacts, and which
	// makes the output depend on the temporary directory layout.
	args := []string{"-e", "-S", "lzma", "-A"}
	// The same tuning has to reach both ends: the encoder needs it to keep its
	// windows within what the device can afford, and `decoder_arguments` in the
	// meta-data passes it to the decoder on the device.
	args = append(args, decoderArgs...)
	args = append(args, "-s", baseFile, targetFile, outputFile)

	cmd := exec.Command(xdelta3, args...)
	cmd.Stdout = os.Stderr
	cmd.Stderr = os.Stderr
	// xdelta3 splices the contents of the `XDELTA` environment variable into
	// its command line, which could override the options pinned above without
	// leaving any trace in the Artifact, so the subprocess must not see it.
	cmd.Env = make([]string, 0, len(os.Environ()))
	for _, e := range os.Environ() {
		if !strings.HasPrefix(e, "XDELTA=") {
			cmd.Env = append(cmd.Env, e)
		}
	}
	if err := cmd.Run(); err != nil {
		return errors.Wrap(err, "xdelta3 failed to generate the delta payload")
	}
	return nil
}

func writeDeltaArtifact(
	c *cli.Context,
	base, target *extractedArtifact,
	deltaPath string,
	rootfsSize int64,
	decoderArgs []string,
) error {
	comp, err := artifact.NewCompressorFromId(c.GlobalString("compression"))
	if err != nil {
		return cli.NewExitError(
			"compressor '"+c.GlobalString("compression")+"' is not supported: "+err.Error(),
			1,
		)
	}

	typeInfo, err := deltaTypeInfo(c, base, target)
	if err != nil {
		return cli.NewExitError(err.Error(), errArtifactInvalidParameters)
	}

	metaData, err := deltaMetaData(c, target, rootfsSize, decoderArgs)
	if err != nil {
		return err
	}

	// State scripts are inherited from the target Artifact, and `--script`
	// adds to them.
	scr, err := scripts(append(append([]string{}, target.scripts...),
		c.StringSlice("script")...))
	if err != nil {
		return cli.NewExitError(err.Error(), 1)
	}

	name := c.String("output-path")
	var w io.Writer
	if name == "-" {
		w = os.Stdout
	} else {
		f, err := os.Create(name)
		if err != nil {
			return cli.NewExitError(
				"can not create artifact file: "+err.Error(),
				errArtifactCreate,
			)
		}
		defer f.Close()
		w = f
	}

	aw, err := artifactWriter(c, comp, w, LatestFormatVersion)
	if err != nil {
		return cli.NewExitError(err.Error(), 1)
	}

	handler := handlers.NewModuleImage(deltaPayloadType)
	if err := handler.SetUpdateFiles([]*handlers.DataFile{{Name: deltaPath}}); err != nil {
		return cli.NewExitError(err.Error(), errArtifactCreate)
	}

	artifactName := c.String("artifact-name")
	if artifactName == "" {
		artifactName = target.name
	}
	devices := getCompatibleDevices(c)
	if len(devices) == 0 {
		devices = target.devices
	}
	group := c.String("provides-group")
	if group == "" {
		group = target.group
	}
	nameDepends := c.StringSlice("artifact-name-depends")
	if len(nameDepends) == 0 {
		nameDepends = target.nameDepends
	}
	groupDepends := c.StringSlice("depends-groups")
	if len(groupDepends) == 0 {
		groupDepends = target.groupDepends
	}

	if !c.Bool("no-progress") {
		ctx, cancel := context.WithCancel(context.Background())
		go reportProgress(ctx, aw.State)
		defer cancel()
		aw.ProgressWriter = utils.NewProgressWriter()
	}

	err = aw.WriteArtifact(&awriter.WriteArtifactArgs{
		Format:  "mender",
		Version: LatestFormatVersion,
		Devices: devices,
		Name:    artifactName,
		Updates: &awriter.Updates{Updates: []handlers.Composer{handler}},
		Scripts: scr,
		Depends: &artifact.ArtifactDepends{
			ArtifactName:      nameDepends,
			CompatibleDevices: devices,
			ArtifactGroup:     groupDepends,
		},
		Provides: &artifact.ArtifactProvides{
			ArtifactName:  artifactName,
			ArtifactGroup: group,
		},
		TypeInfoV3: typeInfo,
		MetaData:   metaData,
	})
	if err != nil {
		return cli.NewExitError(err.Error(), errArtifactCreate)
	}

	return checkArtifactSizeLimits(name, c)
}

// rootfsChecksumKey returns the spelling of the rootfs checksum key used by
// the given provides, preferring the standard one, and whether any was found.
func rootfsChecksumKey(provides artifact.TypeInfoProvides) (string, bool) {
	if _, ok := provides[standardChecksumKey]; ok {
		return standardChecksumKey, true
	}
	if _, ok := provides[legacyChecksumKey]; ok {
		return legacyChecksumKey, true
	}
	return standardChecksumKey, false
}

// deltaChecksumKeys validates the rootfs checksum contract of the two input
// Artifacts and returns the checksum key spelling of each. The two sides are
// resolved independently: the depends side has to use the base Artifact's
// spelling, because that is the key a device running the base image stores its
// checksum under, while the provides side keeps the target Artifact's.
func deltaChecksumKeys(base, target *extractedArtifact) (
	string, string, error,
) {
	baseKey, ok := rootfsChecksumKey(base.provides)
	if !ok {
		return "", "", errors.New(
			"the base Artifact does not provide a rootfs checksum (was it " +
				"written with --no-checksum-provide?), so the generated delta " +
				"could never match the provides of a device running it")
	}
	if declared := base.provides[baseKey]; declared != base.checksum {
		return "", "", fmt.Errorf(
			"the base Artifact provides `%s: %s`, but its rootfs image "+
				"checksums to %s", baseKey, declared, base.checksum)
	}

	targetKey, ok := rootfsChecksumKey(target.provides)
	if ok {
		if declared := target.provides[targetKey]; declared != target.checksum {
			return "", "", fmt.Errorf(
				"the target Artifact provides `%s: %s`, but its rootfs image "+
					"checksums to %s", targetKey, declared, target.checksum)
		}
	}
	return baseKey, targetKey, nil
}

// deltaTypeInfo builds the payload type-info. The base layer is inherited from
// the target Artifact, the shared `--provides`/`--depends`/`--software-*`
// flags overlay it, and the rootfs checksum contract is written last: the
// payload provides the target rootfs and depends on the base rootfs being the
// one currently installed.
func deltaTypeInfo(
	c *cli.Context,
	base, target *extractedArtifact,
) (*artifact.TypeInfoV3, error) {
	typeInfo, _, err := makeTypeInfo(c, deltaPayloadType)
	if err != nil {
		return nil, err
	}

	provides := artifact.TypeInfoProvides{}
	for k, v := range target.provides {
		provides[k] = v
	}
	for k, v := range typeInfo.ArtifactProvides {
		provides[k] = v
	}
	// Values are `interface{}` because a single depends key may hold a list of
	// accepted values, so they have to be copied as-is.
	depends := artifact.TypeInfoDepends{}
	for k, v := range target.depends {
		depends[k] = v
	}
	for k, v := range typeInfo.ArtifactDepends {
		depends[k] = v
	}

	baseKey, targetKey, err := deltaChecksumKeys(base, target)
	if err != nil {
		return nil, err
	}

	// The checksums are the delta contract, always computed from the two
	// payloads, so they win over inherited and `--provides`/`--depends` given
	// values.
	delete(depends, standardChecksumKey)
	delete(depends, legacyChecksumKey)
	provides[targetKey] = target.checksum
	depends[baseKey] = base.checksum

	// makeTypeInfo already collected the command line clears plus the same
	// defaults `write rootfs-image` uses, since a delta replaces the whole
	// rootfs. The target's own clears are merged in on top, so that nothing
	// the target Artifact would have cleared survives a delta update to it.
	clears := typeInfo.ClearsArtifactProvides
	if !c.Bool(noDefaultClearsProvidesFlag) {
		clears = mergeUniqueStrings(clears, target.clears)
	}

	typeInfo.ArtifactProvides = provides
	typeInfo.ArtifactDepends = depends
	typeInfo.ClearsArtifactProvides = clears
	return typeInfo, nil
}

func mergeUniqueStrings(lists ...[]string) []string {
	var merged []string
	seen := map[string]bool{}
	for _, list := range lists {
		for _, v := range list {
			if !seen[v] {
				seen[v] = true
				merged = append(merged, v)
			}
		}
	}
	return merged
}

// deltaMetaData layers the meta-data file given with `--meta-data` over the
// meta-data inherited from the target Artifact, then sets the keys which
// describe the delta payload.
func deltaMetaData(
	c *cli.Context,
	target *extractedArtifact,
	rootfsSize int64,
	decoderArgs []string,
) (map[string]interface{}, error) {
	metaData := map[string]interface{}{}
	for k, v := range target.metaData {
		metaData[k] = v
	}
	userMetaData, _, err := makeMetaData(c)
	if err != nil {
		return nil, err
	}
	for k, v := range userMetaData {
		metaData[k] = v
	}
	metaData["rootfs_file_size"] = rootfsSize
	metaData["delta_algorithm"] = deltaAlgorithm
	if len(decoderArgs) > 0 {
		metaData["decoder_arguments"] = decoderArgs
	}
	return metaData, nil
}
