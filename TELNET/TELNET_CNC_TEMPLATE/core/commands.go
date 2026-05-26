package core

import (
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"
)

var (
	ErrUnknownCmd = errors.New("unknown command")
	ErrNoPerm     = errors.New("no permission")
)

type Cmd struct {
	Names []string
	Desc  string
	Perms []string
	Run   func(s *Session, args []string) error
}

func (c *Cmd) Allowed(rank string) bool {
	if len(c.Perms) == 0 {
		return true
	}
	for _, p := range c.Perms {
		if strings.EqualFold(p, rank) {
			return true
		}
	}
	return false
}

var (
	cmdMu sync.Mutex
	cmds  []*Cmd
)

func Register(c *Cmd) {
	cmdMu.Lock()
	defer cmdMu.Unlock()
	cmds = append(cmds, c)
}

func Find(name string) *Cmd {
	cmdMu.Lock()
	defer cmdMu.Unlock()
	for _, c := range cmds {
		for _, n := range c.Names {
			if strings.EqualFold(n, name) {
				return c
			}
		}
	}
	return nil
}

func AllCmds() []*Cmd {
	cmdMu.Lock()
	defer cmdMu.Unlock()
	out := make([]*Cmd, len(cmds))
	copy(out, cmds)
	return out
}

func Run(s *Session, args []string) error {
	if len(args) == 0 {
		return nil
	}
	c := Find(args[0])
	if c == nil {
		return ErrUnknownCmd
	}
	if !c.Allowed(s.User.Rank) {
		return ErrNoPerm
	}
	if c.Run == nil {
		return nil
	}
	return c.Run(s, args[1:])
}

var startTime = time.Now()

func wf(s *Session, f string, a ...any) { fmt.Fprintf(s.Conn, f+"\r\n", a...) }

func head(s *Session, title string) {
	bar := strings.Repeat("─", 56-len(title)-4)
	wf(s, "")
	wf(s, "  %s┌─ %s%s%s %s%s┐%s", DGray, Cyan, title, Reset, DGray, bar, Reset)
}

func foot(s *Session) {
	wf(s, "  %s└%s┘%s", DGray, strings.Repeat("─", 58), Reset)
	wf(s, "")
}

func kv(s *Session, k, v string) {
	wf(s, "  %s│%s  %s%-12s%s %s%s%s", DGray, Reset, Yellow, k, Reset, White, v, Reset)
}

func ok(s *Session, f string, a ...any) {
	wf(s, "  %s%s✓%s "+f+"%s", append([]any{Green, Bold, Reset}, append(a, Reset)...)...)
}

func bad(s *Session, f string, a ...any) {
	wf(s, "  %s%s✗%s %s"+f+"%s", append([]any{Red, Bold, Reset, Red}, append(a, Reset)...)...)
}

