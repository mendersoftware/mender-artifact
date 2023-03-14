// Copyright 2021 Northern.tech AS
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
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/pkg/errors"

	"github.com/EcoG-io/mender-artifact/utils"
)

const (
	debugfsMissingErr = "The `debugfs` binary is not found on the system. The binary can" +
		" typically be installed through the `e2fsprogs` package."
)

func debugfsCopyFile(file, image string) (ret string, err error) {
	tmpDir, err := ioutil.TempDir("", "mender")
	if err != nil {
		return "", errors.Wrap(err, "debugfs: create temp directory")
	}
	defer func() {
		if err != nil {
			os.RemoveAll(tmpDir)
		}
	}()

	dumpCmd := fmt.Sprintf("dump %s %s", file,
		filepath.Join(tmpDir, filepath.Base(file)))
	bin, err := utils.GetBinaryPath("debugfs")
	if err != nil {
		return "", fmt.Errorf(debugfsMissingErr)
	}
	cmd := exec.Command(bin, "-R", dumpCmd, image)
	ep, err := cmd.StderrPipe()
	if err != nil {
		return "", errors.Wrap(err, "failed to open stderr pipe of command")
	}
	if err = cmd.Start(); err != nil {
		return "", errors.Wrap(err, "debugfs: run debugfs dump")
	}
	data, err := ioutil.ReadAll(ep)
	if err != nil {
		return "", errors.Wrap(err, "Failed to read from stderr-pipe")
	}

	if len(data) > 0 && strings.Contains(string(data), "File not found") {
		return "", fmt.Errorf("file %s not found in image", file)
	}
	if err = cmd.Wait(); err != nil {
		return "", errors.Wrap(err, "debugfs copy-file command failed")
	}

	return tmpDir, nil
}

func debugfsMakeDir(imageFile, image string) (err error) {
	_, err = debugfsExecuteCommand(fmt.Sprintf("stat %s", imageFile), image)
	// If directory already exists, just return
	if err == nil {
		return err
	}
	// Remove the `/` suffix if present, as debugfs mkdir does not play nice with it.
	imageFile = strings.TrimRight(imageFile, "/")
	// Recursively create parent directories if they do not exist
	dir, _ := filepath.Split(imageFile)
	if dir != "" {
		if err = debugfsMakeDir(dir, image); err != nil {
			return errors.Wrap(err, "debugfsMakeDir")
		}
	}
	cmd := fmt.Sprintf("mkdir %s", imageFile)
	if _, err = debugfsExecuteCommand(cmd, image); err != nil {
		return errors.Wrap(err, "debugfsMakeDir")
	}
	return nil
}

func debugfsReplaceFile(imageFile, hostFile, image string) (err error) {
	// First check that the path exists. (cd path)
	cmd := fmt.Sprintf("cd %s\nclose", filepath.Dir(imageFile))
	if _, err = debugfsExecuteCommand(cmd, image); err != nil {
		return err
	}
	// remove command can fail, if the file does not already exist, but this is not critical
	// so simply ignore the error.
	cmd = fmt.Sprintf("rm %s\nclose", imageFile)
	_, _ = debugfsExecuteCommand(cmd, image)
	// Write to the partition
	cmd = fmt.Sprintf(
		"cd %s\nwrite %s %s\nclose",
		filepath.Dir(imageFile),
		hostFile,
		filepath.Base(imageFile),
	)
	_, err = debugfsExecuteCommand(cmd, image)
	return err
}

func debugfsRemoveFileOrDir(imageFile, image string, recursive bool) (err error) {
	// Check that the file or directory exists.
	cmd := fmt.Sprintf("cd %s", filepath.Dir(imageFile))
	if _, err = debugfsExecuteCommand(cmd, image); err != nil {
		return err
	}
	isDir := filepath.Dir(imageFile) == filepath.Clean(imageFile)
	if isDir {
		return debugfsRemoveDir(imageFile, image, recursive)
	}
	cmd = fmt.Sprintf("%s %s\nclose", "rm", imageFile)
	_, err = debugfsExecuteCommand(cmd, image)
	return err
}

func debugfsRemoveDir(imageFile, image string, recursive bool) (err error) {
	// Remove the `/` suffix if present, as debugfs does not play nice with it.
	imageFile = strings.TrimRight(imageFile, "/")
	cmd := fmt.Sprintf("rmdir %s", imageFile)
	if _, err = debugfsExecuteCommand(cmd, image); err != nil {
		if recursive && strings.Contains(err.Error(), "directory not empty") {
			// Recurse and remove all files in the directory.
			cmd = fmt.Sprintf("ls %s", imageFile)
			buf, err := debugfsExecuteCommand(cmd, image)
			if err != nil {
				return errors.Wrap(err, "debugfsRemoveDir")
			}
			regexp := regexp.MustCompile(`[0-9]+ +\([0-9]+\) +((.\w+.)+)`)
			for _, filename := range regexp.FindAllStringSubmatch(buf.String(), -1) {
				err = debugfsRemoveFileOrDir(
					filepath.Join(imageFile, string(filename[1])),
					image,
					recursive,
				)
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

// debugfsExecuteCommand takes a command string and passes it on to debugfs on the image given.
func debugfsExecuteCommand(cmdstr, image string) (stdout *bytes.Buffer, err error) {
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
	bin, err := utils.GetBinaryPath("debugfs")
	if err != nil {
		return nil, fmt.Errorf(debugfsMissingErr)
	}

	cmd := exec.Command(bin, "-w", "-f", scr.Name(), image)
	cmd.Env = []string{"DEBUGFS_PAGER='cat'"}
	errbuf := bytes.NewBuffer(nil)
	stdout = bytes.NewBuffer(nil)
	cmd.Stderr = errbuf
	cmd.Stdout = stdout
	if err = cmd.Run(); err != nil {
		return nil, errors.Wrap(err, "debugfs: run debugfs script")
	}

	// Remove the debugfs standard message
	reg := regexp.MustCompile(`^debugfs.*\n`)
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
