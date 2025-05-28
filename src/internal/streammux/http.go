package streammux

import (
	"bufio"
	"context"
	"fmt"
	"net"
	"net/http"
	"sync/atomic"
	"time"
)

type RoundTripper struct {
	mux     *Mux
	lastTag uint32
}

func NewRoundTripper(m *Mux) *RoundTripper {
	return &RoundTripper{mux: m}
}

func (r *RoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	tag := atomic.AddUint32(&r.lastTag, 1)
	stream, err := r.mux.Open(tag)
	if err != nil {
		return nil, err
	}
	defer stream.Close()
	if err := req.Write(stream); err != nil {
		return nil, err
	}
	br := bufio.NewReader(stream)
	return http.ReadResponse(br, req)
}

var _ net.Listener = &Listener{}

type Listener struct {
	Context context.Context
	Mux     *Mux
}

func (l *Listener) Accept() (net.Conn, error) {
	stream, err := l.Mux.Accept(l.Context)
	if err != nil {
		return nil, err
	}
	return &Conn{stream: stream}, nil
}

func (l *Listener) Addr() net.Addr {
	return nil
}

func (l *Listener) Close() error {
	return nil
}

var _ net.Conn = &Conn{}

type Conn struct {
	stream *Stream
}

func (c *Conn) Read(p []byte) (n int, err error) {
	return c.stream.Read(p)
}

func (c *Conn) Write(p []byte) (n int, err error) {
	return c.stream.Write(p)
}

func (c *Conn) Close() error {
	return c.stream.Close()
}

func (c *Conn) LocalAddr() net.Addr {
	return nil
}

func (c *Conn) RemoteAddr() net.Addr {
	return nil
}

func (c *Conn) SetDeadline(t time.Time) error {
	return fmt.Errorf("not implemented")
}

func (c *Conn) SetReadDeadline(t time.Time) error {
	return fmt.Errorf("not implemented")
}

func (c *Conn) SetWriteDeadline(t time.Time) error {
	return fmt.Errorf("not implemented")
}
