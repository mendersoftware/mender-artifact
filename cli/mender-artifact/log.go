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
	"os"

	"github.com/sirupsen/logrus"
)

// Log is a global reference to our logger.
var Log *logrus.Logger

type simpleFormatter struct {
}

func (f *simpleFormatter) Format(entry *logrus.Entry) ([]byte, error) {
	return []byte(fmt.Sprintf("%s: %s\n", entry.Level, entry.Message)), nil
}

func init() {
	// init logger
	Log = logrus.New()
	Log.Out = os.Stdout
	Log.Formatter = new(simpleFormatter)
}
