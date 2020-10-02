// Copyright 2020 Northern.tech AS
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

// Collection of functions that help with making sure that new flags that are
// introduced will be handled by all the different sub commands.

package main

import (
	"runtime"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/urfave/cli"
)

type flagChecker struct {
	command      string
	handledFlags []string
}

func newFlagChecker(command string) *flagChecker {
	return &flagChecker{
		command: command,
	}
}

func (f *flagChecker) addFlags(handledFlags []string) {
	f.handledFlags = append(f.handledFlags, handledFlags...)
}

func (f *flagChecker) checkAllFlagsTested(t *testing.T) {
	require.NotEmpty(t, f.command, "Must specify command to check in flagChecker.")

	app := getCliContext()

	handledFlags := f.handledFlags
	availableFlags := []string{}

	for _, command := range app.Commands {
		if command.Name == f.command {
			availableFlags = getFlagsRecursive(command)
			break
		}
	}

	for _, flag := range availableFlags {
		for _, handledFlag := range handledFlags {
			if flag == handledFlag {
				goto found
			}
		}
		{
			pc, file, line, ok := runtime.Caller(1)
			require.True(t, ok)
			fn := runtime.FuncForPC(pc)
			require.NotNil(t, fn)
			t.Fail()
			t.Logf("Flag \"%s\" not handled for command \"%s\"\n"+
				"Function: %s()\n"+
				"Location: %s:%d\n"+
				"Note that this test may require all tests to run.",
				flag, f.command, fn.Name(), file, line)
		}

	found:
		// Continue
	}
}

func getFlagsRecursive(cmd cli.Command) []string {
	availableFlags := []string{}

	for _, flag := range cmd.Flags {
		names := strings.Split(flag.GetName(), ",")
		var longestName string
		longest := -1
		for _, name := range names {
			name = strings.TrimSpace(name)
			if len(name) > longest {
				longestName = name
				longest = len(name)
			}
		}
		// No augmented flags are handled right now.
		if strings.Index(longestName, "augment") < 0 {
			availableFlags = append(availableFlags, longestName)
		}
	}

	for _, subcommand := range cmd.Subcommands {
		availableFlags = append(availableFlags, getFlagsRecursive(subcommand)...)
	}

	return availableFlags
}
