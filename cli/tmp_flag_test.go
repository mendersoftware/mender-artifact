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
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// setupImpossibleTmp creates a file and returns a sub-path that cannot exist
func setupImpossibleTmp(t *testing.T, workDir string) string {
	blockedPath := filepath.Join(workDir, "i_am_a_file")
	err := os.WriteFile(blockedPath, []byte("data"), 0644)
	require.NoError(t, err)
	return filepath.Join(blockedPath, "workspace")
}

func TestTmpFlagRespectedInInstall(t *testing.T) {
	workDir := t.TempDir()
	artifactPath := filepath.Join(workDir, "install.mender")
	moduleFile := filepath.Join(workDir, "module.img")
	menderConfFile := filepath.Join(workDir, "mender.conf")

	// Setup mock files
	require.NoError(t, os.WriteFile(moduleFile, []byte("fake module content"), 0644))
	require.NoError(t, os.WriteFile(menderConfFile, []byte("fake config content"), 0644))

	// Create initial artifact
	err := Run([]string{
		"mender-artifact", "write", "module-image",
		"-n", "initial-name",
		"-t", "test-device",
		"-T", "test-device-type",
		"-o", artifactPath,
	})
	require.NoError(t, err)

	impossibleTmp := setupImpossibleTmp(t, workDir)
	confPath := moduleFile + ":/etc/mender/"

	err = Run([]string{
		"mender-artifact", "install", 
		menderConfFile, confPath,
		"--tmp", impossibleTmp,
		"--mode", "0644",
	})

	assert.Error(t, err)
	assert.Contains(t, err.Error(), filepath.Join(workDir, "i_am_a_file"))
}

func TestTmpFlagRespectedInSSHWrite(t *testing.T) {
	workDir := t.TempDir()
	impossibleTmp := setupImpossibleTmp(t, workDir)

	err := Run([]string{
		"mender-artifact", "write", "rootfs-image",
		"-n", "test-version",
		"-t", "test-device",
		"-o", filepath.Join(workDir, "out.mender"),
		"-f", "ssh://user@localhost",
		"--tmp", impossibleTmp,
	})

	assert.Error(t, err)
	assert.Contains(t, err.Error(), filepath.Join(workDir, "i_am_a_file"))
}

func TestTmpFlagRespectedInModify(t *testing.T) {
	workDir := t.TempDir()
	artifactPath := filepath.Join(workDir, "modify.mender")
	moduleFile := filepath.Join(workDir, "module.img")

	require.NoError(t, os.WriteFile(moduleFile, []byte("fake rootfs content"), 0644))

	// Create initial artifact
	err := Run([]string{
		"mender-artifact", "write", "module-image",
		"-n", "initial-name",
		"-t", "test-device",
		"-T", "test-device-type",
		"-o", artifactPath,
	})
	require.NoError(t, err)

	impossibleTmp := setupImpossibleTmp(t, workDir)

	err = Run([]string{
		"mender-artifact", "modify",
		"-n", "updated-name",
		"--tmp", impossibleTmp,
		artifactPath,
	})

	assert.Error(t, err)
	assert.Contains(t, err.Error(), filepath.Join(workDir, "i_am_a_file"))
}
