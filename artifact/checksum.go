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
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"hash"
	"io"
	"strings"
	"syscall"

	"github.com/pkg/errors"
)

type Checksum struct {
	w io.Writer // underlying writer
	r io.Reader
	h hash.Hash
	c []byte // checksum
}

func NewWriterChecksum(w io.Writer) *Checksum {
	h := sha256.New()
	return &Checksum{
		w: io.MultiWriter(h, w),
		h: h,
	}
}

func NewReaderChecksum(r io.Reader) *Checksum {
	h := sha256.New()
	return &Checksum{
		r: io.TeeReader(r, h),
		h: h,
	}
}

func (c *Checksum) Write(p []byte) (int, error) {
	if c.w == nil {
		return 0, syscall.EBADF
	}
	return c.w.Write(p)
}

func (c *Checksum) Read(p []byte) (int, error) {
	if c.r == nil {
		return 0, syscall.EBADF
	}
	return c.r.Read(p)
}

func (c *Checksum) Checksum() []byte {
	sum := c.h.Sum(nil)
	checksum := make([]byte, hex.EncodedLen(len(sum)))
	hex.Encode(checksum, c.h.Sum(nil))
	return checksum
}

type ChecksumStore struct {
	raw  *bytes.Buffer
	sums map[string]([]byte)
}

func NewChecksumStore() *ChecksumStore {
	return &ChecksumStore{
		sums: make(map[string]([]byte), 1),
		raw:  bytes.NewBuffer(nil),
	}
}

func (c *ChecksumStore) Add(file string, sum []byte) error {
	c.sums[file] = sum
	_, err := c.raw.WriteString(fmt.Sprintf("%s  %s\n", sum, file))
	return err
}

func (c *ChecksumStore) Get(file string) ([]byte, error) {
	sum, ok := c.sums[file]
	if !ok {
		return nil, errors.Errorf("checksum: can not find chacksum for: '%s'", file)
	}
	return sum, nil
}

func (c *ChecksumStore) GetRaw() []byte {
	return c.raw.Bytes()
}

func (c *ChecksumStore) ReadRaw(data []byte) error {
	c.raw = bytes.NewBuffer(data)
	for {
		line, err := c.raw.ReadString('\n')
		if err == io.EOF {
			break
		} else if err != nil {
			return errors.Wrap(err, "checksum: can not read raw")
		}
		if err := c.readChecksums(line); err != nil {
			return err
		}
	}
	return nil
}

func (c *ChecksumStore) readChecksums(line string) error {
	l := strings.Split(strings.TrimSpace(line), "  ")
	if len(l) != 2 {
		return errors.Errorf("checksum: malformed checksum line: '%s'", line)
	}
	// add element to map
	c.sums[l[1]] = []byte(l[0])
	return nil
}
