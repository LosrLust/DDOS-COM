package core

import (
	"fmt"
	"io"
	"log"
	"net"
	"strings"
	"sync"
)

type Session struct {
	ID   int
	User *User
	Conn net.Conn
	IP   string
}

var (
	sessionMu sync.RWMutex
	sessions  = map[int]*Session{}
	sessionID int
)

func newSession(u *User, c net.Conn, ip string) *Session {
	sessionMu.Lock()
	defer sessionMu.Unlock()
	sessionID++
	s := &Session{ID: sessionID, User: u, Conn: c, IP: ip}
	sessions[s.ID] = s
	return s
}

func removeSession(id int) {
	sessionMu.Lock()
	defer sessionMu.Unlock()
	delete(sessions, id)
}

func SessionCount() int {
	sessionMu.RLock()
	defer sessionMu.RUnlock()
	return len(sessions)
}

func AllSessions() []*Session {
	sessionMu.RLock()
	defer sessionMu.RUnlock()
	out := make([]*Session, 0, len(sessions))
	for _, s := range sessions {
		out = append(out, s)
	}
	return out
}

const (
	iac  = 255
	dont = 254
	do   = 253
	wont = 252
	will = 251
	sb   = 250
	se   = 240
	echo = 1
	sga  = 3
)

func negotiate(c net.Conn) {
	c.Write([]byte{iac, will, echo, iac, will, sga, iac, dont, echo})
}

func StartTelnet(host string, port int) error {
	addr := fmt.Sprintf("%s:%d", host, port)
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return err
	}
	log.Printf("[TELNET] Listening on %s", addr)

	for {
		c, err := ln.Accept()
		if err != nil {
			log.Printf("[TELNET] accept: %s", err)
			continue
		}
		go runSession(c)
	}
}

func runSession(c net.Conn) {
	defer c.Close()
	defer func() { _ = recover() }()

	negotiate(c)

	ip := c.RemoteAddr().String()
	u, err := login(c)
	if err != nil || u == nil {
		return
	}

	s := newSession(u, c, ip)
	defer removeSession(s.ID)

	log.Printf("[+] %s @ %s (#%d)", u.Username, ip, s.ID)

	drawHome(c, u)
	prompt(s)

	log.Printf("[-] %s disconnected (#%d)", u.Username, s.ID)
}

func drawBanner(w io.Writer) {
	for _, line := range banner {
		fmt.Fprintf(w, "%s%s%s\r\n", Cyan, line, Reset)
	}
}

func drawLogin(w io.Writer) {
	w.Write([]byte(Clear))
	fmt.Fprintln(w)
	drawBanner(w)
	fmt.Fprintln(w)
	fmt.Fprintf(w, "  %s%s┌─ Authentication ──────────────────────────────────────┐%s\r\n", DGray, Bold, Reset)
	fmt.Fprintf(w, "  %s│%s  Please log in to continue.                            %s│%s\r\n", DGray, Gray, DGray, Reset)
	fmt.Fprintf(w, "  %s└────────────────────────────────────────────────────────┘%s\r\n", DGray, Reset)
	fmt.Fprintln(w)
}

func drawHome(w io.Writer, u *User) {
	w.Write([]byte(Clear))
	fmt.Fprintln(w)
	drawBanner(w)
	fmt.Fprintln(w)
	fmt.Fprintf(w, "  %s%s● Connected as %s%s%s  %s(%s)%s\r\n",
		Green, Bold, White, u.Username, Reset, Gray, u.Rank, Reset)
	fmt.Fprintf(w, "  %sType %s%shelp%s%s for a list of commands.%s\r\n",
		Gray, Cyan, Bold, Reset, Gray, Reset)
	fmt.Fprintln(w)
	fmt.Fprintf(w, "  %s%s%s\r\n", DGray, strings.Repeat("─", 60), Reset)
	fmt.Fprintln(w)
}

