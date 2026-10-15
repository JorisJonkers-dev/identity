package mail_test

import (
	"bufio"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/base64"
	"errors"
	"io"
	"math/big"
	"mime/quotedprintable"
	"net"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/JorisJonkers-dev/identity/internal/platform/mail"
	"github.com/JorisJonkers-dev/identity/internal/users/app"
)

// server is a fake SMTP server that records one session. starttls offers and performs STARTTLS
// with cert; refuse makes the named verb fail with a 550.
type server struct {
	ln       net.Listener
	starttls *tls.Config
	refuse   string

	mu      sync.Mutex
	auth    string
	from    string
	rcpt    string
	data    string
	upgrade bool
	done    chan struct{}
}

func newServer(t *testing.T, starttls *tls.Config, refuse string) *server {
	t.Helper()
	var lc net.ListenConfig
	ln, err := lc.Listen(t.Context(), "tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	s := &server{ln: ln, starttls: starttls, refuse: refuse, done: make(chan struct{})}
	t.Cleanup(func() { _ = ln.Close() })
	go s.serve()
	return s
}

func (s *server) serve() {
	defer close(s.done)
	conn, err := s.ln.Accept()
	if err != nil {
		return
	}
	defer func() { _ = conn.Close() }()
	r, w := bufio.NewReader(conn), conn
	say := func(line string) { _, _ = io.WriteString(w, line+"\r\n") }
	say("220 fake ESMTP")
	for {
		line, err := r.ReadString('\n')
		if err != nil {
			return
		}
		line = strings.TrimRight(line, "\r\n")
		verb := strings.ToUpper(strings.Fields(line + " x")[0])
		if verb == s.refuse {
			say("550 refused")
			continue
		}
		switch verb {
		case "EHLO":
			if s.starttls != nil && !s.upgrade {
				say("250-fake")
				say("250-STARTTLS")
			} else {
				say("250-fake")
			}
			say("250 AUTH PLAIN")
		case "STARTTLS":
			say("220 go ahead")
			tc := tls.Server(conn, s.starttls)
			if err := tc.HandshakeContext(context.Background()); err != nil {
				return
			}
			s.mu.Lock()
			s.upgrade = true
			s.mu.Unlock()
			conn, r, w = tc, bufio.NewReader(tc), tc
		case "AUTH":
			s.mu.Lock()
			s.auth = line
			s.mu.Unlock()
			say("235 ok")
		case "MAIL":
			s.mu.Lock()
			s.from = line
			s.mu.Unlock()
			say("250 ok")
		case "RCPT":
			s.mu.Lock()
			s.rcpt = line
			s.mu.Unlock()
			say("250 ok")
		case "DATA":
			say("354 go on")
			var body strings.Builder
			for {
				l, err := r.ReadString('\n')
				if err != nil {
					return
				}
				if l == ".\r\n" {
					break
				}
				body.WriteString(l)
			}
			s.mu.Lock()
			s.data = body.String()
			s.mu.Unlock()
			if s.refuse == "END" {
				say("554 refused")
				continue
			}
			say("250 queued")
		case "QUIT":
			say("221 bye")
			return
		default:
			say("502 unknown")
		}
	}
}

func selfSigned(t *testing.T) (*tls.Config, *tls.Config) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: "127.0.0.1"},
		IPAddresses:  []net.IP{net.ParseIP("127.0.0.1")},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(time.Hour),
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	cert, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatal(err)
	}
	pool := x509.NewCertPool()
	pool.AddCert(cert)
	server := &tls.Config{Certificates: []tls.Certificate{{Certificate: [][]byte{der}, PrivateKey: key}}, MinVersion: tls.VersionTLS12}
	client := &tls.Config{RootCAs: pool, ServerName: "127.0.0.1", MinVersion: tls.VersionTLS12}
	return server, client
}

var message = app.Mail{To: "ada@example.org", Subject: "Confirm your email", Text: "Hello Ada,\n\nA long line that is well past seventy-six characters so quoted-printable has to wrap it.\n"}

