package core

import (
	"fmt"
	"io"
	"strings"
	"time"
)

type Button struct {
	Label string
	Row   int
	X1    int
	X2    int
	Color string
}

type Field struct {
	Row    int
	X1     int
	X2     int
	Value  []byte
	Mask   bool
	MaxLen int
}

type Event struct {
	IsMouse bool
	MX      int
	MY      int
	Key     byte
	Esc     bool
	Err     error
}

type EventReader struct {
	r       io.Reader
	pending []byte
}

func enableMouse(w io.Writer) { fmt.Fprint(w, "\033[?1000h\033[?25l") }
func disableMouse(w io.Writer) { fmt.Fprint(w, "\033[?1000l\033[?25h") }
func at(row, col int) string   { return fmt.Sprintf("\033[%d;%dH", row, col) }

func (er *EventReader) fill() error {
	buf := make([]byte, 32)
	n, err := er.r.Read(buf)
	if err != nil {
		return err
	}
	er.pending = append(er.pending, buf[:n]...)
	return nil
}

func (er *EventReader) next() Event {
	for {
		if len(er.pending) == 0 {
			if err := er.fill(); err != nil {
				return Event{Err: err}
			}
			continue
		}

		if er.pending[0] != 27 {
			b := er.pending[0]
			er.pending = er.pending[1:]
			return Event{Key: b}
		}

		if len(er.pending) >= 6 && er.pending[1] == '[' && er.pending[2] == 'M' {
			btn := er.pending[3]
			mx := int(er.pending[4]) - 32
			my := int(er.pending[5]) - 32
			er.pending = er.pending[6:]
			if btn == 32 {
				return Event{IsMouse: true, MX: mx, MY: my}
			}
			continue
		}

		if len(er.pending) >= 3 && er.pending[1] == '[' && er.pending[2] != 'M' {
			er.pending = er.pending[3:]
			continue
		}

		if len(er.pending) < 6 {
			if err := er.fill(); err != nil {
				return Event{Err: err}
			}
			continue
		}

		er.pending = er.pending[1:]
		return Event{Esc: true}
	}
}

func hit(ev Event, b Button) bool {
	return ev.IsMouse && ev.MY == b.Row && ev.MX >= b.X1 && ev.MX <= b.X2
}

func hitField(ev Event, f Field) bool {
	return ev.IsMouse && ev.MY == f.Row && ev.MX >= f.X1 && ev.MX <= f.X2
}

func drawButton(w io.Writer, b Button, hovered bool) {
	width := b.X2 - b.X1 + 1
	pad := (width - len(b.Label)) / 2
	if pad < 0 {
		pad = 0
	}
	label := strings.Repeat(" ", pad) + b.Label + strings.Repeat(" ", width-pad-len(b.Label))
	style := "\033[48;5;236m" + b.Color
	if hovered {
		style = "\033[48;5;15m\033[38;5;16m\033[1m"
	}
	fmt.Fprintf(w, "%s%s%s%s", at(b.Row, b.X1), style, label, Reset)
}

func drawField(w io.Writer, f Field, focused bool) {
	width := f.X2 - f.X1 + 1
	val := string(f.Value)
	if f.Mask {
		val = strings.Repeat("*", len(f.Value))
	}
	if len(val) > width {
		val = val[len(val)-width:]
	}
	bg := "\033[48;5;236m"
	border := DGray
	if focused {
		bg = "\033[48;5;235m"
		border = Cyan
	}
	fmt.Fprintf(w, "%s%s%s%s%-*s%s",
		at(f.Row, f.X1-1), border, "│", bg+White, width, val, Reset)
	fmt.Fprintf(w, "%s%s│%s", at(f.Row, f.X2+1), border, Reset)
}

func drawBanner(w io.Writer, row int) {
	for i, line := range banner {
		fmt.Fprintf(w, "%s%s%s%s", at(row+i, 1), Cyan, line, Reset)
	}
}

func drawTopBar(w io.Writer, title string) {
	fmt.Fprintf(w, "%s%s%s %s %s%s",
		at(1, 1), DGray+"═══[ ", Cyan+Bold, title, Reset+DGray,
		strings.Repeat("═", 60-len(title))+Reset)
}

