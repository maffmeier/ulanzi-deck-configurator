#!/usr/bin/env python3
"""Example ulanzi-deck plugin: shows the current weather of a city.

The deck sends one JSON object per line on stdin; the plugin answers with
JSON lines on stdout. Only the Python standard library is used.
"""
import json
import sys
import threading
import time
import urllib.parse
import urllib.request

REFRESH_SECONDS = 600
widgets = {}  # instance id -> city
lock = threading.Lock()


def send(message):
    with lock:
        sys.stdout.write(json.dumps(message) + "\n")
        sys.stdout.flush()


def fetch(city):
    url = "https://wttr.in/" + urllib.parse.quote(city) + "?format=%t|%C"
    with urllib.request.urlopen(url, timeout=10) as res:
        temp, condition = res.read().decode().strip().split("|", 1)
    return temp.replace("+", ""), condition


def update(instance, city):
    try:
        temp, condition = fetch(city)
        send({"update": {"instance": instance, "title": city, "lines": [temp, condition]}})
    except Exception as err:  # network errors must not stop the plugin
        send({"update": {"instance": instance, "title": city, "lines": ["?"]}})
        send({"log": f"{city}: {err}"})


def refresh_all():
    for instance, city in list(widgets.items()):
        update(instance, city)


def timer():
    while True:
        time.sleep(REFRESH_SECONDS)
        refresh_all()


threading.Thread(target=timer, daemon=True).start()
for line in sys.stdin:
    event = json.loads(line)
    if event["event"] == "widget":
        city = event["settings"].get("city") or "Berlin"
        widgets[event["instance"]] = city
        threading.Thread(target=update, args=(event["instance"], city), daemon=True).start()
    elif event["event"] == "action" and event["action"] == "refresh":
        threading.Thread(target=refresh_all, daemon=True).start()
