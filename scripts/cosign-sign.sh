#!/usr/bin/env bash
set -uo pipefail

OUTPUT=$(cosign "$@" 2>&1)
EXIT_CODE=$?

if [ $EXIT_CODE -eq 0 ]; then
  echo "$OUTPUT"
  exit 0
fi

# Check if the failure was a Rekor transparency log conflict
if echo "$OUTPUT" | grep -qE "(createLogEntryConflict|already exists)"; then
  echo "::warning::Rekor transparency log conflict detected: $OUTPUT"

  BUNDLE_FILE=""
  for arg in "$@"; do
    if [[ "$arg" =~ ^--bundle=(.*)$ ]]; then
      BUNDLE_FILE="${BASH_REMATCH[1]}"
    fi
  done

  if [ -n "$BUNDLE_FILE" ] && [ -s "$BUNDLE_FILE" ]; then
    echo "Bundle file $BUNDLE_FILE is already present and non-empty."
    exit 0
  fi

  UUID=$(echo "$OUTPUT" | grep -oE "UUID [0-9a-f]+" | awk '{print $2}' || true)
  if [ -n "$UUID" ] && [ -n "$BUNDLE_FILE" ]; then
    echo "Retrieving Rekor log entry for UUID $UUID into $BUNDLE_FILE..."
    curl -s "https://rekor.sigstore.dev/api/v1/log/entries/${UUID}" | jq -r '.[].body' | base64 --decode > "$BUNDLE_FILE" || true
    if [ -s "$BUNDLE_FILE" ]; then
      echo "Successfully restored bundle from Rekor log entry."
      exit 0
    fi
  fi

  if [ -z "$BUNDLE_FILE" ]; then
    echo "Artifact is already recorded in transparency log. Treating as success."
    exit 0
  fi
fi

echo "$OUTPUT" >&2
exit $EXIT_CODE
