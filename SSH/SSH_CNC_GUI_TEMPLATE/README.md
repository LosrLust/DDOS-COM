# SSH CNC GUI Template

Mouse-driven (clickable) SSH CNC starter template. All interaction is through clickable buttons — no commands to type.

## Requirements

- Go 1.21 or newer
- An SSH client with **xterm mouse support** (PuTTY, xterm, GNOME Terminal, Konsole, iTerm2, Windows Terminal — pretty much all of them)
- Ubuntu / Debian / any Linux (also works on macOS / Windows)

## Install Go on Ubuntu

```bash
sudo apt update
sudo apt install -y golang-go
```

## Run

```bash
git clone https://github.com/LosrLust/DDOS-COM.git
cd DDOS-COM/SSH/SSH_CNC_GUI_TEMPLATE
go mod tidy
go run .
```

The server will:
- Listen on `0.0.0.0:2222`
- Auto-generate `host.key` on first run
- Auto-create `cnc.db` and seed default user `admin` / `admin`

## Connect

```bash
ssh -p 2222 anyone@<server-ip>
```

**PuTTY users:** Settings → Window → Selection → Mouse usage → set to "xterm" (or just leave default — mouse clicks work out of the box on most setups).

## How it works

- **Login screen** — click the Username or Password field, type, then click `[ LOGIN ]` (or press Enter / Tab between fields)
- **Main menu** — click any button to open that screen
- **Sub-screens** — click `[ BACK ]` to return to the menu
- **Logout** — click `[ LOGOUT ]` on the main menu

## Built-in Screens

| Button     | Description                | Permission |
|------------|----------------------------|------------|
| PROFILE    | View your account info     | all        |
| SESSIONS   | View online users          | all        |
| CREDITS    | Show template credits      | all        |
| PASSWD     | Change your password       | all        |
| USERS      | View user list             | admin      |
| LOGOUT     | Disconnect                 | all        |

## Configuration

Edit `config.toml`:

```toml
[ssh]
host = "0.0.0.0"
port = 2222
key_file = "host.key"

[database]
path = "cnc.db"
```

## Adding a new screen

Open `core/gui.go` and add a new function using the `screenFrame` helper:

```go
func showMyScreen(s *Session) {
    screenFrame(s, "My Screen", func(b *strings.Builder) {
        line(b, 12, "Hello", s.User.Username)
        line(b, 13, "Some Info", "Goes here")
    })
}
```

Then add an entry to the `items` slice in `mainMenu`:

```go
{"MY SCREEN", showMyScreen, false},
```

## Project Structure

```
SSH_CNC_GUI_TEMPLATE/
├── main.go          entry point + config loader
├── config.toml      ssh / database config
├── go.mod / go.sum
└── core/
    ├── theme.go     colors + ASCII banner
    ├── database.go  sqlite + users + auth
    ├── server.go    ssh server + session management
    └── gui.go       mouse handling + buttons + screens
```

## How the mouse works (technical)

The server sends `\033[?1000h` to enable X10 mouse tracking. The client then sends `ESC [ M <button> <col> <row>` whenever a mouse button is pressed, where each coordinate is offset by 32. The `readEvent` function in `gui.go` parses these into clicks and dispatches them to button hitboxes.

## Credits

- Template: cnc ssh gui
- Developer: Lust
