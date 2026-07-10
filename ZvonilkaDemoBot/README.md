# ZvonilkaDemoBot

`ZvonilkaDemoBot` is a runnable Bot API compatibility demo for the Zvonilka
server. It uses only the Go standard library, long polling, and the same
Telegram-shaped JSON contract that Zvonilka exposes.

## Run

```sh
cp .env.example .env
# Put the registered bot token into .env.
set -a; . ./.env; set +a
go run .
```

The token is deliberately not stored in this repository. The production bot
is registered as `Demo_bot` (usernames are case-insensitive and are persisted
canonically by Zvonilka).

## Demo surface

Send `/start` to get the interactive menu. The bot has commands for:

- messages, reply keyboards, callbacks, editing and deletion;
- photos, documents, videos, animations, audio, voice, stickers and video notes;
- locations, venues, contacts, polls and dice;
- forwarding and copying messages;
- chat/member/file inspection;
- games and high scores;
- bot profile, commands, menu button and administrator rights;
- inline query answers;
- webhook lifecycle and long polling.

The demo replies use `parse_mode=HTML`, so bold, italic, links and edited
formatted messages are visible in clients that support message entities.

Use `/methods` for the exact method inventory exposed by the current Zvonilka
server. Media commands accept a Zvonilka/Telegram `file_id`, for example
`/media photo <file_id>`. Send a file to the chat first if you need a fresh
file ID. `/demo_all` performs safe calls and reports methods that need a
message, file ID, callback, inline query, or public webhook URL.

The upstream Bot API 9.5 additions are represented in the capability report.
Methods not implemented by the current Zvonilka server are reported as
`server capability missing` instead of being faked by the demo bot.

## Environment

- `ZVONILKA_BOT_TOKEN` is required.
- `ZVONILKA_BOT_API_BASE_URL` defaults to `https://zvonilka.geartech.club`.
- `ZVONILKA_BOT_POLL_TIMEOUT` defaults to 5 seconds because the public Caddy
  route currently closes longer HTTP requests.
- `ZVONILKA_BOT_WEBHOOK_URL` is optional; if set, `/webhook` exercises the
  webhook lifecycle against that URL.