func init() {
	Register(&Cmd{
		Names: []string{"help", "?"},
		Desc:  "Show available commands",
		Run: func(s *Session, _ []string) error {
			head(s, "Commands")
			for _, c := range AllCmds() {
				if !c.Allowed(s.User.Rank) {
					continue
				}
				tag := ""
				if len(c.Perms) > 0 {
					tag = fmt.Sprintf(" %s[%s]%s", Magenta, strings.Join(c.Perms, ","), Reset)
				}
				wf(s, "  %s│%s  %s%-12s%s %s%s%s%s",
					DGray, Reset, Cyan, c.Names[0], Reset, Gray, c.Desc, Reset, tag)
			}
			foot(s)
			return nil
		},
	})

	Register(&Cmd{
		Names: []string{"clear", "cls"},
		Desc:  "Clear the screen",
		Run:   func(*Session, []string) error { return nil },
	})

	Register(&Cmd{
		Names: []string{"exit", "quit", "logout"},
		Desc:  "Disconnect from the server",
		Run: func(s *Session, _ []string) error {
			wf(s, "")
			wf(s, "  %sGoodbye, %s%s%s.%s", Gray, White, s.User.Username, Gray, Reset)
			s.Conn.Close()
			return nil
		},
	})

	Register(&Cmd{
		Names: []string{"whoami"},
		Desc:  "Show your user info",
		Run: func(s *Session, _ []string) error {
			head(s, "User Profile")
			kv(s, "Username", s.User.Username)
			kv(s, "Rank", s.User.Rank)
			kv(s, "Session", fmt.Sprintf("#%d", s.ID))
			kv(s, "IP", s.IP)
			kv(s, "Joined", s.User.CreatedAt.Format("2006-01-02 15:04"))
			foot(s)
			return nil
		},
	})

	Register(&Cmd{
		Names: []string{"uptime"},
		Desc:  "Show server uptime",
		Run: func(s *Session, _ []string) error {
			d := time.Since(startTime)
			head(s, "Server Status")
			kv(s, "Uptime", fmt.Sprintf("%dh %dm %ds", int(d.Hours()), int(d.Minutes())%60, int(d.Seconds())%60))
			kv(s, "Online", fmt.Sprintf("%d user(s)", SessionCount()))
			foot(s)
			return nil
		},
	})

	Register(&Cmd{
		Names: []string{"sessions", "who"},
		Desc:  "Show online users",
		Run: func(s *Session, _ []string) error {
			all := AllSessions()
			head(s, fmt.Sprintf("Online Sessions (%d)", len(all)))
			wf(s, "  %s│%s  %s%-5s %-15s %-20s%s", DGray, Reset, Yellow, "ID", "Username", "IP", Reset)
			for _, x := range all {
				wf(s, "  %s│%s  %s%-5d %s%-15s %s%-20s%s",
					DGray, Reset, White, x.ID, Cyan, x.User.Username, Gray, x.IP, Reset)
			}
			foot(s)
			return nil
		},
	})

	Register(&Cmd{
		Names: []string{"passwd", "password"},
		Desc:  "Change your password",
		Run: func(s *Session, args []string) error {
			if len(args) < 1 {
				wf(s, "  %sUsage:%s passwd <new_password>", Gray, Reset)
				return nil
			}
			if err := UpdatePassword(s.User.Username, args[0]); err != nil {
				bad(s, "%s", err)
				return nil
			}
			ok(s, "Password updated.")
			return nil
		},
	})

	Register(&Cmd{
		Names: []string{"users"},
		Desc:  "Manage users",
		Perms: []string{"admin"},
		Run: func(s *Session, args []string) error {
			if len(args) == 0 {
				list, err := ListUsers()
				if err != nil {
					bad(s, "%s", err)
					return nil
				}
				head(s, fmt.Sprintf("Users (%d)", len(list)))
				wf(s, "  %s│%s  %s%-5s %-15s %-10s%s", DGray, Reset, Yellow, "ID", "Username", "Rank", Reset)
				for _, u := range list {
					rc := Gray
					if u.Rank == "admin" {
						rc = Magenta
					}
					wf(s, "  %s│%s  %s%-5d %s%-15s %s%-10s%s",
						DGray, Reset, White, u.ID, Cyan, u.Username, rc, u.Rank, Reset)
				}
				foot(s)
				return nil
			}

			switch strings.ToLower(args[0]) {
			case "add", "create":
				if len(args) < 3 {
					wf(s, "  %sUsage:%s users add <username> <password> [rank]", Gray, Reset)
					return nil
				}
				rank := "user"
				if len(args) >= 4 {
					rank = args[3]
				}
				if err := CreateUser(args[1], args[2], rank); err != nil {
					bad(s, "%s", err)
					return nil
				}
				ok(s, "User '%s' created with rank '%s'.", args[1], rank)
			case "del", "delete", "remove":
				if len(args) < 2 {
					wf(s, "  %sUsage:%s users del <username>", Gray, Reset)
					return nil
				}
				if args[1] == s.User.Username {
					bad(s, "Cannot delete yourself.")
					return nil
				}
				if err := DeleteUser(args[1]); err != nil {
					bad(s, "%s", err)
					return nil
				}
				ok(s, "User '%s' deleted.", args[1])
			case "rank":
				if len(args) < 3 {
					wf(s, "  %sUsage:%s users rank <username> <rank>", Gray, Reset)
					return nil
				}
				if err := SetRank(args[1], args[2]); err != nil {
					bad(s, "%s", err)
					return nil
				}
				ok(s, "User '%s' rank set to '%s'.", args[1], args[2])
			default:
				wf(s, "  %sSubcommands:%s add, del, rank", Gray, Reset)
			}
			return nil
		},
	})

	Register(&Cmd{
		Names: []string{"credits", "about"},
		Desc:  "Show credits",
		Run: func(s *Session, _ []string) error {
			head(s, "Credits")
			kv(s, "Template", "cnc telnet")
			kv(s, "Developer", "Lust")
			foot(s)
			return nil
		},
	})
}
