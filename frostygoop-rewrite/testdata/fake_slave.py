#!/usr/bin/env python3
"""
Minimal Modbus TCP fake slave for malware-analysis labs.

Implemented function codes:
  0x03 - Read Holding Registers
  0x06 - Write Single Holding Register
  0x10 - Write Multiple Holding Registers

This server keeps an in-memory holding-register table and logs every request.
It is intended for isolated lab use only.
"""

from __future__ import annotations

import argparse
import socketserver
import struct
import sys
import threading
import time
from datetime import datetime
from typing import Iterable


FC_READ_HOLDING = 0x03
FC_WRITE_SINGLE = 0x06
FC_WRITE_MULTIPLE = 0x10

EX_ILLEGAL_FUNCTION = 0x01
EX_ILLEGAL_ADDRESS = 0x02
EX_ILLEGAL_VALUE = 0x03


def now() -> str:
    return datetime.now().strftime("%Y-%m-%d %H:%M:%S.%f")[:-3]


def hexdump(data: bytes) -> str:
    return " ".join(f"{byte:02x}" for byte in data)


def recv_exact(sock, size: int) -> bytes:
    chunks = bytearray()
    while len(chunks) < size:
        chunk = sock.recv(size - len(chunks))
        if not chunk:
            raise EOFError
        chunks.extend(chunk)
    return bytes(chunks)


def u16(data: bytes, offset: int) -> int:
    return struct.unpack_from(">H", data, offset)[0]


def pack_registers(values: Iterable[int]) -> bytes:
    return b"".join(struct.pack(">H", value & 0xFFFF) for value in values)


def modbus_exception(fc: int, code: int) -> bytes:
    return bytes([fc | 0x80, code])


class ModbusState:
    def __init__(self, register_count: int) -> None:
        self.registers = [(i & 0xFFFF) for i in range(register_count)]

    def valid_range(self, address: int, count: int) -> bool:
        return 0 <= address and 0 <= count and address + count <= len(self.registers)


class ModbusRequestHandler(socketserver.BaseRequestHandler):
    def handle(self) -> None:
        peer = f"{self.client_address[0]}:{self.client_address[1]}"
        self.server.log(f"connect peer={peer}")
        try:
            while True:
                header = recv_exact(self.request, 7)
                transaction_id, protocol_id, length, unit_id = struct.unpack(">HHHB", header)
                if length < 2:
                    self.server.log(
                        f"peer={peer} malformed_mbap tid={transaction_id} length={length}"
                    )
                    return

                pdu = recv_exact(self.request, length - 1)
                if not pdu:
                    return

                response_pdu = self.dispatch(peer, transaction_id, protocol_id, unit_id, pdu)
                response = struct.pack(
                    ">HHHB",
                    transaction_id,
                    protocol_id,
                    len(response_pdu) + 1,
                    unit_id,
                ) + response_pdu
                if self.server.delay_ms:
                    time.sleep(self.server.delay_ms / 1000.0)
                self.request.sendall(response)
                if self.server.note_request_should_stop():
                    self.server.log("max_requests reached; stopping")
                    self.server.stop_async()
                    return
        except EOFError:
            self.server.log(f"disconnect peer={peer}")
        except ConnectionResetError:
            self.server.log(f"reset peer={peer}")

    def dispatch(
        self,
        peer: str,
        transaction_id: int,
        protocol_id: int,
        unit_id: int,
        pdu: bytes,
    ) -> bytes:
        fc = pdu[0]
        state: ModbusState = self.server.state

        if fc == FC_READ_HOLDING:
            if len(pdu) != 5:
                self.server.log_request(peer, transaction_id, unit_id, fc, pdu, "bad_length")
                return modbus_exception(fc, EX_ILLEGAL_VALUE)
            address = u16(pdu, 1)
            count = u16(pdu, 3)
            if count < 1 or count > 125:
                self.server.log_request(
                    peer, transaction_id, unit_id, fc, pdu, f"bad_count count={count}"
                )
                return modbus_exception(fc, EX_ILLEGAL_VALUE)
            if not state.valid_range(address, count):
                self.server.log_request(
                    peer,
                    transaction_id,
                    unit_id,
                    fc,
                    pdu,
                    f"bad_address address={address} count={count}",
                )
                return modbus_exception(fc, EX_ILLEGAL_ADDRESS)

            values = state.registers[address : address + count]
            preview = values[:8]
            suffix = "..." if len(values) > len(preview) else ""
            self.server.log_request(
                peer,
                transaction_id,
                unit_id,
                fc,
                pdu,
                f"read_holding address={address} count={count} values={preview}{suffix}",
            )
            return bytes([fc, count * 2]) + pack_registers(values)

        if fc == FC_WRITE_SINGLE:
            if len(pdu) != 5:
                self.server.log_request(peer, transaction_id, unit_id, fc, pdu, "bad_length")
                return modbus_exception(fc, EX_ILLEGAL_VALUE)
            address = u16(pdu, 1)
            value = u16(pdu, 3)
            if not state.valid_range(address, 1):
                self.server.log_request(
                    peer,
                    transaction_id,
                    unit_id,
                    fc,
                    pdu,
                    f"bad_address address={address}",
                )
                return modbus_exception(fc, EX_ILLEGAL_ADDRESS)

            old_value = state.registers[address]
            state.registers[address] = value
            self.server.log_request(
                peer,
                transaction_id,
                unit_id,
                fc,
                pdu,
                f"write_single address={address} old={old_value} new={value}",
            )
            return pdu

        if fc == FC_WRITE_MULTIPLE:
            if len(pdu) < 6:
                self.server.log_request(peer, transaction_id, unit_id, fc, pdu, "bad_length")
                return modbus_exception(fc, EX_ILLEGAL_VALUE)
            address = u16(pdu, 1)
            count = u16(pdu, 3)
            byte_count = pdu[5]
            expected_len = 6 + byte_count
            if len(pdu) != expected_len or byte_count != count * 2:
                self.server.log_request(
                    peer,
                    transaction_id,
                    unit_id,
                    fc,
                    pdu,
                    f"bad_payload count={count} byte_count={byte_count}",
                )
                return modbus_exception(fc, EX_ILLEGAL_VALUE)
            if count < 1 or count > 123:
                self.server.log_request(
                    peer, transaction_id, unit_id, fc, pdu, f"bad_count count={count}"
                )
                return modbus_exception(fc, EX_ILLEGAL_VALUE)
            if not state.valid_range(address, count):
                self.server.log_request(
                    peer,
                    transaction_id,
                    unit_id,
                    fc,
                    pdu,
                    f"bad_address address={address} count={count}",
                )
                return modbus_exception(fc, EX_ILLEGAL_ADDRESS)

            values = [u16(pdu, 6 + i * 2) for i in range(count)]
            old_values = state.registers[address : address + count]
            state.registers[address : address + count] = values
            preview = values[:8]
            old_preview = old_values[:8]
            suffix = "..." if len(values) > len(preview) else ""
            self.server.log_request(
                peer,
                transaction_id,
                unit_id,
                fc,
                pdu,
                (
                    f"write_multiple address={address} count={count} "
                    f"old={old_preview}{suffix} new={preview}{suffix}"
                ),
            )
            return struct.pack(">BHH", fc, address, count)

        self.server.log_request(
            peer, transaction_id, unit_id, fc, pdu, "illegal_function"
        )
        return modbus_exception(fc, EX_ILLEGAL_FUNCTION)


