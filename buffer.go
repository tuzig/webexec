package main

import (
	"sync"
)

// Buffer is a used to represet a fixed size buffer with markers
type Buffer struct {
	markers map[int]int
	data    []byte
	end     int
	m       sync.Mutex
	size    int
}

// Buffer.NewBuffer creates and returns a new buffer of a given size
func NewBuffer(size int) *Buffer {
	return &Buffer{markers: make(map[int]int),
		data: make([]byte, size),
		size: size}
}

// Buffer.Add adds a slice of bytes to the buffer
func (buffer *Buffer) Add(b []byte) {
	for i := range b {
		buffer.data[buffer.end] = b[i]
		buffer.end++
		if buffer.end == buffer.size {
			buffer.end = 0
		}
		for k, v := range buffer.markers {
			if v == buffer.end {
				buffer.markers[k] = -1
			}
		}
	}
}

// Buffer.Mark adds a new marker in the next buffer position
func (buffer *Buffer) Mark(id int) {
	buffer.markers[id] = buffer.end
}

// Buffer.GetSinceMarker returns a byte slice with all the accumlated data
// since a given marker id and deltes the marker. If the marker is too ancient
// cycle or id is -1 then all the buffer's data is returned.
func (buffer *Buffer) GetSinceMarker(id int) []byte {
	var (
		r     []byte
		end   int
		start int
	)
	if id == -1 || buffer.markers[id] == -1 {
		// the case when the marker is lost in history, send all the buffer
		start = buffer.end
		if start == 0 {
			end = buffer.size
		} else {
			end = start - 1
		}
	} else {
		start = buffer.markers[id]
		end = buffer.end
	}
	for i := start; i != end; i++ {
		if i == buffer.size {
			i = 0
		}
		r = append(r, buffer.data[i])
	}
	// markers are for one-use - delete it
	if id != -1 {
		delete(buffer.markers, id)
	}
	return r
}
