#!/usr/bin/env python3

import argparse
import asyncio
import sys

from meshcore import EventType, MeshCore


async def open_interface(interface_kind: str, device: str):
    if interface_kind != "serial":
        raise RuntimeError(f"unsupported interface kind: {interface_kind}")
    return await MeshCore.create_serial(device)


async def command_send(parsed):
    meshcore = await open_interface(parsed.interface_kind, parsed.device)
    try:
        result = await meshcore.commands.send_msg(parsed.destination, parsed.text)
        if result.type == EventType.ERROR:
            raise RuntimeError(str(result.payload))
    finally:
        await meshcore.disconnect()


async def command_listen(parsed):
    received = []

    async def on_message(event):
        payload = event.payload or {}
        text = payload.get("text")
        if not isinstance(text, str):
            return
        if parsed.prefix and not text.startswith(parsed.prefix):
            return
        received.append(text)

    meshcore = await open_interface(parsed.interface_kind, parsed.device)
    try:
        meshcore.subscribe(EventType.CONTACT_MSG_RECV, on_message)
        await meshcore.start_auto_message_fetching()
        deadline = asyncio.get_running_loop().time() + parsed.timeout_seconds
        while asyncio.get_running_loop().time() < deadline and len(received) < parsed.limit:
            await asyncio.sleep(0.1)
        await meshcore.stop_auto_message_fetching()
    finally:
        await meshcore.disconnect()

    for text in received[: parsed.limit]:
        sys.stdout.write(text + "\n")
    sys.stdout.flush()


def parse_args():
    parser = argparse.ArgumentParser(description="MeshCore federation bridge helper")
    subparsers = parser.add_subparsers(dest="command", required=True)

    send_parser = subparsers.add_parser("send", help="send one text frame")
    send_parser.add_argument("--interface-kind", required=True)
    send_parser.add_argument("--device", required=True)
    send_parser.add_argument("--destination", required=True)
    send_parser.add_argument("--text", required=True)

    listen_parser = subparsers.add_parser("listen", help="listen for text frames")
    listen_parser.add_argument("--interface-kind", required=True)
    listen_parser.add_argument("--device", required=True)
    listen_parser.add_argument("--prefix", default="")
    listen_parser.add_argument("--limit", type=int, default=1)
    listen_parser.add_argument("--timeout-seconds", type=float, default=3.0)

    return parser.parse_args()


async def async_main():
    parsed = parse_args()
    if parsed.command == "send":
        await command_send(parsed)
        return
    if parsed.command == "listen":
        await command_listen(parsed)
        return
    raise RuntimeError(f"unsupported command: {parsed.command}")


def main():
    asyncio.run(async_main())


if __name__ == "__main__":
    try:
        main()
    except Exception as exc:  # pragma: no cover - helper script entrypoint
        sys.stderr.write(str(exc) + "\n")
        sys.exit(1)
