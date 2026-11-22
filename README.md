# PadDeck

**PadDeck** is a small Windows app for cheap USB macro keypads with keys and rotary knobs, the kind sold on Amazon and AliExpress.
You decide what every key and knob does. Volume, screen brightness, per-app volume, media keys, shortcuts, text snippets and launching apps are built in.
PadDeck runs in the system tray and has its own settings window.

It's a single portable `.exe` with no installer, no runtime and no admin rights needed. It's written in Go using only the standard library and the Win32 API.

## Features

- **Program every key and knob.** Each knob has three actions: turn left, press and turn right.
- **Actions:**
  - Volume: up, down and mute.
  - Screen brightness: up, down and presets. Laptop screens use WMI; external monitors use DDC/CI.
  - Volume of the app in front only, e.g. a game or Spotify, without touching the rest.
  - Microphone mute.
  - Media: play/pause, next, previous and stop.
  - Keyboard shortcuts, typing text, opening apps, files, folders or URLs, and running commands.
  - Scroll, horizontal scroll, zoom and lock PC.
- **Writes the codes to the keypad itself** so it never clashes with your normal keyboard. Each key and knob position gets a unique code from **F13–F24**, optionally with Shift and/or Ctrl. Normal keyboards don't have these keys.
- **Learn mode** captures the code a key sends, and **Detect layout** works out how the pad numbers its keys.
- **On-screen display** shows volume and brightness as you turn a knob.
- **Seven color themes:** Light, Dark, Midnight blue, Forest, Lavender, Sunset and Mint.
- **Start with Windows**, pause from the tray, and single instance (a second launch brings up the open window).

## Supported keypads

| USB ID | Chip / protocol | Notes |
|---|---|---|
| `514C:8850` | CH57x, "0xFD" protocol | 16 keys (4×4) + 3 knobs. Fully tested. |
| `1189:8840`, `1189:8842` | CH57x "884x" | Same protocol as ch57x-keyboard-tool. Untested. |
| `1189:8890` | CH57x "8890" | Up to 12 keys + 3 knobs. Untested. |

Other keypads still work if you set their codes once with the maker's software.
Assign each key the F13–F24 code PadDeck shows on it, or use **Learn**.

## Download

Get `PadDeck.exe` from the [Releases](../../releases) page and put it in a permanent folder, e.g. `C:\Tools\PadDeck\`.
Windows SmartScreen may warn about an unsigned app: click **More info → Run anyway**.

## Quick start

1. Plug in the keypad and start `PadDeck.exe`.
2. Under **Settings**, check **Key rows / Columns / Knobs**. Click **Detect layout** and press the top-left key, then the top-right key.
3. Click **Write trigger codes to keypad**. It takes about 30 s; don't touch the pad meanwhile.
4. Press keys and turn knobs: they light up in the keypad picture and run their action.
5. Click any key or knob in the picture to change its label, action or trigger code. Changes save right away and don't need re-programming.
6. Tick **Start with Windows**.

Closing the window keeps PadDeck running in the tray. Click the tray icon to open it again.
To quit, right-click the tray icon → **Exit**.

### Default layout

| Knob | Turn left | Press | Turn right |
|---|---|---|---|
| 1 – Volume | volume down | mute | volume up |
| 2 – Screen brightness | darker | cycle 10/40/70/100 % | brighter |
| 3 – Focused-app volume | quieter | mute that app | louder |

The keys default to play/pause, previous, next, mic mute, screenshot, Task Manager, Explorer, show desktop, calculator, copy, paste, lock PC, undo, clipboard history, task view and settings.

## Troubleshooting

| Problem | Fix |
|---|---|
| The pad types letters (a, b, c…) | It still has its factory programming. Click **Write trigger codes to keypad**. |
| "No supported keypad found" | Your pad uses another chip. Set the F13–F24 codes once with the maker's software, then use **Learn**. |
| One key or knob position does nothing | Give it a trigger code (or click **Reset trigger codes**) and program the pad again. |
| Brightness doesn't change on an external monitor | Turn on **DDC/CI** in the monitor's own menu. **Check displays** shows what PadDeck can control. |
| Keys don't work while Task Manager or another admin window is in front | Windows blocks this for normal apps. Run PadDeck as administrator if you need it. |

Settings and log are stored in `%APPDATA%\PadDeck\` (`config.json`, `paddeck.log`).

## Building from source

Requires [Go 1.24+](https://go.dev/dl/).

```bat
build.bat
```

Or cross-compile from Linux/macOS with `./build.sh`. Manually:

```sh
GOOS=windows GOARCH=amd64 go build -trimpath -ldflags "-H windowsgui -s -w" -o PadDeck.exe .
```

Tests (key codes, layout rotation and keypad packets) run on any OS: `go test ./...`

Pushing a tag like `v1.0.0` makes GitHub Actions build `PadDeck.exe` and attach it to a release.

### Project structure

| File | Contents |
|---|---|
| `main.go` | Startup, single instance, logging |
| `settings.go`, `theme.go` | Settings window (Win32 controls, owner-drawn) and color themes |
| `gui.go` | Tray icon, on-screen display, message loop |
| `hook.go` | Low-level keyboard hook, Learn mode, dispatching |
| `actions.go` | Shortcuts, text typing, launching, media/mouse input |
| `audio.go` | Core Audio (master, mic and per-app volume) via COM |
| `brightness.go` | DDC/CI and WMI brightness |
| `hid.go`, `protocol.go` | Finding the keypad and building programming packets |
| `keys.go`, `layout.go` | Key names, trigger codes, key-number rotation (portable, tested) |
| `config.go` | Settings file, defaults, migration, autostart |
| `docs/PROTOCOL.md` | Keypad programming protocols |

## Credits

The keypad protocols are based on public research by
[kriomant/ch57x-keyboard-tool](https://github.com/kriomant/ch57x-keyboard-tool) and
[elmitash/macropad-16key-3knob](https://github.com/elmitash/macropad-16key-3knob).
No code was copied; see [docs/PROTOCOL.md](docs/PROTOCOL.md).

## License

[MIT](LICENSE). Programming a keypad writes to its memory. Use at your own risk.
