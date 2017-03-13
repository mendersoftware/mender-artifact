// Copyright 2016 Mender Software AS
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

package artifact

import (
	"bufio"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/pkg/errors"
)

type Manifest struct {
	sums map[string]([]byte)

	r *bufio.Reader
	w *bufio.Writer
}

func NewWriterManifest(w io.Writer) *Manifest {
	return &Manifest{
		w:    bufio.NewWriter(w),
		sums: make(map[string]([]byte), 4),
	}
}

func NewReaderManifest(r io.Reader) *Manifest {
	return &Manifest{
		sums: make(map[string]([]byte), 4),
		r:    bufio.NewReader(r),
	}
}

func (m *Manifest) ReadAll() error {
	for {
		line, err := m.r.ReadString('\n')
		if err == io.EOF {
			break
		} else if err != nil {
			return errors.Wrap(err, "manifest: can not read")
		}
		if strings.HasPrefix(line, "Version:") ||
			strings.HasPrefix(line, "Date:") ||
			strings.HasPrefix(line, "SHA256:") {
			// we don't care about this for now
			continue
		}
		if err := m.readChecksums(line); err != nil {
			return err
		}
	}
	return nil
}

func (m *Manifest) AddChecksum(file string, sum []byte) {
	m.sums[file] = sum
}

func (m *Manifest) GetChecksum(file string) ([]byte, error) {
	sum, ok := m.sums[file]
	if !ok {
		return nil, errors.Errorf("manifest: can not find chacksum for: '%s'", file)
	}
	return sum, nil
}

func (m *Manifest) WriteAll(version string) error {
	if _, err := m.w.WriteString(
		fmt.Sprintf("Version: %s\nDate: %s\nSHA256:\n", version, time.Now())); err != nil {
		return errors.Wrap(err, "manifest: can not write manifest file")
	}
	for k, v := range m.sums {
		if _, err := m.w.WriteString(fmt.Sprintf(" %s %s\n", v, k)); err != nil {
			return errors.Wrap(err, "manifest: can not write manifest file")
		}
	}
	err := m.w.Flush()
	if err != nil {
		return errors.Wrapf(err, "manifest: can not write stream data")
	}
	return nil
}

func (m *Manifest) readChecksums(line string) error {
	l := strings.Split(strings.TrimSpace(line), " ")
	if len(l) != 2 {
		return errors.Errorf("manifest: malformed checksum line: '%s'", line)
	}
	// add element to map
	m.AddChecksum(l[1], []byte(l[0]))
	return nil
}