func loginScreen(rw io.ReadWriteCloser, er *EventReader) *User {
	enableMouse(rw)
	defer disableMouse(rw)

	uname := Field{Row: 13, X1: 24, X2: 53, MaxLen: 30}
	pass := Field{Row: 15, X1: 24, X2: 53, MaxLen: 30, Mask: true}
	loginBtn := Button{Label: "LOGIN", Row: 18, X1: 24, X2: 35, Color: Green}
	quitBtn := Button{Label: "QUIT", Row: 18, X1: 42, X2: 53, Color: Red}

	focus := 0
	msg := ""

	render := func() {
		var b strings.Builder
		b.WriteString(Clear)
		drawBanner(&b, 2)
		fmt.Fprintf(&b, "%s%s%s%s",
			at(10, 16), DGray, "─── Authentication ──────────────────", Reset)

		fmt.Fprintf(&b, "%s%sUsername:%s", at(13, 14), White, Reset)
		fmt.Fprintf(&b, "%s%sPassword:%s", at(15, 14), White, Reset)
		drawField(&b, uname, focus == 0)
		drawField(&b, pass, focus == 1)

		drawButton(&b, loginBtn, false)
		drawButton(&b, quitBtn, false)

		if msg != "" {
			fmt.Fprintf(&b, "%s%s%s%s", at(20, 24), Red, msg, Reset)
		}
		fmt.Fprintf(&b, "%s%sClick a field or use Tab to switch. Press Enter or click LOGIN.%s",
			at(22, 8), Gray, Reset)

		rw.Write([]byte(b.String()))
	}

	tryLogin := func() *User {
		u, err := Auth(strings.TrimSpace(string(uname.Value)), string(pass.Value))
		if err != nil {
			msg = "✗ Invalid credentials."
			pass.Value = pass.Value[:0]
			return nil
		}
		fmt.Fprintf(rw, "%s%s%s✓ Login successful.%s", at(20, 24), Green, Bold, Reset)
		time.Sleep(400 * time.Millisecond)
		return u
	}

	for {
		render()

		ev := er.next()
		if ev.Err != nil {
			return nil
		}

		if ev.IsMouse {
			switch {
			case hitField(ev, uname):
				focus = 0
			case hitField(ev, pass):
				focus = 1
			case hit(ev, loginBtn):
				if u := tryLogin(); u != nil {
					return u
				}
			case hit(ev, quitBtn):
				return nil
			}
			continue
		}

		switch ev.Key {
		case 9:
			focus = 1 - focus
		case 13, 10:
			if u := tryLogin(); u != nil {
				return u
			}
		case 127, 8:
			if focus == 0 && len(uname.Value) > 0 {
				uname.Value = uname.Value[:len(uname.Value)-1]
			} else if focus == 1 && len(pass.Value) > 0 {
				pass.Value = pass.Value[:len(pass.Value)-1]
			}
		case 3, 4:
			return nil
		default:
			if ev.Key >= 32 && ev.Key < 127 {
				if focus == 0 && len(uname.Value) < uname.MaxLen {
					uname.Value = append(uname.Value, ev.Key)
				} else if focus == 1 && len(pass.Value) < pass.MaxLen {
					pass.Value = append(pass.Value, ev.Key)
				}
			}
		}
		if ev.Esc {
			return nil
		}
	}
}

