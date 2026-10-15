// Package mail sends the users context's mail to Stalwart over SMTP submission (docs/adr/0012).
package mail

import (
	"bytes"
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"mime"
	"mime/quotedprintable"
	"net"
	"net/smtp"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/JorisJonkers-dev/identity/internal/users/app"
)

// ErrNoTLS is a server that does not offer STARTTLS while TLS is required.
var ErrNoTLS = errors.New("the mail server does not offer STARTTLS")

// errHeader is a header value that would break out of its line.
var errHeader = errors.New("a mail header holds a line break")

// SMTP is the app.Mailer. Without RequireTLS it sends in the clear when the server offers no
// STARTTLS, which only a local run (Mailpit) should allow.
type SMTP struct {
	Addr       string // host:port
	Username   string
	Password   string
	From       string
	RequireTLS bool
	// TLSConfig overrides the client's TLS settings; tests use it to trust their own certificate.
	TLSConfig *tls.Config
	Now       func() time.Time
}

// Send delivers m, or returns why not.
func (s SMTP) Send(ctx context.Context, m app.Mail) error {
	msg, err := s.message(m)
	if err != nil {
		return err
	}
	c, err := s.connect(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = c.Close() }()
	return s.deliver(c, m.To, msg)
}

// connect dials, greets, upgrades to TLS where offered (or refuses where required) and signs in.
func (s SMTP) connect(ctx context.Context) (*smtp.Client, error) {
	host, _, err := net.SplitHostPort(s.Addr)
	if err != nil {
		return nil, fmt.Errorf("mail address %q: %w", s.Addr, err)
	}
	var d net.Dialer
	conn, err := d.DialContext(ctx, "tcp", s.Addr)
	if err != nil {
		return nil, fmt.Errorf("dial the mail server: %w", err)
	}
	c, err := smtp.NewClient(conn, host)
	if err != nil {
		_ = conn.Close()
		return nil, fmt.Errorf("greet the mail server: %w", err)
	}
	if err := s.secure(c, host); err != nil {
		_ = c.Close()
		return nil, err
	}
	return c, nil
}

func (s SMTP) secure(c *smtp.Client, host string) error {
	if ok, _ := c.Extension("STARTTLS"); ok {
		cfg := s.TLSConfig
		if cfg == nil {
			cfg = &tls.Config{ServerName: host, MinVersion: tls.VersionTLS12}
		}
		if err := c.StartTLS(cfg); err != nil {
			return fmt.Errorf("start TLS: %w", err)
		}
	} else if s.RequireTLS {
		return ErrNoTLS
	}
	if s.Username == "" {
		return nil
	}
	if err := c.Auth(smtp.PlainAuth("", s.Username, s.Password, host)); err != nil {
		return fmt.Errorf("authenticate: %w", err)
	}
	return nil
}

// deliver sends one message to one recipient over a connected client.
func (s SMTP) deliver(c *smtp.Client, to string, msg []byte) error {
	if err := c.Mail(s.From); err != nil {
		return fmt.Errorf("MAIL FROM: %w", err)
	}
	if err := c.Rcpt(to); err != nil {
		return fmt.Errorf("RCPT TO: %w", err)
	}
	w, err := c.Data()
	if err != nil {
		return fmt.Errorf("DATA: %w", err)
	}
	// The writer buffers, so a failed write surfaces at Close; both are one outcome.
	_, err = w.Write(msg)
	if err = errors.Join(err, w.Close()); err != nil {
		return fmt.Errorf("send the message: %w", err)
	}
	return c.Quit()
}

// message renders m as an RFC 5322 message with a quoted-printable UTF-8 body.
func (s SMTP) message(m app.Mail) ([]byte, error) {
	for _, v := range []string{s.From, m.To, m.Subject} {
		if strings.ContainsAny(v, "\r\n") {
			return nil, errHeader
		}
	}
	now := time.Now
	if s.Now != nil {
		now = s.Now
	}
	_, domain, _ := strings.Cut(s.From, "@")
	var b bytes.Buffer
	fmt.Fprintf(&b, "From: %s\r\nTo: %s\r\nSubject: %s\r\nDate: %s\r\nMessage-ID: <%s@%s>\r\n",
		s.From, m.To, mime.QEncoding.Encode("utf-8", m.Subject), now().UTC().Format(time.RFC1123Z), uuid.NewString(), domain)
	b.WriteString("MIME-Version: 1.0\r\nContent-Type: text/plain; charset=utf-8\r\nContent-Transfer-Encoding: quoted-printable\r\n\r\n")
	qp := quotedprintable.NewWriter(&b)
	_, _ = qp.Write([]byte(strings.ReplaceAll(m.Text, "\n", "\r\n")))
	_ = qp.Close()
	return b.Bytes(), nil
}
