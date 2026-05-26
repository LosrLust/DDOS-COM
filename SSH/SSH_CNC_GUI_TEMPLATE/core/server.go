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
	"sync"

	"golang.org/x/crypto/ssh"
)

type Session struct {
	ID     int
	User   *User
	Conn   io.ReadWriteCloser
	Reader *EventReader
	IP     string
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

	er := &EventReader{r: ch}
	u := loginScreen(ch, er)
	if u == nil {
		return
	}

	s := newSession(u, ch, ip)
	s.Reader = er
	defer removeSession(s.ID)

	log.Printf("[+] %s @ %s (#%d)", u.Username, ip, s.ID)
	mainMenu(s)
	log.Printf("[-] %s disconnected (#%d)", u.Username, s.ID)
}
