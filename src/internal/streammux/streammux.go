package streammux

import (
	"context"
	"encoding/binary"
	"fmt"
	"io"
	"sync"
)

type Mux struct {
	// mu synchronizes reads and protects the mux state
	mu      sync.Mutex
	rw      io.ReadWriter
	streams map[uint32]*Stream
	wait    chan struct{}

	// wmu synchronizes writes
	wmu sync.Mutex

	incoming chan *Stream
}

func New(rw io.ReadWriter) *Mux {
	m := &Mux{
		rw:       rw,
		streams:  make(map[uint32]*Stream),
		wait:     make(chan struct{}, 1),
		incoming: make(chan *Stream, 1),
	}
	m.wait <- struct{}{}
	return m
}

func (m *Mux) Open(tag uint32) (*Stream, error) {
	stream := m.newStream(tag)
	m.mu.Lock()
	if _, exists := m.streams[tag]; exists {
		m.mu.Unlock()
		return nil, fmt.Errorf("stream %d already exists", tag)
	} else {
		m.streams[tag] = stream
		m.mu.Unlock()
	}

	if err := m.write(tag, nil); err != nil {
		return nil, err
	}
	return stream, nil
}

func (m *Mux) Accept(ctx context.Context) (*Stream, error) {
	for {
		select {
		case stream := <-m.incoming:
			m.wait <- struct{}{}
			return stream, nil
		default:
		}

		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-m.wait:
			if err := m.processHeader(); err != nil {
				return nil, err
			}
		case stream := <-m.incoming:
			m.wait <- struct{}{}
			return stream, nil
		}
	}
}

func (m *Mux) newStream(tag uint32) *Stream {
	return &Stream{
		m:       m,
		tag:     tag,
		lengths: make(chan int, 1),
		closed:  make(chan struct{}),
	}
}

func (m *Mux) processHeader() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	nextTag, err := readUint32(m.rw)
	if err != nil {
		return err
	}
	l, err := readUint32(m.rw)
	if err != nil {
		return err
	}

	stream, exists := m.streams[nextTag]
	switch {
	case !exists && l == 0:
		stream = m.newStream(nextTag)
		m.incoming <- stream
		m.streams[nextTag] = stream
	case exists && l == 0:
		stream.Close()
		delete(m.streams, nextTag)
	case !exists:
		return fmt.Errorf("stream %d does not exist", nextTag)
	case exists:
		stream.lengths <- int(l)
	default:
		panic("unreachable")
	}
	return nil
}

func (m *Mux) write(tag uint32, p []byte) (err error) {
	m.wmu.Lock()
	defer m.wmu.Unlock()
	if err := binary.Write(m.rw, binary.LittleEndian, tag); err != nil {
		return err
	}
	if err := binary.Write(m.rw, binary.LittleEndian, uint32(len(p))); err != nil {
		return err
	}
	if _, err := m.rw.Write(p); err != nil {
		return err
	}
	return nil
}

func (m *Mux) drop(tag uint32) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	_, exists := m.streams[tag]
	delete(m.streams, tag)
	return exists
}

type Stream struct {
	m       *Mux
	tag     uint32
	lengths chan int
	closed  chan struct{}
}

func (s *Stream) Write(p []byte) (n int, err error) {
	select {
	case <-s.closed:
		return 0, io.EOF
	default:
		if err := s.m.write(s.tag, p); err != nil {
			return 0, err
		}
		return len(p), nil
	}
}

func (s *Stream) Read(p []byte) (n int, err error) {
	for {
		select {
		case <-s.closed:
			return 0, io.EOF
		default:
		}

		select {
		case <-s.closed:
			return 0, io.EOF
		case <-s.m.wait:
			if err := s.m.processHeader(); err != nil {
				return 0, err
			}
		case l := <-s.lengths:
			n, err := s.m.rw.Read(p[:l])
			if err != nil {
				return n, err
			}
			if n < l {
				s.lengths <- l - n
			} else {
				s.m.wait <- struct{}{}
			}
			return n, nil
		}
	}
}

func (s *Stream) Close() error {
	if s.m.drop(s.tag) {
		close(s.closed)
	}
	return nil
}

func readUint32(r io.Reader) (uint32, error) {
	var buf [4]byte
	if _, err := io.ReadFull(r, buf[:]); err != nil {
		return 0, err
	}
	return binary.LittleEndian.Uint32(buf[:]), nil
}
