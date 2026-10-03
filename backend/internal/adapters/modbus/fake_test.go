package modbus

import (
	"encoding/binary"
	"io"
	"net"
	"testing"
)

// fakeModbusServer is a minimal in-process Modbus TCP slave for tests. It
// responds to each request with the bytes returned by reply (which should be
// unit+fn+data, i.e. without the MBAP header). Returning nil closes the
// connection and ends the session.
type fakeModbusServer struct {
	host string
	port int
	ln   net.Listener
}

func newFakeModbusServer(t *testing.T, reply func(req []byte) []byte, _ int) *fakeModbusServer {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	s := &fakeModbusServer{host: "127.0.0.1", port: ln.Addr().(*net.TCPAddr).Port, ln: ln}
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			go func(c net.Conn) {
				defer c.Close()
				for {
					hdr := make([]byte, 7)
					if _, err := io.ReadFull(c, hdr); err != nil {
						return
					}
					length := int(binary.BigEndian.Uint16(hdr[4:6]))
					if length < 1 {
						return
					}
					body := make([]byte, length-1)
					if _, err := io.ReadFull(c, body); err != nil {
						return
					}
					req := append(hdr, body...)
					resp := reply(req)
					if resp == nil {
						return
					}
					out := make([]byte, 7, 7+len(resp))
					copy(out, hdr)                                            // echo txn + proto
					binary.BigEndian.PutUint16(out[4:6], uint16(len(resp)+1)) // unit + PDU
					out = append(out, resp...)
					if _, err := c.Write(out); err != nil {
						return
					}
				}
			}(conn)
		}
	}()
	return s
}

func (s *fakeModbusServer) Close() { _ = s.ln.Close() }