func login(c net.Conn) (*User, error) {
	drawLogin(c)

	for i := 0; i < 3; i++ {
		fmt.Fprintf(c, "  %s%s>%s %sUsername:%s ", Cyan, Bold, Reset, White, Reset)
		name, err := readLine(c, false)
		if err != nil {
			return nil, err
		}
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}

		fmt.Fprintf(c, "  %s%s>%s %sPassword:%s ", Cyan, Bold, Reset, White, Reset)
		pass, err := readLine(c, true)
		if err != nil {
			return nil, err
		}
		pass = strings.TrimSpace(pass)

		u, err := Auth(name, pass)
		if err != nil {
			fmt.Fprintf(c, "\r\n  %s%s✗%s %sInvalid credentials.%s\r\n\r\n",
				Red, Bold, Reset, Red, Reset)
			continue
		}
		fmt.Fprintf(c, "\r\n  %s%s✓%s %sLogin successful.%s\r\n",
			Green, Bold, Reset, Green, Reset)
		return u, nil
	}

	fmt.Fprintf(c, "\r\n  %s%s✗ Too many failed attempts.%s\r\n", Red, Bold, Reset)
	return nil, fmt.Errorf("auth failed")
}

func prompt(s *Session) {
	for {
		fmt.Fprintf(s.Conn, "%s%s%s%s@%s%scnc%s%s>%s ",
			Magenta, Bold, s.User.Username, Reset,
			Cyan, Bold, Reset, White, Reset)

		line, err := readLine(s.Conn, false)
		if err != nil {
			return
		}
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		args := strings.Fields(line)
		if strings.EqualFold(args[0], "clear") || strings.EqualFold(args[0], "cls") {
			drawHome(s.Conn, s.User)
			continue
		}

		if err := Run(s, args); err != nil {
			switch err {
			case ErrUnknownCmd:
				fmt.Fprintf(s.Conn, "  %s%s✗%s %sUnknown command:%s %s%s%s\r\n",
					Red, Bold, Reset, Gray, Reset, White, args[0], Reset)
			case ErrNoPerm:
				fmt.Fprintf(s.Conn, "  %s%s✗%s %sPermission denied.%s\r\n",
					Red, Bold, Reset, Red, Reset)
			default:
				fmt.Fprintf(s.Conn, "  %s%s✗%s %sError:%s %s\r\n",
					Red, Bold, Reset, Red, Reset, err)
			}
		}
	}
}

func readByte(c net.Conn) (byte, error) {
	one := make([]byte, 1)
	for {
		n, err := c.Read(one)
		if err != nil {
			return 0, err
		}
		if n == 0 {
			continue
		}
		b := one[0]
		if b != iac {
			return b, nil
		}
		if _, err := c.Read(one); err != nil {
			return 0, err
		}
		cmd := one[0]
		switch cmd {
		case will, wont, do, dont:
			if _, err := c.Read(one); err != nil {
				return 0, err
			}
		case sb:
			for {
				if _, err := c.Read(one); err != nil {
					return 0, err
				}
				if one[0] == iac {
					if _, err := c.Read(one); err != nil {
						return 0, err
					}
					if one[0] == se {
						break
					}
				}
			}
		}
	}
}

func readLine(c net.Conn, mask bool) (string, error) {
	var buf []byte
	for {
		b, err := readByte(c)
		if err != nil {
			return "", err
		}
		switch {
		case b == '\r':
			c.Write([]byte("\r\n"))
			return string(buf), nil
		case b == '\n' || b == 0:
			continue
		case b == 0x7f || b == 0x08:
			if len(buf) > 0 {
				buf = buf[:len(buf)-1]
				c.Write([]byte("\b \b"))
			}
		case b == 0x03:
			c.Write([]byte("^C\r\n"))
			return "", fmt.Errorf("interrupted")
		case b == 0x04:
			if len(buf) == 0 {
				return "", io.EOF
			}
		case b >= 0x20 && b < 0x7f:
			buf = append(buf, b)
			if mask {
				c.Write([]byte("*"))
			} else {
				c.Write([]byte{b})
			}
		}
	}
}
