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
	"bytes"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mendersoftware/mender-artifact/areader"
	"github.com/mendersoftware/mender-artifact/handlers"
	"github.com/mendersoftware/mender-artifact/utils"
)

// deltaTestInputs writes two rootfs images which differ only in a trailing
// block, packs each of them into a `rootfs-image` Artifact, and returns the
// paths to the images and the Artifacts.
type deltaTestInputs struct {
	baseImage      string
	targetImage    string
	baseArtifact   string
	targetArtifact string
}

func makeDeltaTestInputs(t *testing.T, dir string) deltaTestInputs {
	in := deltaTestInputs{
		baseImage:      filepath.Join(dir, "rootfs-v1.ext4"),
		targetImage:    filepath.Join(dir, "rootfs-v2.ext4"),
		baseArtifact:   filepath.Join(dir, "base.mender"),
		targetArtifact: filepath.Join(dir, "target.mender"),
	}

	shared := bytes.Repeat([]byte("shared rootfs content, compresses well\n"), 512)
	require.NoError(t, os.WriteFile(in.baseImage, shared, 0644))
	require.NoError(t, os.WriteFile(in.targetImage,
		append(shared, []byte("and something only the new image has\n")...), 0644))

	require.NoError(t, Run([]string{"mender-artifact", "write", "rootfs-image",
		"-c", "mydevice", "-n", "release-v1", "-f", in.baseImage,
		"-o", in.baseArtifact, "--no-progress"}))
	require.NoError(t, Run([]string{"mender-artifact", "write", "rootfs-image",
		"-c", "mydevice", "-n", "release-v2", "-f", in.targetImage,
		"-o", in.targetArtifact, "--no-progress"}))

	return in
}

// readArtifactPayload reads the Artifact headers, extracts the single payload
// into extractDir and returns the reader, the payload handler and the path to
// the extracted file.
func readArtifactPayload(
	t *testing.T,
	artifactPath, extractDir string,
) (*areader.Reader, handlers.Installer, string) {
	f, err := os.Open(artifactPath)
	require.NoError(t, err)
	t.Cleanup(func() { f.Close() })

	ar := areader.NewReader(f)
	require.NoError(t, ar.ReadArtifactHeaders())

	artHandlers := ar.GetHandlers()
	require.Len(t, artHandlers, 1)
	handler := artHandlers[0]

	handler.SetUpdateStorerProducer(&dumpFileStore{fileDir: extractDir, args: &[]string{}})
	require.NoError(t, ar.ReadArtifactData())

	files, err := os.ReadDir(extractDir)
	require.NoError(t, err)
	require.Len(t, files, 1)

	return ar, handler, filepath.Join(extractDir, files[0].Name())
}

func requireXdelta3(t *testing.T) string {
	path, err := utils.GetBinaryPath("xdelta3")
	if err != nil {
		t.Skip("xdelta3 is not installed, skipping delta Artifact test")
	}
	return path
}

func TestWriteDeltaImage(t *testing.T) {
	requireXdelta3(t)

	tmpdir := t.TempDir()
	in := makeDeltaTestInputs(t, tmpdir)

	baseChecksum, err := sha256File(in.baseImage)
	require.NoError(t, err)
	targetChecksum, err := sha256File(in.targetImage)
	require.NoError(t, err)
	targetInfo, err := os.Stat(in.targetImage)
	require.NoError(t, err)

	deltaArtifact := filepath.Join(tmpdir, "delta.mender")
	require.NoError(t, Run([]string{"mender-artifact", "write", "delta-image",
		"--from", in.baseArtifact, "--to", in.targetArtifact,
		"-o", deltaArtifact, "--no-progress"}))

	deltaPayload := filepath.Join(tmpdir, "extracted")
	ar, handler, payloadPath := readArtifactPayload(t, deltaArtifact, deltaPayload)

	// The delta Artifact identifies itself as the target: same name and the
	// same compatible devices, so it is a drop-in replacement for it.
	assert.Equal(t, "release-v2", ar.GetArtifactName())
	assert.Equal(t, []string{"mydevice"}, ar.GetCompatibleDevices())

	require.NotNil(t, handler.GetUpdateType())
	assert.Equal(t, "mender-binary-delta", *handler.GetUpdateType())

	// Provides the target rootfs, depends on the base rootfs being the one
	// currently installed, and inherits the target's other provides.
	assert.Equal(t, map[string]string{
		"rootfs-image.checksum": targetChecksum,
		"rootfs-image.version":  "release-v2",
	}, map[string]string(handler.GetUpdateOriginalProvides()))
	assert.Equal(t, map[string]interface{}{
		"rootfs-image.checksum": baseChecksum,
	}, map[string]interface{}(handler.GetUpdateOriginalDepends()))

	// A delta replaces the whole rootfs, so it has to clear the same provides
	// a full rootfs-image update clears.
	assert.Equal(t, []string{
		"artifact_group",
		"rootfs_image_checksum",
		"rootfs-image.*",
	}, handler.GetUpdateOriginalClearsProvides())

	// The meta-data which describes how to apply the payload. The size is the
	// uncompressed target rootfs, not the delta.
	assert.Equal(t, map[string]interface{}{
		"delta_algorithm":  "xdelta3",
		"rootfs_file_size": float64(targetInfo.Size()),
	}, handler.GetUpdateOriginalMetaData())

	files := handler.GetUpdateFiles()
	require.Len(t, files, 1)
	assert.Equal(t, "rootfs-v2.ext4.delta", files[0].Name)
	assert.Less(t, files[0].Size, targetInfo.Size(),
		"the delta should be smaller than the image it reconstructs")

	assert.Equal(t, payloadPath, filepath.Join(deltaPayload, "rootfs-v2.ext4.delta"))
}

