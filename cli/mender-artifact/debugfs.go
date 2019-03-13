// Copyright 2019 Northern.tech AS
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
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"syscall"

	"github.com/mendersoftware/mender-artifact/utils"
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
			if ws.ExitStatus() == 8 {
				return errors.New("mender-artifact can only modify ext4 payloads")
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
	cmd := exec.Command(utils.GetBinaryPath("debugfs"), "-R", dumpCmd, image)
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

func debugfsReplaceFile(imageFile, newFile, image string) (err error) {
	// First check that the path exists. (cd path)
	cmd := fmt.Sprintf("cd %s\nclose", filepath.Dir(imageFile))
	if _, err = executeCommand(cmd, image); err != nil {
		return err
	}
	// remove command can fail, if the file does not already exist, but this is not critical
	// so simply ignore the error.
	cmd = fmt.Sprintf("rm %s\nclose", imageFile)
	executeCommand(cmd, image)
	// Write to the partition
	cmd = fmt.Sprintf("cd %s\nwrite %s %s\nclose", filepath.Dir(imageFile), newFile, filepath.Base(imageFile))
	_, err = executeCommand(cmd, image)
	return err
}

func debugfsRemoveFileOrDir(imageFile, image string, recursive bool) (err error) {
	// Check that the file or directory exists.
	cmd := fmt.Sprintf("cd %s", filepath.Dir(imageFile))
	if _, err = executeCommand(cmd, image); err != nil {
		return err
	}
	isDir := filepath.Dir(imageFile) == filepath.Clean(imageFile)
	if isDir {
		return debugfsRemoveDir(imageFile, image, recursive)
	}
	cmd = fmt.Sprintf("%s %s\nclose", "rm", imageFile)
	_, err = executeCommand(cmd, image)
	return err
}

func debugfsRemoveDir(imageFile, image string, recursive bool) (err error) {
	// Remove the `/` suffix if present, as debugfs does not play nice with it.
	imageFile = strings.TrimRight(imageFile, "/")
	cmd := fmt.Sprintf("rmdir %s", imageFile)
	if _, err = executeCommand(cmd, image); err != nil {
		if recursive && strings.Contains(err.Error(), "directory not empty") {
			// Recurse and remove all files in the directory.
			cmd = fmt.Sprintf("ls %s", imageFile)
			buf, err := executeCommand(cmd, image)
			if err != nil {
				return errors.Wrap(err, "debugfsRemoveDir")
			}
			regexp := regexp.MustCompile(`[0-9]+ +\([0-9]+\) +((.\w+.)+)`)
			for _, filename := range regexp.FindAllStringSubmatch(buf.String(), -1) {
				err = debugfsRemoveFileOrDir(filepath.Join(imageFile, string(filename[1])), image, recursive)
				if err != nil {
					return errors.Wrap(err, "debugfsRemoveDir")
				}
			}
			// Now that all the subdirs and files should be removed, try and remove the directory
			// once more.
			return debugfsRemoveDir(imageFile, image, false)
		}
		return errors.Wrap(err, "debugfsRemoveDir")
	}
	return nil
}

// executeCommand takes a command string and passes it on to debugfs on the image given.
func executeCommand(cmdstr, image string) (stdout *bytes.Buffer, err error) {
	scr, err := ioutil.TempFile("", "mender-debugfs")
	if err != nil {
		return nil, errors.Wrap(err, "debugfs: create sync script file")
	}
	defer os.Remove(scr.Name())
	defer scr.Close()

	err = scr.Chmod(0755)
	if err != nil {
		return nil, errors.Wrap(err, "debugfs: set script file exec flag")
	}

	if _, err = scr.WriteString(cmdstr); err != nil {
		return nil, errors.Wrap(err, "debugfs: store sync script file")
	}

	if err = scr.Close(); err != nil {
		return nil, errors.Wrap(err, "debugfs: close sync script")
	}
	cmd := exec.Command(utils.GetBinaryPath("debugfs"), "-w", "-f", scr.Name(), image)
	cmd.Env = []string{"DEBUGFS_PAGER='cat'"}
	errbuf := bytes.NewBuffer(nil)
	stdout = bytes.NewBuffer(nil)
	cmd.Stderr = errbuf
	cmd.Stdout = stdout
	if err = cmd.Run(); err != nil {
		return nil, errors.Wrap(err, "debugfs: run debugfs script")
	}

	// Remove the debugfs standard message
	reg := regexp.MustCompile("^debugfs.*\\n")
	loc := reg.FindIndex(errbuf.Bytes())
	if len(loc) == 0 {
		return nil, fmt.Errorf("debugfs: prompt not found in: %s", errbuf.String())
	}
	errbufstr := string(errbuf.Bytes()[loc[1]-1:]) // Strip debugfs: (version) ...
	if len(errbufstr) > 1 {
		return nil, fmt.Errorf("debugfs: error running command: %q, err: %s", cmdstr, errbufstr)
	}

	return stdout, nil
}
