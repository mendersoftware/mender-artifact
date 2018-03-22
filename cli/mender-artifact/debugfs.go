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
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/pkg/errors"
)

// From the fsck man page:
// The exit code returned by fsck is the sum of the following conditions:
//
//              0      No errors
//              1      Filesystem errors corrected
//              2      System should be rebooted
//              4      Filesystem errors left uncorrected
//              8      Operational error
//              16     Usage or syntax error
//              32     Checking canceled by user request
//              128    Shared-library error
func debugfsRunFsck(image string) error {
	cmd := exec.Command("fsck.ext4", "-a", image)
	if err := cmd.Run(); err != nil {
		// try to get the exit code
		if exitError, ok := err.(*exec.ExitError); ok {
			ws := exitError.Sys().(syscall.WaitStatus)
			if ws.ExitStatus() == 0 || ws.ExitStatus() == 1 {
				return nil
			}
			return errors.Wrap(err, "fsck error")
		}
		return errors.New("fsck returned unparsed error")
	}
	return nil
}

func debugfsCopyFile(file, image string) (string, error) {
	tmpDir, err := ioutil.TempDir("", "mender")
	if err != nil {
		return "", errors.Wrap(err, "debugfs: create temp directory")
	}

	dumpCmd := fmt.Sprintf("dump %s %s", file,
		filepath.Join(tmpDir, filepath.Base(file)))
	cmd := exec.Command("debugfs", "-R", dumpCmd, image)
	ep, err := cmd.StderrPipe()
	if err != nil {
		return "", errors.Wrap(err, "failed to open stderr pipe of command")
	}
	if err = cmd.Start(); err != nil {
		os.RemoveAll(tmpDir)
		return "", errors.Wrap(err, "debugfs: run debugfs dump")
	}
	data, err := ioutil.ReadAll(ep)
	if err != nil {
		os.RemoveAll(tmpDir)
		return "", errors.Wrap(err, "Failed to read from stderr-pipe")
	}

	if len(data) > 0 && strings.Contains(string(data), "File not found") {
		os.RemoveAll(tmpDir)
		return "", fmt.Errorf("file %s not found in image", file)
	}
	if err = cmd.Wait(); err != nil {
		os.RemoveAll(tmpDir)
		return "", errors.Wrap(err, "debugfs copy-file command failed")
	}

	return tmpDir, nil
}

func debugfsReplaceFile(imageFile, newFile, image string) error {
	scr, err := ioutil.TempFile("", "mender-debugfs")
	if err != nil {
		return errors.Wrap(err, "debugfs: create sync script file")
	}
	defer os.Remove(scr.Name())
	defer scr.Close()

	err = scr.Chmod(0755)
	if err != nil {
		return errors.Wrap(err, "debugfs: set script file exec flag")
	}

	syncScript := fmt.Sprintf("cd %s\nrm %s\nwrite %s %s\nclose",
		filepath.Dir(imageFile), filepath.Base(imageFile),
		newFile, filepath.Base(imageFile))
	if _, err = scr.WriteString(syncScript); err != nil {
		return errors.Wrap(err, "debugfs: store sync script file")
	}

	if err = scr.Close(); err != nil {
		return errors.Wrap(err, "debugfs: close sync script")
	}
	cmd := exec.Command("debugfs", "-w", "-f", scr.Name(), image)
	ep, _ := cmd.StderrPipe()
	if err = cmd.Start(); err != nil {
		return errors.Wrap(err, "debugfs: run debugfs script")
	}
	data, err := ioutil.ReadAll(ep)
	if err != nil {
		return err
	}
	if len(data) != 0 && strings.Contains(string(data), "Filesystem not open") || strings.Contains(string(data), "cd: File not found") {
		return errors.New("filesystem not open")
	}

	if err := cmd.Wait(); err != nil {
		return err
	}

	return nil
}