func mainMenu(s *Session) {
	enableMouse(s.Conn)
	defer disableMouse(s.Conn)

	type item struct {
		Label string
		Run   func(*Session)
		Admin bool
	}
	items := []item{
		{"PROFILE", showProfile, false},
		{"SESSIONS", showSessions, false},
		{"CREDITS", showCredits, false},
		{"ADMIN", showAdmin, true},
		{"PASSWD", showPasswd, false},
		{"LOGOUT", nil, false},
	}

	var buttons []Button
	var actions []func(*Session)
	row := 12
	col := 8
	for _, it := range items {
		if it.Admin && s.User.Rank != "admin" {
			continue
		}
		buttons = append(buttons, Button{
			Label: it.Label, Row: row, X1: col, X2: col + 19, Color: Cyan,
		})
		actions = append(actions, it.Run)
		col += 22
		if col > 70 {
			col = 8
			row += 3
		}
	}

	render := func() {
		var b strings.Builder
		b.WriteString(Clear)
		drawBanner(&b, 2)
		fmt.Fprintf(&b, "%s%s● Connected as %s%s%s  %s(%s)%s",
			at(10, 4), Green, White, s.User.Username, Reset, Gray, s.User.Rank, Reset)
		fmt.Fprintf(&b, "%s%sClick a button below to continue.%s",
			at(11, 4), Gray, Reset)
		for _, btn := range buttons {
			drawButton(&b, btn, false)
		}
		fmt.Fprintf(&b, "%s%sSession #%d  ·  %d online  ·  %s%s",
			at(23, 4), DGray, s.ID, SessionCount(), s.IP, Reset)
		s.Conn.Write([]byte(b.String()))
	}

	for {
		render()
		ev := s.Reader.next()
		if ev.Err != nil {
			return
		}
		if ev.IsMouse {
			for i, btn := range buttons {
				if hit(ev, btn) {
					if btn.Label == "LOGOUT" {
						s.Conn.Write([]byte(Clear + "\r\n  Goodbye.\r\n"))
						return
					}
					if actions[i] != nil {
						actions[i](s)
					}
					break
				}
			}
		}
		if ev.Esc || ev.Key == 'q' {
			return
		}
	}
}

func screenFrame(s *Session, title string, body func(b *strings.Builder)) {
	enableMouse(s.Conn)
	back := Button{Label: "BACK", Row: 22, X1: 8, X2: 19, Color: Yellow}

	for {
		var b strings.Builder
		b.WriteString(Clear)
		drawBanner(&b, 2)
		fmt.Fprintf(&b, "%s%s%s─── %s%s%s %s───────────────────────────────────%s",
			at(10, 4), DGray, Bold, Cyan, title, Reset, DGray, Reset)
		body(&b)
		drawButton(&b, back, false)
		s.Conn.Write([]byte(b.String()))

		ev := s.Reader.next()
		if ev.Err != nil {
			return
		}
		if (ev.IsMouse && hit(ev, back)) || ev.Esc || ev.Key == 'b' || ev.Key == 13 {
			return
		}
	}
}

func line(b *strings.Builder, row int, k, v string) {
	fmt.Fprintf(b, "%s%s%-12s%s %s%s%s", at(row, 8), Yellow, k, Reset, White, v, Reset)
}

func showProfile(s *Session) {
	screenFrame(s, "User Profile", func(b *strings.Builder) {
		line(b, 12, "Username", s.User.Username)
		line(b, 13, "Rank", s.User.Rank)
		line(b, 14, "Session", fmt.Sprintf("#%d", s.ID))
		line(b, 15, "IP", s.IP)
		line(b, 16, "Joined", s.User.CreatedAt.Format("2006-01-02 15:04"))
	})
}

func showSessions(s *Session) {
	screenFrame(s, "Online Sessions", func(b *strings.Builder) {
		all := AllSessions()
		fmt.Fprintf(b, "%s%s%-5s %-15s %-22s%s",
			at(12, 8), Yellow, "ID", "Username", "IP", Reset)
		row := 13
		for _, x := range all {
			fmt.Fprintf(b, "%s%s%-5d %s%-15s %s%-22s%s",
				at(row, 8), White, x.ID, Cyan, x.User.Username, Gray, x.IP, Reset)
			row++
			if row > 20 {
				break
			}
		}
	})
}

func showCredits(s *Session) {
	screenFrame(s, "Credits", func(b *strings.Builder) {
		line(b, 12, "Template", "cnc ssh gui")
		line(b, 13, "Developer", "Lust")
		line(b, 14, "UI", "Mouse-driven SSH terminal")
	})
}

func showUsers(s *Session) {
	screenFrame(s, "Users (admin)", func(b *strings.Builder) {
		users, err := ListUsers()
		if err != nil {
			fmt.Fprintf(b, "%s%sError: %s%s", at(12, 8), Red, err, Reset)
			return
		}
		fmt.Fprintf(b, "%s%s%-5s %-15s %-10s%s",
			at(12, 8), Yellow, "ID", "Username", "Rank", Reset)
		row := 13
		for _, u := range users {
			rc := Gray
			if u.Rank == "admin" {
				rc = Magenta
			}
			fmt.Fprintf(b, "%s%s%-5d %s%-15s %s%-10s%s",
				at(row, 8), White, u.ID, Cyan, u.Username, rc, u.Rank, Reset)
			row++
			if row > 20 {
				break
			}
		}
	})
}

