// Copyright 2011-2019 Rémy Oudompheng. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package xz

/*
#cgo LDFLAGS: -llzma -pthread
#include <lzma.h>
#include <stdlib.h>
#include <stdio.h>
#include <stdint.h>
#include <stdbool.h>
int go_lzma_code(
    lzma_stream* handle,
    void* next_in,
    void* next_out,
    lzma_action action
);

int go_lzma_code_mt(
    lzma_stream* handle,
    void* next_in,
    void* next_out,
    lzma_action action
) {
    handle->next_in = next_in;
    handle->next_out = next_out;
    return lzma_code(handle, action);
}

int go_lzma_init_mt(
	lzma_stream* handle,
	uint32_t preset,
	lzma_check check,
	int32_t thread_num
) {
	lzma_mt options = {
		.flags = 0,
		.block_size = 1024 * 1024 * 8,
		.timeout = 0,
		.threads = 0,
		.preset = preset,
		.filters = NULL,
		.check = check,
	};
	options.threads = lzma_cputhreads();
	if (thread_num > 0 && thread_num <= options.threads) {
		options.threads = thread_num;
	} else if (thread_num == 0) {
		options.threads = 1;
	}
	return lzma_stream_encoder_mt(handle, &options);
}
*/
import "C"
import (
	"bytes"
	"io"
	"runtime"
	"unsafe"
)

// Compressor ...
type Compressor struct {
	handle  *C.lzma_stream
	writer  io.Writer
	buffer  []byte
	totalIn uint64
}

var _ io.WriteCloser = &Compressor{}

func allocLzmaStream(t *C.lzma_stream) *C.lzma_stream {
	return (*C.lzma_stream)(C.calloc(1, (C.size_t)(unsafe.Sizeof(*t))))
}

// NewWriter ...
func NewWriter(w io.Writer, preset Preset) (*Compressor, error) {
	enc := new(Compressor)
	// The zero lzma_stream is the same thing as LZMA_STREAM_INIT.
	enc.writer = w
	enc.buffer = make([]byte, DefaultBufsize)
	enc.handle = allocLzmaStream(enc.handle)
	enc.totalIn = 0
	threadNum := runtime.NumCPU()
	// Initialize encoder
	ret := C.go_lzma_init_mt(enc.handle, C.uint32_t(preset), C.lzma_check(CheckCRC64), C.int32_t(threadNum))
	if Errno(ret) != Ok {
		return nil, Errno(ret)
	}

	return enc, nil
}

// NewWriterCustom Initializes a XZ encoder with additional settings.
func NewWriterCustom(w io.Writer, preset Preset, check Checksum, bufsize int, maxThreadNum int) (*Compressor, error) {
	enc := new(Compressor)
	// The zero lzma_stream is the same thing as LZMA_STREAM_INIT.
	enc.writer = w
	enc.buffer = make([]byte, bufsize)
	enc.handle = allocLzmaStream(enc.handle)
	enc.totalIn = 0

	// Initialize encoder
	ret := C.go_lzma_init_mt(enc.handle, C.uint32_t(preset), C.lzma_check(check), C.int32_t(maxThreadNum))
	if Errno(ret) != Ok {
		return nil, Errno(ret)
	}

	return enc, nil
}

// Write data with underlying C lib
func (enc *Compressor) Write(in []byte) (n int, er error) {

	if len(in) <= 0 {
		return 0, nil
	}

	enc.totalIn += uint64(len(in))
	partCount := 0
	offset := 0
	enc.handle.avail_in = 0
	enc.handle.avail_out = C.size_t(len(enc.buffer))

	for {
		if enc.handle.avail_in == 0 && partCount <= len(in)/DefaultPartSize {
			offset = DefaultPartSize * partCount
			if partCount < len(in)/DefaultPartSize {
				enc.handle.avail_in = C.size_t(DefaultPartSize)
			} else {
				enc.handle.avail_in = C.size_t(len(in) - DefaultPartSize*partCount)
			}
			partCount++
		}

		ret := C.go_lzma_code_mt(
			enc.handle,
			unsafe.Pointer(&in[offset]),
			unsafe.Pointer(&enc.buffer[0]),
			C.lzma_action(Run),
		)
		switch Errno(ret) {
		case Ok:
			break
		default:
			er = Errno(ret)
			return 0, er
		}

		produced := len(enc.buffer) - int(enc.handle.avail_out)

		if produced > 0 {
			// Write back result.
			_, er = enc.writer.Write(enc.buffer[:produced])
			if er != nil {
				// Short write.
				return
			}
			enc.handle.avail_out = C.size_t(len(enc.buffer))
		}

		if enc.totalIn == uint64(enc.handle.total_in) {
			n = len(in)
			return
		}
	}
}

// Flush - just before close
func (enc *Compressor) Flush() error {
	enc.handle.avail_in = 0

	for {
		enc.handle.avail_out = C.size_t(len(enc.buffer))
		// If Flush is invoked after Write produced an error, avail_in and next_in will point to
		// the bytes previously provided to Write, which may no longer be valid.
		enc.handle.avail_in = 0

		ret := C.go_lzma_code(
			enc.handle,
			nil,
			unsafe.Pointer(&enc.buffer[0]),
			C.lzma_action(Finish),
		)

		// Write back result.
		produced := len(enc.buffer) - int(enc.handle.avail_out)
		toWrite := bytes.NewBuffer(enc.buffer[:produced])
		_, er := io.Copy(enc.writer, toWrite)
		if er != nil {
			// Short write.
			return er
		}

		if Errno(ret) == StreamEnd {
			return nil
		}
	}
}

// Close frees any resources allocated by liblzma. It does not close the
// underlying reader.
func (enc *Compressor) Close() error {
	if enc != nil {
		er := enc.Flush()
		C.lzma_end(enc.handle)
		C.free(unsafe.Pointer(enc.handle))
		enc.handle = nil
		if er != nil {
			return er
		}
	}
	return nil
}