func TestSendUpgradesToTLSAuthenticatesAndDelivers(t *testing.T) {
	serverTLS, clientTLS := selfSigned(t)
	s := newServer(t, serverTLS, "")
	m := mail.SMTP{
		Addr: s.ln.Addr().String(), Username: "identity@jorisjonkers.dev", Password: "secret",
		From: "identity@jorisjonkers.dev", RequireTLS: true, TLSConfig: clientTLS,
		Now: func() time.Time { return time.Date(2026, 10, 15, 12, 0, 0, 0, time.UTC) },
	}
	if err := m.Send(context.Background(), message); err != nil {
		t.Fatal(err)
	}
	<-s.done
	if !s.upgrade {
		t.Fatal("the session was never upgraded to TLS")
	}
	creds, _ := base64.StdEncoding.DecodeString(strings.Fields(s.auth)[2])
	if string(creds) != "\x00identity@jorisjonkers.dev\x00secret" {
		t.Fatalf("AUTH sent %q", creds)
	}
	if s.from != "MAIL FROM:<identity@jorisjonkers.dev>" || !strings.HasPrefix(s.rcpt, "RCPT TO:<ada@example.org>") {
		t.Fatalf("envelope %q / %q", s.from, s.rcpt)
	}
	for _, header := range []string{
		"From: identity@jorisjonkers.dev\r\n", "To: ada@example.org\r\n", "Subject: Confirm your email\r\n",
		"Date: Thu, 15 Oct 2026 12:00:00 +0000\r\n", "Content-Type: text/plain; charset=utf-8\r\n", "@jorisjonkers.dev>\r\n",
	} {
		if !strings.Contains(s.data, header) {
			t.Errorf("message lacks %q:\n%s", header, s.data)
		}
	}
	_, body, _ := strings.Cut(s.data, "\r\n\r\n")
	decoded, err := io.ReadAll(quotedprintable.NewReader(strings.NewReader(body)))
	if err != nil || string(decoded) != strings.ReplaceAll(message.Text, "\n", "\r\n") {
		t.Fatalf("body decodes to %q, %v", decoded, err)
	}
}

func TestSendRefusesAServerWithoutTLSWhenTLSIsRequired(t *testing.T) {
	s := newServer(t, nil, "")
	err := mail.SMTP{Addr: s.ln.Addr().String(), From: "a@b.c", RequireTLS: true}.Send(context.Background(), message)
	if !errors.Is(err, mail.ErrNoTLS) {
		t.Fatalf("err = %v, want ErrNoTLS", err)
	}
}

func TestSendInTheClearOnlyWhenAllowed(t *testing.T) {
	s := newServer(t, nil, "")
	if err := (mail.SMTP{Addr: s.ln.Addr().String(), From: "a@b.c"}).Send(context.Background(), message); err != nil {
		t.Fatal(err)
	}
	<-s.done
	if s.auth != "" {
		t.Fatal("authenticated without credentials configured")
	}
}

func TestSendReportsEveryRefusal(t *testing.T) {
	serverTLS, clientTLS := selfSigned(t)
	for _, verb := range []string{"AUTH", "MAIL", "RCPT", "DATA", "END"} {
		s := newServer(t, serverTLS, verb)
		m := mail.SMTP{Addr: s.ln.Addr().String(), Username: "u", Password: "p", From: "a@b.c", RequireTLS: true, TLSConfig: clientTLS}
		if err := m.Send(context.Background(), message); err == nil {
			t.Errorf("a refused %s was reported as sent", verb)
		}
	}
}

func TestSendRefusesAHeaderThatBreaksItsLine(t *testing.T) {
	m := mail.SMTP{Addr: "127.0.0.1:1", From: "a@b.c"}
	if err := m.Send(context.Background(), app.Mail{To: "a@b.c\r\nBcc: x@y.z", Subject: "s", Text: "t"}); err == nil {
		t.Fatal("a header with a line break was sent")
	}
}

func TestSendReportsConnectionFailures(t *testing.T) {
	ctx := context.Background()
	if err := (mail.SMTP{Addr: "no port", From: "a@b.c"}).Send(ctx, message); err == nil {
		t.Error("an address without a port was dialled")
	}
	var lc net.ListenConfig
	ln, err := lc.Listen(ctx, "tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := ln.Addr().String()
	_ = ln.Close()
	if err := (mail.SMTP{Addr: addr, From: "a@b.c"}).Send(ctx, message); err == nil {
		t.Error("a closed port was reported as sent")
	}

	// A server that hangs up instead of greeting.
	ln, err = lc.Listen(ctx, "tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = ln.Close() }()
	go func() {
		c, err := ln.Accept()
		if err == nil {
			_ = c.Close()
		}
	}()
	if err := (mail.SMTP{Addr: ln.Addr().String(), From: "a@b.c"}).Send(ctx, message); err == nil {
		t.Error("a server that never greeted was reported as sent")
	}

	// A server whose certificate the client does not trust.
	serverTLS, _ := selfSigned(t)
	s := newServer(t, serverTLS, "")
	if err := (mail.SMTP{Addr: s.ln.Addr().String(), From: "a@b.c", RequireTLS: true}).Send(ctx, message); err == nil {
		t.Error("an untrusted certificate was accepted")
	}
}