func showAdmin(s *Session) {
	enableMouse(s.Conn)
	defer disableMouse(s.Conn)

	sel := 0
	msg := ""

	userField := Field{Row: 15, X1: 24, X2: 53, MaxLen: 30}
	passField := Field{Row: 17, X1: 24, X2: 53, MaxLen: 30, Mask: true}
	rankField := Field{Row: 19, X1: 24, X2: 53, MaxLen: 16}

	createBtn := Button{Label: "CREATE", Row: 21, X1: 24, X2: 35, Color: Green}
	deleteBtn := Button{Label: "DELETE", Row: 21, X1: 38, X2: 49, Color: Red}
	promoteBtn := Button{Label: "RANK", Row: 21, X1: 52, X2: 63, Color: Cyan}
	listBtn := Button{Label: "LIST", Row: 21, X1: 66, X2: 77, Color: Yellow}
	backBtn := Button{Label: "BACK", Row: 23, X1: 24, X2: 35, Color: Yellow}

	render := func() {
		var b strings.Builder
		b.WriteString(Clear)
		drawBanner(&b, 2)
		fmt.Fprintf(&b, "%s%s%s─── %sAdmin Panel%s %s──────────────────────────────────%s",
			at(10, 4), DGray, Bold, Cyan, Reset, DGray, Reset)
		fmt.Fprintf(&b, "%s%sManage users: create / delete / set rank.%s", at(12, 8), Gray, Reset)

		fmt.Fprintf(&b, "%s%sUsername:%s", at(15, 10), White, Reset)
		fmt.Fprintf(&b, "%s%sPassword:%s", at(17, 10), White, Reset)
		fmt.Fprintf(&b, "%s%sRank:%s", at(19, 10), White, Reset)

		drawField(&b, userField, sel == 0)
		drawField(&b, passField, sel == 1)
		drawField(&b, rankField, sel == 2)

		drawButton(&b, createBtn, false)
		drawButton(&b, deleteBtn, false)
		drawButton(&b, promoteBtn, false)
		drawButton(&b, listBtn, false)
		drawButton(&b, backBtn, false)

		if msg != "" {
			fmt.Fprintf(&b, "%s%s", at(24, 8), msg)
		}

		s.Conn.Write([]byte(b.String()))
	}

	setDefaults := func() {
		if len(rankField.Value) == 0 {
			rankField.Value = []byte("user")
		}
	}

	for {
		render()
		ev := s.Reader.next()
		if ev.Err != nil {
			return
		}

		if ev.IsMouse {
			switch {
			case hitField(ev, userField):
				sel = 0
			case hitField(ev, passField):
				sel = 1
			case hitField(ev, rankField):
				sel = 2
			case hit(ev, createBtn):
				setDefaults()
				un := strings.TrimSpace(string(userField.Value))
				pw := string(passField.Value)
				rk := strings.TrimSpace(string(rankField.Value))
				if un == "" || pw == "" {
					msg = Red + "✗ username/password required." + Reset
					continue
				}
				if rk == "" {
					rk = "user"
				}
				if err := CreateUser(un, pw, rk); err != nil {
					msg = Red + "✗ " + err.Error() + Reset
				} else {
					msg = Green + "✓ user created." + Reset
					userField.Value = userField.Value[:0]
					passField.Value = passField.Value[:0]
					rankField.Value = []byte("user")
				}
			case hit(ev, deleteBtn):
				un := strings.TrimSpace(string(userField.Value))
				if un == "" {
					msg = Red + "✗ username required for delete." + Reset
					continue
				}
				if un == s.User.Username {
					msg = Red + "✗ cannot delete yourself." + Reset
					continue
				}
				if err := DeleteUser(un); err != nil {
					msg = Red + "✗ " + err.Error() + Reset
				} else {
					msg = Green + "✓ user deleted." + Reset
					userField.Value = userField.Value[:0]
				}
			case hit(ev, promoteBtn):
				un := strings.TrimSpace(string(userField.Value))
				rk := strings.TrimSpace(string(rankField.Value))
				if un == "" || rk == "" {
					msg = Red + "✗ username/rank required." + Reset
					continue
				}
				if err := SetRank(un, rk); err != nil {
					msg = Red + "✗ " + err.Error() + Reset
				} else {
					msg = Green + "✓ rank updated." + Reset
				}
			case hit(ev, listBtn):
				showUsers(s)
			case hit(ev, backBtn):
				return
			}
			continue
		}

		switch ev.Key {
		case 9:
			sel = (sel + 1) % 3
		case 127, 8:
			switch sel {
			case 0:
				if len(userField.Value) > 0 {
					userField.Value = userField.Value[:len(userField.Value)-1]
				}
			case 1:
				if len(passField.Value) > 0 {
					passField.Value = passField.Value[:len(passField.Value)-1]
				}
			case 2:
				if len(rankField.Value) > 0 {
					rankField.Value = rankField.Value[:len(rankField.Value)-1]
				}
			}
		case 13, 10:
			setDefaults()
			un := strings.TrimSpace(string(userField.Value))
			pw := string(passField.Value)
			rk := strings.TrimSpace(string(rankField.Value))
			if un == "" || pw == "" {
				msg = Red + "✗ username/password required." + Reset
				continue
			}
			if rk == "" {
				rk = "user"
			}
			if err := CreateUser(un, pw, rk); err != nil {
				msg = Red + "✗ " + err.Error() + Reset
			} else {
				msg = Green + "✓ user created." + Reset
				userField.Value = userField.Value[:0]
				passField.Value = passField.Value[:0]
				rankField.Value = []byte("user")
			}
		case 3, 4:
			return
		default:
			if ev.Key >= 32 && ev.Key < 127 {
				switch sel {
				case 0:
					if len(userField.Value) < userField.MaxLen {
						userField.Value = append(userField.Value, ev.Key)
					}
				case 1:
					if len(passField.Value) < passField.MaxLen {
						passField.Value = append(passField.Value, ev.Key)
					}
				case 2:
					if len(rankField.Value) < rankField.MaxLen {
						rankField.Value = append(rankField.Value, ev.Key)
					}
				}
			}
		}
		if ev.Esc {
			return
		}
	}
}

