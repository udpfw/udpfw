package proto

import (
	"bytes"
	"encoding/binary"
	"fmt"
)

const sig = 0
const size = 1
const payload = 2
const reset = 3

type FSM struct {
	buffer []byte
	state  int
	size   uint32
}

var beginSig = []byte("\x00PKT")

func (f *FSM) Feed(b byte) (bool, []byte, error) {
	switch f.state {
	case reset:
		f.buffer = f.buffer[:0]
		f.size = 0
		f.state = sig
		fallthrough

	case sig:
		f.buffer = append(f.buffer, b)
		if len(f.buffer) == len(beginSig) {
			if !bytes.Equal(beginSig, f.buffer) {
				return false, nil, fmt.Errorf("invalid preamble")
			}

			f.buffer = f.buffer[:0]
			f.state = size
		}

	case size:
		f.buffer = append(f.buffer, b)
		if len(f.buffer) == 4 {
			f.size = binary.BigEndian.Uint32(f.buffer)
			f.buffer = f.buffer[:0]
			f.state = payload
		}
	case payload:
		f.buffer = append(f.buffer, b)
		if uint32(len(f.buffer)) == f.size {
			f.state = reset
			return true, f.buffer[:f.size], nil
		}
	}

	return false, nil, nil
}
