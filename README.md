# Android Keylogger + Go Dashboard

A lightweight Go server + Android accessibility-service keylogger for capturing and monitoring keystrokes, clicks, and focus events from multiple devices in real time — with a live web dashboard featuring per-device tabs and online/offline status.

> **Security note:** This is designed for **authorized use on systems you own or have permission to test**. Running it against systems without explicit written permission is illegal. Only ever use it on infrastructure you control.

---

## ✨ Features

- **Accessibility-based capture** — records text input, taps/clicks, and focus events
- **Device identification** — every event is tagged with Build.MODEL **(e.g., "Moto G", "Redmi Note 8")**, so multiple phones never mix on the dashboard
- **Batched uploads** — buffers events and flushes every 1s / 5 events (configurable) instead of one request per keystroke
- **Offline resilience** — events are always saved locally on-device **(keylog_YYYYMMDD.txt);** pull them later with adb pull even if the server was unreachable
- **Heartbeat** — pings the server every 20s so online/offline status is accurate even when idle
- **Auto-enable via root** — on rooted devices, the service enables itself using su on first launch

  ---

## ⚙️ Setup
```bash
# 1. Clone & enter
git clone https://github.com/Linuxndroid/Smart-Keylogger
cd Smart-Keylogger

# 2. Run The Server
go run server.go
# Listening on :8080

# 3. Change The Ip on Keylogger.java
Line 29 Replace to your server Ip (192.168.122.11:8080)

# 4. Building Apk
./gradlew assembleDebug or Use AIDE Apk
```
  
---
## 🙏 Credits
- Original keylogger concept: https://github.com/bshu2/Android-Keylogger (CS460 project)

 ---

## 🆕 What's New (v2)

- **🗂 Per-device tabs** — multi-phone support; each Build.MODEL gets its own tab
- **🟢 Online/offline** indicators with last-seen time, driven by 20s heartbeats
- ⚡**Batched uploads** — 1s / 5-event flush instead of per-keystroke POSTs
- **💾 Local device logging** — daily files, adb pull-able even when offline
- **🛡 Panic-safe server** — accepts JSON arrays, JSON objects, and legacy pipe format
- **🖥 Live dashboard** — 2s polling, flashing new rows, search & action filters

  ---
  
<p align="center">Made with ❤️ By <a href="https://www.youtube.com/channel/UC2O1Hfg-dDCbUcau5QWGcgg">Linuxndroid</a></p>