// TestWriteDeltaImageApplies is the contract that actually matters: feeding
// the generated payload and the base image to the xdelta3 decoder has to
// reproduce the target image byte for byte.
func TestWriteDeltaImageApplies(t *testing.T) {
	xdelta3 := requireXdelta3(t)

	tmpdir := t.TempDir()
	in := makeDeltaTestInputs(t, tmpdir)

	deltaArtifact := filepath.Join(tmpdir, "delta.mender")
	require.NoError(t, Run([]string{"mender-artifact", "write", "delta-image",
		"--from", in.baseArtifact, "--to", in.targetArtifact,
		"-o", deltaArtifact, "--no-progress"}))

	_, _, payloadPath := readArtifactPayload(t, deltaArtifact, filepath.Join(tmpdir, "extracted"))

	applied := filepath.Join(tmpdir, "applied.ext4")
	out, err := exec.Command(
		xdelta3, "-d", "-s", in.baseImage, payloadPath, applied).CombinedOutput()
	require.NoError(t, err, string(out))

	appliedContent, err := os.ReadFile(applied)
	require.NoError(t, err)
	targetContent, err := os.ReadFile(in.targetImage)
	require.NoError(t, err)
	assert.Equal(t, targetContent, appliedContent)
}

// TestWriteDeltaImageDecoderArguments checks that the xdelta3 tuning is
// recorded in the meta-data, from where it reaches the decoder when the delta
// is applied on the device.
func TestWriteDeltaImageDecoderArguments(t *testing.T) {
	requireXdelta3(t)

	tmpdir := t.TempDir()
	in := makeDeltaTestInputs(t, tmpdir)

	deltaArtifact := filepath.Join(tmpdir, "delta.mender")
	require.NoError(t, Run([]string{"mender-artifact", "write", "delta-image",
		"--from", in.baseArtifact, "--to", in.targetArtifact,
		"-o", deltaArtifact, "--no-progress",
		// One flag given as a quoted group and one on its own, both spellings
		// have to end up as separate arguments.
		"-D", "-B10485760 -W1048576", "-D", "-P262144"}))

	_, handler, _ := readArtifactPayload(t, deltaArtifact, filepath.Join(tmpdir, "extracted"))

	assert.Equal(t, []interface{}{"-B10485760", "-W1048576", "-P262144"},
		handler.GetUpdateOriginalMetaData()["decoder_arguments"])
}

func TestWriteDeltaImageSigned(t *testing.T) {
	requireXdelta3(t)

	tmpdir := t.TempDir()
	in := makeDeltaTestInputs(t, tmpdir)

	privateKey := filepath.Join(tmpdir, "private.key")
	require.NoError(t, os.WriteFile(privateKey, []byte(testPrivateKey), 0644))
	publicKey := filepath.Join(tmpdir, "public.key")
	require.NoError(t, os.WriteFile(publicKey, []byte(testPublicKey), 0644))

	deltaArtifact := filepath.Join(tmpdir, "delta.mender")
	require.NoError(t, Run([]string{"mender-artifact", "write", "delta-image",
		"--from", in.baseArtifact, "--to", in.targetArtifact,
		"-o", deltaArtifact, "--no-progress",
		"-k", privateKey, "--compression", "lzma"}))

	require.NoError(t, Run([]string{"mender-artifact", "validate",
		"-k", publicKey, deltaArtifact}))
}

