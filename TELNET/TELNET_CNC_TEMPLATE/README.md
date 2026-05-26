# TELNET CNC Template

Minimal Telnet CNC starter template (login + database + basic commands).

## Requirements

- Go 1.21 or newer
- Ubuntu / Debian / any Linux (also works on macOS / Windows)

## Install Go on Ubuntu

```bash
sudo apt update
sudo apt install -y golang-go
```

## Run

```bash
git clone https://github.com/LosrLust/DDOS-COM.git
cd DDOS-COM/TELNET/TELNET_CNC_TEMPLATE
go mod tidy
go run .
```

The server will:
- Listen on `0.0.0.0:2323`
- Auto-create `cnc.db` (SQLite database) and seed default user `admin` / `admin`

## Connect

```bash
sudo apt install -y telnet      # if not installed
telnet <server-ip> 2323
```

Login with `admin` / `admin`.

## Default Commands

| Command    | Description                | Permission |
|------------|----------------------------|------------|
| `help`     | Show available commands    | all        |
| `clear`    | Clear the screen           | all        |
| `whoami`   | Show your user info        | all        |
| `uptime`   | Show server uptime         | all        |
| `sessions` | Show online users          | all        |
| `passwd`   | Change your password       | all        |
| `users`    | Add / delete / rank users  | admin      |
| `credits`  | Show template credits      | all        |
| `exit`     | Disconnect                 | all        |

## Configuration

Edit `config.toml`:

```toml
[telnet]
host = "0.0.0.0"
port = 2323

[database]
path = "cnc.db"
```

## Adding a new command

Open `core/commands.go` and add inside `init()`:

```go
Register(&Cmd{
    Names: []string{"mycmd"},
    Desc:  "My new command",
    Perms: []string{"admin"}, // optional, omit for everyone
    Run: func(s *Session, args []string) error {
        wf(s, "  Hello %s", s.User.Username)
        return nil
    },
})
```

## Project Structure

```
TELNET_CNC_TEMPLATE/
├── main.go          entry point + config loader
├── config.toml      telnet / database config
├── go.mod / go.sum
└── core/
    ├── theme.go     colors + ASCII banner
    ├── database.go  sqlite + users + auth
    ├── commands.go  command system + built-in commands
    └── server.go    telnet server + IAC negotiation + session + login + prompt
```

## Credits

- Template: cnc telnet
- Developer: Lust
