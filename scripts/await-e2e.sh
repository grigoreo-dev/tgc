#!/usr/bin/env bash
# Dev-only e2e: drive the BOT side via the Telegram Bot API while `tgc` runs the
# user side (`tgc await` / `tgc send --await-reply`). Not shipped in releases.
#
# Requires:
#   TGC_BOT_TOKEN  the bot's API token (BotFather)
#   USER_CHAT_ID   the user's telegram id, as the bot sees it (the user must have
#                  pressed Start / messaged the bot at least once)
#
# Usage:
#   USER_CHAT_ID=<user-id> TGC_BOT_TOKEN=<token> ./scripts/await-e2e.sh [single|burst|delayed]
#
#   single   one message                          (basic catch)
#   burst    three quick messages                 (debounce coalescing)
#   delayed  sleep, then one message              (send --await-reply round-trip)
set -euo pipefail

: "${TGC_BOT_TOKEN:?set TGC_BOT_TOKEN (the bot API token from BotFather)}"
: "${USER_CHAT_ID:?set USER_CHAT_ID (the users telegram id, as seen by the bot)}"

api="https://api.telegram.org/bot${TGC_BOT_TOKEN}"

send() {
	curl -fsS "${api}/sendMessage" \
		-d chat_id="${USER_CHAT_ID}" \
		-d text="$1" >/dev/null
}

case "${1:-burst}" in
	single)
		send "hello from bot"
		;;
	burst)
		send "one"
		sleep 0.3
		send "two"
		sleep 0.3
		send "three"
		;;
	delayed)
		sleep 3
		send "late reply"
		;;
	*)
		echo "usage: $0 [single|burst|delayed]" >&2
		exit 2
		;;
esac

echo "bot: sent (${1:-burst})"
