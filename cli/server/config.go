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
	"encoding/json"
	"os"
	"path/filepath"

	"github.com/pkg/errors"
)

const configFileName = ".mender-clirc"

// PersistServerToConfig ensures the JSON config file at path has a "server"
// key set to server. It creates the file (and any missing parent directories)
// if it does not exist. If the file already contains a "server" key, it is
// left untouched and false is returned. Otherwise the key is written and true
// is returned.
func PersistServerToConfig(path string, server string) (bool, error) {
	var content map[string]interface{}

	data, err := os.ReadFile(path)
	if err != nil {
		if !os.IsNotExist(err) {
			return false, errors.Wrapf(err, "failed to read config file %s", path)
		}
		if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
			return false, errors.Wrapf(err, "failed to create directory for %s", path)
		}
		content = map[string]interface{}{}
	} else {
		if err := json.Unmarshal(data, &content); err != nil {
			return false, errors.Wrapf(err, "failed to parse config file %s as JSON", path)
		}
		if _, exists := content["server"]; exists {
			return false, nil
		}
	}

	content["server"] = server

	out, err := json.MarshalIndent(content, "", "  ")
	if err != nil {
		return false, errors.Wrap(err, "failed to encode config file content")
	}
	out = append(out, '\n')

	if err := os.WriteFile(path, out, 0600); err != nil {
		return false, errors.Wrapf(err, "failed to write config file %s", path)
	}

	return true, nil
}

// ResolveConfigFilePath returns the config file path to persist the server
// to. If configFileUsed is non-empty (e.g. from a framework like viper), it
// is returned as-is. Otherwise falls back to $HOME/.mender-clirc.
func ResolveConfigFilePath(configFileUsed string) string {
	if configFileUsed != "" {
		return configFileUsed
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return configFileName
	}

	return filepath.Join(home, configFileName)
}

// LoadConfigServer reads the config file and returns the server URL if set.
// Returns empty string if the file doesn't exist or has no "server" key.
func LoadConfigServer(configFileUsed string) string {
	path := ResolveConfigFilePath(configFileUsed)

	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}

	var content map[string]interface{}
	if err := json.Unmarshal(data, &content); err != nil {
		return ""
	}

	if s, ok := content["server"].(string); ok {
		return s
	}
	return ""
}
