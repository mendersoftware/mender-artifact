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

package server

import (
	"fmt"
	"os"
	"os/user"
	"path/filepath"
	"strings"

	"github.com/pkg/errors"
)

const (
	DefaultServer = "https://hosted.mender.io"
	tokenFileName = "authtoken"
	tokenDirName  = "mender"
)

func tokenPath() (string, error) {
	home := os.Getenv("HOME")
	if home == "" {
		u, err := user.Current()
		if err != nil {
			return "", errors.New("cannot determine home directory")
		}
		home = u.HomeDir
	}

	cacheDir := os.Getenv("XDG_CACHE_HOME")
	if cacheDir == "" {
		cacheDir = filepath.Join(home, ".cache")
	}

	newPath := filepath.Join(cacheDir, tokenDirName, tokenFileName)

	oldPath := filepath.Join(home, ".mender", tokenFileName)
	if _, err := os.Stat(oldPath); err == nil {
		if _, err := os.Stat(newPath); os.IsNotExist(err) {
			if err := os.MkdirAll(filepath.Dir(newPath), 0700); err == nil {
				_ = os.Rename(oldPath, newPath)
			}
		}
	}

	return newPath, nil
}

func SaveToken(token string) error {
	path, err := tokenPath()
	if err != nil {
		return err
	}

	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return errors.Wrap(err, "failed to create token directory")
	}

	return os.WriteFile(path, []byte(token), 0600)
}

func LoadToken() (string, error) {
	path, err := tokenPath()
	if err != nil {
		return "", err
	}

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return "", fmt.Errorf(
				"no authentication token found; run 'mender-artifact login' first or use --token",
			)
		}
		return "", errors.Wrap(err, "failed to read auth token")
	}

	token := strings.TrimSpace(string(data))
	if token == "" {
		return "", fmt.Errorf("auth token is empty; run 'mender-artifact login' to re-authenticate")
	}

	return token, nil
}

func ResolveToken(tokenFlag string) (string, error) {
	if tokenFlag != "" {
		return tokenFlag, nil
	}
	return LoadToken()
}
