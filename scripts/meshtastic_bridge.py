#!/usr/bin/env python3

import argparse
import sys
import time

from pubsub import pub


def open_interface(interface_kind: str, device: str):
    if interface_kind != "serial":
        raise RuntimeError(f"unsupported interface kind: {interface_kind}")

    import meshtastic.serial_interface

    return meshtastic.serial_interface.SerialInterface(device)


def close_interface(interface) -> None:
    if interface is None:
        return
    close = getattr(interface, "close", None)
    if callable(close):
        close()


def extract_text(args, kwargs):
    if "text" in kwargs and isinstance(kwargs["text"], str):
        return kwargs["text"]
    if "packet" in kwargs and isinstance(kwargs["packet"], dict):
        packet = kwargs["packet"]
        decoded = packet.get("decoded", {})
        if isinstance(decoded, dict):
            text = decoded.get("text")
            if isinstance(text, str):
                return text

    for item in args:
        if isinstance(item, str):
            return item
        if isinstance(item, dict):
            decoded = item.get("decoded", {})
            if isinstance(decoded, dict):
                text = decoded.get("text")
                if isinstance(text, str):
                    return text
    return None


def command_send(parsed):
    interface = None
    try:
        interface = open_interface(parsed.interface_kind, parsed.device)
        interface.sendText(parsed.text)
    finally:
        close_interface(interface)


def command_listen(parsed):
    received = []

    def on_receive(*args, **kwargs):
        text = extract_text(args, kwargs)
        if not text:
            return
        if parsed.prefix and not text.startswith(parsed.prefix):
            return
        received.append(text)

    pub.subscribe(on_receive, "meshtastic.receive.text")

    interface = None
    try:
        interface = open_interface(parsed.interface_kind, parsed.device)
        deadline = time.monotonic() + parsed.timeout_seconds
        while time.monotonic() < deadline and len(received) < parsed.limit:
            time.sleep(0.1)
    finally:
        close_interface(interface)

    for text in received[: parsed.limit]:
        sys.stdout.write(text + "\n")
    sys.stdout.flush()


def parse_args():
    parser = argparse.ArgumentParser(description="Meshtastic federation bridge helper")
    subparsers = parser.add_subparsers(dest="command", required=True)

    send_parser = subparsers.add_parser("send", help="send one text frame")
    send_parser.add_argument("--interface-kind", required=True)
    send_parser.add_argument("--device", required=True)
    send_parser.add_argument("--text", required=True)

    listen_parser = subparsers.add_parser("listen", help="listen for text frames")
    listen_parser.add_argument("--interface-kind", required=True)
    listen_parser.add_argument("--device", required=True)
    listen_parser.add_argument("--prefix", default="")
    listen_parser.add_argument("--limit", type=int, default=1)
    listen_parser.add_argument("--timeout-seconds", type=float, default=3.0)

    return parser.parse_args()


def main():
    parsed = parse_args()
    if parsed.command == "send":
        command_send(parsed)
        return
    if parsed.command == "listen":
        command_listen(parsed)
        return
    raise RuntimeError(f"unsupported command: {parsed.command}")


if __name__ == "__main__":
    try:
        main()
    except Exception as exc:  # pragma: no cover - helper script entrypoint
        sys.stderr.write(str(exc) + "\n")
        sys.exit(1)
