#!/usr/bin/env python3
"""Minimal Modbus TCP slave simulator for local demo/testing of modbus-gateway.

Usage: python3 scripts/modbus_slave_sim.py [port] [unit]
Serves a register bank on localhost:port. Registers respond per the schema in
deploy/modbus.example.json:
  holding 0   int16  temperature = 25.5  (raw 255)
  input  2   float32 pressure  = 1.02
  holding 10  bool   motor     = 1
  holding 20  uint32 wh        = 0x00040001
Writes (0x06/0x10) update the bank so setpoints can be exercised.
"""
import socket
import struct
import sys
import threading

PORT = int(sys.argv[1]) if len(sys.argv) > 1 else 1502
UNIT = int(sys.argv[2]) if len(sys.argv) > 2 else 1

bank = {0: 255, 10: 1, 20: 0x0004, 21: 0x0001}  # holding registers
inputs = {2: 0x3F82, 3: 0x8F5C}                  # float32 1.02 -> two input regs


def pdu_error(fn):
    return bytes([fn | 0x80, 0x02])


def handle_read(regs, fn, addr, count):
    words = []
    for i in range(count):
        words.append(regs.get(addr + i, 0))
    body = bytes([fn, count * 2]) + b"".join(struct.pack(">H", w) for w in words)
    return body


def handle(unit, req):
    fn = req[0]
    if unit != UNIT and unit != 0:
        return pdu_error(fn)
    if fn == 0x03:  # holding
        addr, count = struct.unpack(">HH", req[1:5])
        return handle_read(bank, fn, addr, count)
    if fn == 0x04:  # input
        addr, count = struct.unpack(">HH", req[1:5])
        return handle_read(inputs, fn, addr, count)
    if fn == 0x06:  # write single
        addr, val = struct.unpack(">HH", req[1:5])
        bank[addr] = val
        return bytes([fn]) + struct.pack(">HH", addr, val)
    if fn == 0x10:  # write multiple
        addr, count = struct.unpack(">HH", req[1:5])
        for i in range(count):
            bank[addr + i] = struct.unpack(">H", req[6 + i * 2 : 8 + i * 2])[0]
        return bytes([fn]) + struct.pack(">HH", addr, count)
    return pdu_error(fn)


def serve(conn):
    try:
        while True:
            hdr = conn.recv(7)
            if len(hdr) < 7:
                return
            txn = hdr[0:2]
            length = struct.unpack(">H", hdr[4:6])[0]
            unit = hdr[6]
            body = b""
            while len(body) < length - 1:
                chunk = conn.recv(length - 1 - len(body))
                if not chunk:
                    return
                body += chunk
            print("req:", txn.hex(), "unit", unit, "pdu", body.hex(), flush=True)
            resp = handle(unit, body)
            out = txn + b"\x00\x00" + struct.pack(">H", len(resp) + 1) + bytes([unit]) + resp
            conn.sendall(out)
    finally:
        conn.close()


def main():
    srv = socket.socket(socket.AF_INET, socket.SOCK_STREAM)
    srv.setsockopt(socket.SOL_SOCKET, socket.SO_REUSEADDR, 1)
    srv.bind(("127.0.0.1", PORT))
    srv.listen(16)
    print(f"modbus slave sim on 127.0.0.1:{PORT} unit {UNIT}", flush=True)
    while True:
        conn, _ = srv.accept()
        threading.Thread(target=serve, args=(conn,), daemon=True).start()


if __name__ == "__main__":
    main()