func TestWriteDeltaImageErrors(t *testing.T) {
	tmpdir := t.TempDir()

	payload := filepath.Join(tmpdir, "update.ext4")
	require.NoError(t, os.WriteFile(payload, []byte("some payload"), 0644))

	moduleArtifact := filepath.Join(tmpdir, "module.mender")
	require.NoError(t, Run([]string{"mender-artifact", "write", "module-image",
		"-c", "mydevice", "-n", "module-v1", "-T", "testType", "-f", payload,
		"-o", moduleArtifact, "--no-progress"}))

	rootfsArtifact := filepath.Join(tmpdir, "rootfs.mender")
	require.NoError(t, Run([]string{"mender-artifact", "write", "rootfs-image",
		"-c", "mydevice", "-n", "rootfs-v1", "-f", payload,
		"-o", rootfsArtifact, "--no-progress"}))

	out := filepath.Join(tmpdir, "delta.mender")

	t.Run("missing --to", func(t *testing.T) {
		err := Run([]string{"mender-artifact", "write", "delta-image",
			"--from", rootfsArtifact, "-o", out, "--no-progress"})
		assert.Error(t, err)
	})

	t.Run("base is not a rootfs-image", func(t *testing.T) {
		requireXdelta3(t)
		err := Run([]string{"mender-artifact", "write", "delta-image",
			"--from", moduleArtifact, "--to", rootfsArtifact,
			"-o", out, "--no-progress"})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "requires `rootfs-image` Artifacts")
	})

	t.Run("target is not a rootfs-image", func(t *testing.T) {
		requireXdelta3(t)
		err := Run([]string{"mender-artifact", "write", "delta-image",
			"--from", rootfsArtifact, "--to", moduleArtifact,
			"-o", out, "--no-progress"})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "requires `rootfs-image` Artifacts")
	})

	t.Run("base does not exist", func(t *testing.T) {
		err := Run([]string{"mender-artifact", "write", "delta-image",
			"--from", filepath.Join(tmpdir, "nope.mender"), "--to", rootfsArtifact,
			"-o", out, "--no-progress"})
		assert.Error(t, err)
	})

	t.Run("format version 2 input", func(t *testing.T) {
		v2Artifact := filepath.Join(tmpdir, "v2.mender")
		require.NoError(t, Run([]string{"mender-artifact", "write", "rootfs-image",
			"-c", "mydevice", "-n", "rootfs-v2fmt", "-f", payload, "-v", "2",
			"-o", v2Artifact, "--no-progress"}))
		err := Run([]string{"mender-artifact", "write", "delta-image",
			"--from", v2Artifact, "--to", rootfsArtifact,
			"-o", out, "--no-progress"})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "version 3")
	})

	t.Run("base without a checksum provide", func(t *testing.T) {
		noChecksum := filepath.Join(tmpdir, "nochecksum.mender")
		require.NoError(t, Run([]string{"mender-artifact", "write", "rootfs-image",
			"-c", "mydevice", "-n", "rootfs-nochk", "-f", payload,
			"--no-checksum-provide", "-o", noChecksum, "--no-progress"}))
		err := Run([]string{"mender-artifact", "write", "delta-image",
			"--from", noChecksum, "--to", rootfsArtifact,
			"-o", out, "--no-progress"})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "rootfs checksum")
	})

	t.Run("unsupported decoder argument", func(t *testing.T) {
		// `-c` would make the encoder dump the delta to stdout, so anything
		// outside the xdelta3 memory options has to be rejected up front.
		err := Run([]string{"mender-artifact", "write", "delta-image",
			"--from", rootfsArtifact, "--to", rootfsArtifact,
			"-o", out, "-D", "-c", "--no-progress"})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "unsupported decoder argument")
	})

	t.Run("decoder argument without its value", func(t *testing.T) {
		err := Run([]string{"mender-artifact", "write", "delta-image",
			"--from", rootfsArtifact, "--to", rootfsArtifact,
			"-o", out, "-D", "-B", "--no-progress"})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "byte size")
	})

	t.Run("device-type and compatible-types conflict", func(t *testing.T) {
		err := Run([]string{"mender-artifact", "write", "delta-image",
			"--from", rootfsArtifact, "--to", rootfsArtifact,
			"-t", "somedevice", "-c", "otherdevice",
			"-o", out, "--no-progress"})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "mutually exclusive")
	})

	t.Run("whitespace in artifact-name", func(t *testing.T) {
		err := Run([]string{"mender-artifact", "write", "delta-image",
			"--from", rootfsArtifact, "--to", rootfsArtifact,
			"-n", "two words", "-o", out, "--no-progress"})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "whitespace")
	})

	t.Run("augmented input", func(t *testing.T) {
		augmentPayload := filepath.Join(tmpdir, "augment.ext4")
		require.NoError(t, os.WriteFile(augmentPayload, []byte("augment payload"), 0644))
		augmented := filepath.Join(tmpdir, "augmented.mender")
		require.NoError(t, Run([]string{"mender-artifact", "write", "module-image",
			"-c", "mydevice", "-n", "augmented-v1", "-T", "rootfs-image",
			"-f", payload, "--augment-type", "rootfs-image",
			"--augment-file", augmentPayload,
			"-o", augmented, "--no-progress"}))
		err := Run([]string{"mender-artifact", "write", "delta-image",
			"--from", augmented, "--to", rootfsArtifact,
			"-o", out, "--no-progress"})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "augmented")
	})
}

