#!/usr/bin/env bash
# Turns the failures in a CI step's output into GitHub annotations, which
# show on the run's summary page (and on a phone) without opening the log.
# GitHub keeps 10 error annotations per step, so each failing test, panic
# or other problem becomes one, its message lines folded into it.
#
# Usage: scripts/ci-annotate.sh LOG
set -uo pipefail

log="${1:?usage: $0 LOG}"
[ -f "$log" ] || exit 0

awk '
function flush() {
	if (title != "" && count < 10) {
		gsub(/%/, "%25", body); gsub(/\r/, "", body); gsub(/\n/, "%0A", body)
		printf "::error title=%s::%s\n", title, substr(body, 1, 3000)
		count++
	}
	title = ""; body = ""
}
# A test that failed, with the lines it logged above its result.
/^[[:space:]]*--- FAIL: / {
	name = $3
	flush()
	title = "FAIL " name
	body = pending[name]
	next
}
/^=== (RUN|CONT|PAUSE|NAME)/ { current = $3; next }
/^[[:space:]]+[A-Za-z0-9_]+_test\.go:[0-9]+:/ {
	line = $0; sub(/^[[:space:]]+/, "", line)
	pending[current] = pending[current] line "\n"
	if (title != "") body = body line "\n"
	next
}
/^panic: / { flush(); title = "panic"; body = $0 "\n"; inpanic = 1; next }
inpanic && /\.go:[0-9]+/ { body = body $0 "\n"; if (++frames > 20) { inpanic = 0 } ; next }
/^FAIL[[:space:]]/ { flush(); next }
/^go: downloading / { next }
/gofmt needed:|^# |^go: |cannot find|undefined:|declared and not used|vet: / {
	flush(); title = "build"; body = $0 "\n"; next
}
title == "build" && /\.go(:[0-9]+:[0-9]+:|$)/ { body = body $0 "\n"; next }
END { flush() }
' "$log"
