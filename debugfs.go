// Copyright 2017 Northern.tech AS
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

	"github.com/pkg/errors"
)

func debugfsCopyFile(file, image string) (string, error) {
	tmpDir, err := ioutil.TempDir("", "mender")
	if err != nil {
		return "", errors.Wrap(err, "debugfs: create temp directory")
	}

	dumpCmd := fmt.Sprintf("dump %s %s", file,
		filepath.Join(tmpDir, filepath.Base(file)))
	cmd := exec.Command("debugfs", "-R", dumpCmd, image)
	if err = cmd.Run(); err != nil {
		os.RemoveAll(tmpDir)
		return "", errors.Wrap(err, "debugfs: run debugfs dump")
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
	if err = cmd.Run(); err != nil {
		return errors.Wrap(err, "debugfs: run debugfs script")
	}

	return nil
}