// TestWriteDeltaImageChecksumKeySpellings: the two sides of the checksum
// contract are resolved independently. A device running a legacy base image
// stores its checksum under `rootfs_image_checksum`, so that is the key the
// depends has to use, while the provides keep the target's standard spelling.
func TestWriteDeltaImageChecksumKeySpellings(t *testing.T) {
	requireXdelta3(t)

	tmpdir := t.TempDir()
	in := makeDeltaTestInputs(t, tmpdir)

	// Rewrite the base as a legacy Artifact.
	require.NoError(t, Run([]string{"mender-artifact", "write", "rootfs-image",
		"-c", "mydevice", "-n", "release-v1", "-f", in.baseImage,
		"-o", in.baseArtifact, "--legacy-rootfs-image-checksum", "--no-progress"}))

	baseChecksum, err := sha256File(in.baseImage)
	require.NoError(t, err)
	targetChecksum, err := sha256File(in.targetImage)
	require.NoError(t, err)

	deltaArtifact := filepath.Join(tmpdir, "delta.mender")
	require.NoError(t, Run([]string{"mender-artifact", "write", "delta-image",
		"--from", in.baseArtifact, "--to", in.targetArtifact,
		"-o", deltaArtifact, "--no-progress"}))

	_, handler, _ := readArtifactPayload(t, deltaArtifact, filepath.Join(tmpdir, "extracted"))

	provides := map[string]string(handler.GetUpdateOriginalProvides())
	assert.Equal(t, targetChecksum, provides["rootfs-image.checksum"])
	assert.NotContains(t, provides, "rootfs_image_checksum")

	assert.Equal(t, map[string]interface{}{
		"rootfs_image_checksum": baseChecksum,
	}, map[string]interface{}(handler.GetUpdateOriginalDepends()))
}

// TestWriteDeltaImageOverrides: the shared payload flags overlay what is
// inherited from the target Artifact, and `--clears-provides` adds to the
// defaults instead of replacing them, like it does for the other write
// commands.
func TestWriteDeltaImageOverrides(t *testing.T) {
	requireXdelta3(t)

	tmpdir := t.TempDir()
	in := makeDeltaTestInputs(t, tmpdir)

	metaFile := filepath.Join(tmpdir, "meta.json")
	require.NoError(t, os.WriteFile(metaFile, []byte(`{"custom_meta": "yes"}`), 0644))

	deltaArtifact := filepath.Join(tmpdir, "delta.mender")
	require.NoError(t, Run([]string{"mender-artifact", "write", "delta-image",
		"--from", in.baseArtifact, "--to", in.targetArtifact,
		"-o", deltaArtifact, "--no-progress",
		"-n", "delta-name",
		"--software-version", "v2.override",
		"-p", "custom.key:custom-value",
		"-d", "custom.dep:dep-value",
		"-m", metaFile,
		"--clears-provides", "custom.*"}))

	ar, handler, _ := readArtifactPayload(t, deltaArtifact, filepath.Join(tmpdir, "extracted"))

	assert.Equal(t, "delta-name", ar.GetArtifactName())

	provides := map[string]string(handler.GetUpdateOriginalProvides())
	assert.Equal(t, "custom-value", provides["custom.key"])
	// An explicit --software-version overrides the version inherited from the
	// target Artifact, but the artifact name alone does not.
	assert.Equal(t, "v2.override", provides["rootfs-image.version"])

	depends := map[string]interface{}(handler.GetUpdateOriginalDepends())
	assert.Equal(t, "dep-value", depends["custom.dep"])

	clears := handler.GetUpdateOriginalClearsProvides()
	assert.Contains(t, clears, "custom.*")
	assert.Contains(t, clears, "artifact_group")
	assert.Contains(t, clears, "rootfs_image_checksum")
	assert.Contains(t, clears, "rootfs-image.*")

	metaData := handler.GetUpdateOriginalMetaData()
	assert.Equal(t, "yes", metaData["custom_meta"])
	assert.Equal(t, "xdelta3", metaData["delta_algorithm"])
}