class ThreadedModbusServer(socketserver.ThreadingTCPServer):
    allow_reuse_address = True

    def __init__(
        self,
        server_address,
        handler_class,
        state: ModbusState,
        raw: bool,
        delay_ms: int,
        max_requests: int,
    ) -> None:
        super().__init__(server_address, handler_class)
        self.state = state
        self.raw = raw
        self.delay_ms = delay_ms
        self.max_requests = max_requests
        self.request_count = 0
        self.request_count_lock = threading.Lock()

    def note_request_should_stop(self) -> bool:
        if self.max_requests <= 0:
            return False
        with self.request_count_lock:
            self.request_count += 1
            return self.request_count >= self.max_requests

    def stop_async(self) -> None:
        threading.Thread(target=self.shutdown, daemon=True).start()

    def log(self, message: str) -> None:
        print(f"[{now()}] {message}", flush=True)

    def log_request(
        self,
        peer: str,
        transaction_id: int,
        unit_id: int,
        fc: int,
        pdu: bytes,
        detail: str,
    ) -> None:
        raw = f" pdu={hexdump(pdu)}" if self.raw else ""
        self.log(
            f"peer={peer} tid={transaction_id} unit={unit_id} fc=0x{fc:02x} "
            f"{detail}{raw}"
        )


def parse_args(argv: list[str]) -> argparse.Namespace:
    parser = argparse.ArgumentParser(
        description="Fake Modbus TCP slave for isolated malware-analysis labs."
    )
    parser.add_argument("--host", default="127.0.0.1", help="bind address")
    parser.add_argument("--port", type=int, default=502, help="bind port")
    parser.add_argument(
        "--registers",
        type=int,
        default=10000,
        help="number of in-memory holding registers",
    )
    parser.add_argument(
        "--delay-ms",
        type=int,
        default=0,
        help="delay each Modbus response by this many milliseconds",
    )
    parser.add_argument(
        "--max-requests",
        type=int,
        default=0,
        help="stop after this many Modbus requests; 0 means unlimited",
    )
    parser.add_argument("--raw", action="store_true", help="also log raw PDU bytes")
    return parser.parse_args(argv)


def main(argv: list[str]) -> int:
    args = parse_args(argv)
    if args.registers <= 0 or args.registers > 65536:
        print("--registers must be between 1 and 65536", file=sys.stderr)
        return 2
    if args.delay_ms < 0:
        print("--delay-ms must be non-negative", file=sys.stderr)
        return 2
    if args.max_requests < 0:
        print("--max-requests must be non-negative", file=sys.stderr)
        return 2

    state = ModbusState(args.registers)
    with ThreadedModbusServer(
        (args.host, args.port),
        ModbusRequestHandler,
        state,
        args.raw,
        args.delay_ms,
        args.max_requests,
    ) as server:
        server.log(
            (
                f"listening host={args.host} port={args.port} "
                f"registers={args.registers} delay_ms={args.delay_ms} "
                f"max_requests={args.max_requests}"
            )
        )
        try:
            server.serve_forever()
        except KeyboardInterrupt:
            server.log("stopping")
    return 0


if __name__ == "__main__":
    raise SystemExit(main(sys.argv[1:]))
