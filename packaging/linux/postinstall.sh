#!/bin/sh
# Apply the udev rule right away so the deck works without replugging.
if command -v udevadm >/dev/null 2>&1; then
  udevadm control --reload-rules || true
  udevadm trigger --subsystem-match=hidraw || true
fi