// TestWriteDeltaImageScripts: state scripts are inherited from the target
// Artifact, and `--script` adds to them.
func TestWriteDeltaImageScripts(t *testing.T) {
	requireXdelta3(t)

	tmpdir := t.TempDir()
	in := makeDeltaTestInputs(t, tmpdir)

	inheritedScript := filepath.Join(tmpdir, "ArtifactInstall_Enter_01")
	require.NoError(t, os.WriteFile(inheritedScript, []byte("#!/bin/sh\ntrue\n"), 0755))
	addedScript := filepath.Join(tmpdir, "ArtifactCommit_Leave_01")
	require.NoError(t, os.WriteFile(addedScript, []byte("#!/bin/sh\ntrue\n"), 0755))

	// Rewrite the target with a state script attached.
	require.NoError(t, Run([]string{"mender-artifact", "write", "rootfs-image",
		"-c", "mydevice", "-n", "release-v2", "-f", in.targetImage,
		"-o", in.targetArtifact, "-s", inheritedScript, "--no-progress"}))

	deltaArtifact := filepath.Join(tmpdir, "delta.mender")
	require.NoError(t, Run([]string{"mender-artifact", "write", "delta-image",
		"--from", in.baseArtifact, "--to", in.targetArtifact,
		"-o", deltaArtifact, "-s", addedScript, "--no-progress"}))

	f, err := os.Open(deltaArtifact)
	require.NoError(t, err)
	defer f.Close()
	ar := areader.NewReader(f)
	var scriptNames []string
	ar.ScriptsReadCallback = func(r io.Reader, info os.FileInfo) error {
		scriptNames = append(scriptNames, info.Name())
		return nil
	}
	require.NoError(t, ar.ReadArtifactHeaders())

	assert.Contains(t, scriptNames, "ArtifactInstall_Enter_01")
	assert.Contains(t, scriptNames, "ArtifactCommit_Leave_01")
}

// TestWriteDeltaImageIgnoresXdeltaEnv: xdelta3 splices the contents of the
// XDELTA environment variable into its command line, which could silently
// override the pinned encoder options, so `write delta-image` must shield the
// subprocess from it.
func TestWriteDeltaImageIgnoresXdeltaEnv(t *testing.T) {
	requireXdelta3(t)

	tmpdir := t.TempDir()
	in := makeDeltaTestInputs(t, tmpdir)

	// If this leaked through to the subprocess, xdelta3 would exit non-zero.
	t.Setenv("XDELTA", "--not-a-valid-option")

	deltaArtifact := filepath.Join(tmpdir, "delta.mender")
	assert.NoError(t, Run([]string{"mender-artifact", "write", "delta-image",
		"--from", in.baseArtifact, "--to", in.targetArtifact,
		"-o", deltaArtifact, "--no-progress"}))
}

// TestWriteDeltaImageStdout: `-o -` streams the Artifact to stdout instead of
// creating a file named `-`.
func TestWriteDeltaImageStdout(t *testing.T) {
	requireXdelta3(t)

	tmpdir := t.TempDir()
	in := makeDeltaTestInputs(t, tmpdir)

	oldStdout := os.Stdout
	r, w, err := os.Pipe()
	require.NoError(t, err)
	os.Stdout = w
	defer func() { os.Stdout = oldStdout }()

	var buf bytes.Buffer
	done := make(chan error, 1)
	go func() {
		_, copyErr := io.Copy(&buf, r)
		done <- copyErr
	}()

	runErr := Run([]string{"mender-artifact", "write", "delta-image",
		"--from", in.baseArtifact, "--to", in.targetArtifact,
		"-o", "-", "--no-progress"})
	w.Close()
	os.Stdout = oldStdout
	require.NoError(t, <-done)
	require.NoError(t, runErr)

	assert.NotZero(t, buf.Len(), "the Artifact should have been written to stdout")
	_, err = os.Stat("-")
	assert.True(t, os.IsNotExist(err), "a file named '-' must not be created")
}