func showPasswd(s *Session) {
	enableMouse(s.Conn)
	field := Field{Row: 14, X1: 24, X2: 53, MaxLen: 30, Mask: true}
	save := Button{Label: "SAVE", Row: 17, X1: 24, X2: 35, Color: Green}
	back := Button{Label: "BACK", Row: 17, X1: 42, X2: 53, Color: Yellow}
	msg := ""

	for {
		var b strings.Builder
		b.WriteString(Clear)
		drawBanner(&b, 2)
		fmt.Fprintf(&b, "%s%s%s─── %sChange Password%s %s───────────────────────────────%s",
			at(10, 4), DGray, Bold, Cyan, Reset, DGray, Reset)
		fmt.Fprintf(&b, "%s%sNew password:%s", at(14, 10), White, Reset)
		drawField(&b, field, true)
		drawButton(&b, save, false)
		drawButton(&b, back, false)
		if msg != "" {
			fmt.Fprintf(&b, "%s%s", at(20, 24), msg)
		}
		s.Conn.Write([]byte(b.String()))

		ev := s.Reader.next()
		if ev.Err != nil {
			return
		}
		if ev.IsMouse {
			switch {
			case hit(ev, save):
				if len(field.Value) < 1 {
					msg = Red + "Enter a password." + Reset
					continue
				}
				if err := UpdatePassword(s.User.Username, string(field.Value)); err != nil {
					msg = Red + "Error: " + err.Error() + Reset
				} else {
					msg = Green + "✓ Password updated." + Reset
					field.Value = field.Value[:0]
				}
			case hit(ev, back):
				return
			}
			continue
		}
		switch ev.Key {
		case 13, 10:
			if len(field.Value) >= 1 {
				if err := UpdatePassword(s.User.Username, string(field.Value)); err != nil {
					msg = Red + "Error: " + err.Error() + Reset
				} else {
					msg = Green + "✓ Password updated." + Reset
					field.Value = field.Value[:0]
				}
			}
		case 127, 8:
			if len(field.Value) > 0 {
				field.Value = field.Value[:len(field.Value)-1]
			}
		case 3, 4:
			return
		default:
			if ev.Key >= 32 && ev.Key < 127 && len(field.Value) < field.MaxLen {
				field.Value = append(field.Value, ev.Key)
			}
		}
		if ev.Esc {
			return
		}
	}
}
