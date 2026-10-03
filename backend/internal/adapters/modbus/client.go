// Package modbus implements a minimal Modbus TCP (MBAP) master client used by
// the modbus-gateway to poll registers and write setpoints to PLCs.
//
// Only the function codes needed for telemetry are implemented: read holding
// (0x03), read input (0x04), write single register (0x06) and write multiple
// registers (0x10). Modbus TCP carries no CRC; the MBAP header carries length
// and transaction id.
package modbus

import (
	"encoding/binary"
	"fmt"
	"io"
	"net"
	"sync"
	"time"
)

// Function codes.
const (
	fnReadHolding = 0x03
	fnReadInput   = 0x04
	fnWriteSingle = 0x06
	fnWriteMulti  = 0x10
)

// Exception codes (subset).
const (
	excIllegalFunction = 0x01
	excIllegalAddress  = 0x02
	excIllegalValue    = 0x03
	excSlaveFailure    = 0x04
	excServerBusy      = 0x06
)

// Client is a synchronous Modbus TCP master connection.
type Client struct {
	mu      sync.Mutex
	conn    net.Conn
	seq     uint16
	timeout time.Duration
}

// Dial opens a Modbus TCP connection to host:port.
func Dial(host string, port int, timeout time.Duration) (*Client, error) {
	conn, err := net.DialTimeout("tcp", fmt.Sprintf("%s:%d", host, port), timeout)
	if err != nil {
		return nil, err
	}
	if timeout <= 0 {
		timeout = 5 * time.Second
	}
	c := &Client{conn: conn, timeout: timeout}
	_ = conn.SetDeadline(time.Now().Add(timeout))
	return c, nil
}

// Close terminates the connection.
func (c *Client) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.conn == nil {
		return nil
	}
	err := c.conn.Close()
	c.conn = nil
	return err
}

// ReadRegisters reads count registers starting at addr using function fn
// (0x03 holding or 0x04 input). Returns the register values big-endian.
func (c *Client) ReadRegisters(unit byte, fn byte, addr uint16, count uint16) ([]uint16, error) {
	if fn != fnReadHolding && fn != fnReadInput {
		return nil, fmt.Errorf("unsupported read function 0x%02x", fn)
	}
	var pdu []byte
	if fn == fnReadHolding {
		pdu = buildReadPDU(fnReadHolding, addr, count)
	} else {
		pdu = buildReadPDU(fnReadInput, addr, count)
	}
	resp, err := c.transact(unit, pdu)
	if err != nil {
		return nil, err
	}
	if len(resp) < 2 || int(resp[1]) != len(resp)-2 {
		return nil, fmt.Errorf("bad read response length")
	}
	payload := resp[2:]
	out := make([]uint16, 0, len(payload)/2)
	for i := 0; i+1 < len(payload); i += 2 {
		out = append(out, binary.BigEndian.Uint16(payload[i:i+2]))
	}
	if len(out) != int(count) {
		return nil, fmt.Errorf("expected %d registers, got %d", count, len(out))
	}
	return out, nil
}

// WriteSingle writes one holding register (function 0x06).
func (c *Client) WriteSingle(unit byte, addr uint16, value uint16) error {
	pdu := make([]byte, 5)
	pdu[0] = fnWriteSingle
	binary.BigEndian.PutUint16(pdu[1:3], addr)
	binary.BigEndian.PutUint16(pdu[3:5], value)
	resp, err := c.transact(unit, pdu)
	if err != nil {
		return err
	}
	if len(resp) != 5 || resp[0] != fnWriteSingle {
		return fmt.Errorf("unexpected write-single response")
	}
	return nil
}

// WriteMultiple writes consecutive holding registers (function 0x10).
func (c *Client) WriteMultiple(unit byte, addr uint16, values []uint16) error {
	pdu := make([]byte, 6+len(values)*2)
	pdu[0] = fnWriteMulti
	binary.BigEndian.PutUint16(pdu[1:3], addr)
	binary.BigEndian.PutUint16(pdu[3:5], uint16(len(values)))
	pdu[5] = byte(len(values) * 2)
	for i, v := range values {
		binary.BigEndian.PutUint16(pdu[6+i*2:8+i*2], v)
	}
	resp, err := c.transact(unit, pdu)
	if err != nil {
		return err
	}
	if len(resp) != 5 || resp[0] != fnWriteMulti {
		return fmt.Errorf("unexpected write-multiple response")
	}
	return nil
}

func buildReadPDU(fn byte, addr, count uint16) []byte {
	pdu := make([]byte, 5)
	pdu[0] = fn
	binary.BigEndian.PutUint16(pdu[1:3], addr)
	binary.BigEndian.PutUint16(pdu[3:5], count)
	return pdu
}

// transact sends a request and awaits the matching response, protected by the
// per-connection lock. In case of a Modbus exception the response error is
// returned to the caller.
func (c *Client) transact(unit byte, pdu []byte) ([]byte, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.conn == nil {
		return nil, fmt.Errorf("connection closed")
	}
	c.seq++
	seq := c.seq
	header := make([]byte, 7)
	binary.BigEndian.PutUint16(header[0:2], seq)
	binary.BigEndian.PutUint16(header[4:6], uint16(1+len(pdu)))
	header[6] = unit

	_ = c.conn.SetDeadline(time.Now().Add(c.timeout))
	msg := append(header, pdu...)
	if _, err := c.conn.Write(msg); err != nil {
		return nil, err
	}

	respHdr := make([]byte, 7)
	_ = c.conn.SetDeadline(time.Now().Add(c.timeout))
	if _, err := io.ReadFull(c.conn, respHdr); err != nil {
		return nil, err
	}
	if got := binary.BigEndian.Uint16(respHdr[0:2]); got != seq {
		return nil, fmt.Errorf("transaction mismatch: sent %d got %d", seq, got)
	}
	length := int(binary.BigEndian.Uint16(respHdr[4:6]))
	if length < 2 || length > 254 {
		return nil, fmt.Errorf("bad frame length %d", length)
	}
	body := make([]byte, length-1) // minus unit id
	if _, err := io.ReadFull(c.conn, body); err != nil {
		return nil, err
	}
	fn := body[0]
	if fn&0x80 != 0 {
		if len(body) > 1 {
			return nil, fmt.Errorf("modbus exception 0x%02x (%s)", body[1], excName(body[1]))
		}
		return nil, fmt.Errorf("modbus exception")
	}
	if fn != pdu[0] {
		return nil, fmt.Errorf("function mismatch: sent 0x%02x got 0x%02x", pdu[0], fn)
	}
	return body, nil
}

func excName(code byte) string {
	switch code {
	case excIllegalFunction:
		return "illegal function"
	case excIllegalAddress:
		return "illegal data address"
	case excIllegalValue:
		return "illegal data value"
	case excSlaveFailure:
		return "slave device failure"
	case excServerBusy:
		return "server busy"
	default:
		return fmt.Sprintf("code 0x%02x", code)
	}
}
