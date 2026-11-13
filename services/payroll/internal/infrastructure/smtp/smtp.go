// Package smtp sends the payslip mail over implicit TLS (port 465, D-62 butir 8) or STARTTLS (587), with one PDF
// attachment. The body and the PDF are never logged.
package smtp

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/base64"
	"errors"
	"fmt"
	"mime"
	"mime/multipart"
	"mime/quotedprintable"
	"net"
	"net/mail"
	"net/smtp"
	"net/textproto"
	"strconv"
	"strings"
	"time"

	"github.com/mutugading/goapps-backend/services/payroll/internal/infrastructure/config"
)

// Mail is one message with a single PDF attachment.
type Mail struct {
	MessageID  string // with angle brackets
	From       string
	FromName   string
	ReplyTo    string
	To         string
	Subject    string
	Text, HTML string
	AttachName string
	Attach     []byte
	Date       time.Time
}

// Sender delivers a Mail and returns the server response summary.
type Sender interface {
	Send(ctx context.Context, m Mail) (response string, err error)
}

// Client is the net/smtp implementation of Sender.
type Client struct {
	Cfg     config.SMTPConfig
	Timeout time.Duration
}

// NewClient creates a Client with a 60 s dial/IO timeout.
func NewClient(cfg config.SMTPConfig) *Client { return &Client{Cfg: cfg, Timeout: 60 * time.Second} }

var _ Sender = (*Client)(nil)

// Send connects, authenticates, and delivers m. The relay's final reply text is not exposed by net/smtp, so a
// success is recorded as "250 OK"; a failure carries the server's code and text in the error.
func (c *Client) Send(ctx context.Context, m Mail) (string, error) {
	msg, err := BuildMessage(m)
	if err != nil {
		return "", err
	}
	cl, conn, err := c.connect(ctx)
	if err != nil {
		return "", err
	}
	defer func() { _ = conn.Close() }()                         //nolint:errcheck // also closes on every error path
	stop := context.AfterFunc(ctx, func() { _ = conn.Close() }) //nolint:errcheck // unblocks I/O on cancel
	defer stop()
	if c.Cfg.TLSMode == config.SMTPStartTLS {
		if err := cl.StartTLS(&tls.Config{ServerName: c.Cfg.Host, MinVersion: tls.VersionTLS12}); err != nil {
			return "", fmt.Errorf("smtp starttls: %w", err)
		}
	}
	rcpt, err := mail.ParseAddress(m.To)
	if err != nil {
		return "", fmt.Errorf("smtp: alamat tujuan tidak valid: %w", err)
	}
	m.To = rcpt.Address
	if err := c.deliver(cl, m, msg); err != nil {
		return "", err
	}
	_ = cl.Quit() //nolint:errcheck // the message was accepted; QUIT failure is irrelevant
	return "250 OK", nil
}

// connect dials (implicit TLS for mode TLS) and reads the greeting; the deadline covers the whole session.
func (c *Client) connect(ctx context.Context) (*smtp.Client, net.Conn, error) {
	addr := net.JoinHostPort(c.Cfg.Host, strconv.Itoa(c.Cfg.Port))
	var conn net.Conn
	var err error
	if c.Cfg.TLSMode == config.SMTPTLS {
		d := tls.Dialer{NetDialer: &net.Dialer{Timeout: c.Timeout}, Config: &tls.Config{ServerName: c.Cfg.Host, MinVersion: tls.VersionTLS12}}
		conn, err = d.DialContext(ctx, "tcp", addr)
	} else {
		d := net.Dialer{Timeout: c.Timeout}
		conn, err = d.DialContext(ctx, "tcp", addr)
	}
	if err != nil {
		return nil, nil, fmt.Errorf("smtp dial: %w", err)
	}
	dl := time.Now().Add(c.Timeout)
	if cd, ok := ctx.Deadline(); ok && cd.Before(dl) {
		dl = cd
	}
	if err := conn.SetDeadline(dl); err != nil {
		_ = conn.Close() //nolint:errcheck // already failing
		return nil, nil, err
	}
	cl, err := smtp.NewClient(conn, c.Cfg.Host)
	if err != nil {
		_ = conn.Close() //nolint:errcheck // already failing
		return nil, nil, fmt.Errorf("smtp greet: %w", err)
	}
	return cl, conn, nil
}

func (c *Client) deliver(cl *smtp.Client, m Mail, msg []byte) error {
	if c.Cfg.User != "" {
		if err := cl.Auth(smtp.PlainAuth("", c.Cfg.User, c.Cfg.Password, c.Cfg.Host)); err != nil {
			return fmt.Errorf("smtp auth: %w", err)
		}
	}
	if err := cl.Mail(m.From); err != nil {
		return fmt.Errorf("smtp mail from: %w", err)
	}
	if err := cl.Rcpt(m.To); err != nil {
		return fmt.Errorf("smtp rcpt: %w", err)
	}
	w, err := cl.Data()
	if err != nil {
		return fmt.Errorf("smtp data: %w", err)
	}
	if _, err := w.Write(msg); err != nil {
		return fmt.Errorf("smtp write: %w", err)
	}
	if err := w.Close(); err != nil {
		return fmt.Errorf("smtp send: %w", err)
	}
	return nil
}

// BuildMessage renders the RFC 5322 message: multipart/mixed(alternative(text, html), pdf). Header values reject
// CR/LF so a name or address can never inject headers.
func BuildMessage(m Mail) ([]byte, error) {
	for _, v := range []string{m.MessageID, m.From, m.FromName, m.ReplyTo, m.To, m.Subject, m.AttachName} {
		if strings.ContainsAny(v, "\r\n") {
			return nil, errors.New("smtp: header berisi CR/LF")
		}
	}
	to, err := mail.ParseAddress(m.To)
	if err != nil {
		return nil, fmt.Errorf("smtp: alamat tujuan tidak valid: %w", err)
	}
	var b bytes.Buffer
	h := func(k, v string) { b.WriteString(k + ": " + v + "\r\n") }
	h("From", (&mail.Address{Name: m.FromName, Address: m.From}).String())
	h("To", to.String())
	if m.ReplyTo != "" {
		h("Reply-To", m.ReplyTo)
	}
	h("Subject", mime.QEncoding.Encode("utf-8", m.Subject))
	h("Date", m.Date.UTC().Format(time.RFC1123Z))
	h("Message-ID", m.MessageID)
	h("MIME-Version", "1.0")
	mixed := multipart.NewWriter(&b)
	h("Content-Type", "multipart/mixed; boundary="+mixed.Boundary())
	b.WriteString("\r\n")

	altBody, altBoundary, err := alternative(m)
	if err != nil {
		return nil, err
	}
	part, err := mixed.CreatePart(textproto.MIMEHeader{"Content-Type": {"multipart/alternative; boundary=" + altBoundary}})
	if err != nil {
		return nil, err
	}
	if _, err := part.Write(altBody); err != nil {
		return nil, err
	}
	if len(m.Attach) > 0 {
		ap, err := mixed.CreatePart(textproto.MIMEHeader{
			"Content-Type":              {"application/pdf; name=\"" + m.AttachName + "\""},
			"Content-Disposition":       {"attachment; filename=\"" + m.AttachName + "\""},
			"Content-Transfer-Encoding": {"base64"},
		})
		if err != nil {
			return nil, err
		}
		enc := base64.StdEncoding.EncodeToString(m.Attach)
		for len(enc) > 76 {
			_, _ = ap.Write([]byte(enc[:76] + "\r\n")) //nolint:errcheck // bytes.Buffer never fails
			enc = enc[76:]
		}
		_, _ = ap.Write([]byte(enc + "\r\n")) //nolint:errcheck // bytes.Buffer never fails
	}
	if err := mixed.Close(); err != nil {
		return nil, err
	}
	return b.Bytes(), nil
}

// alternative renders the text/html bodies as quoted-printable parts of multipart/alternative.
func alternative(m Mail) (body []byte, boundary string, err error) {
	alt := &bytes.Buffer{}
	aw := multipart.NewWriter(alt)
	for _, p := range []struct{ ct, body string }{{"text/plain", m.Text}, {"text/html", m.HTML}} {
		if p.body == "" {
			continue
		}
		pw, err := aw.CreatePart(textproto.MIMEHeader{"Content-Type": {p.ct + "; charset=utf-8"}, "Content-Transfer-Encoding": {"quoted-printable"}})
		if err != nil {
			return nil, "", err
		}
		qw := quotedprintable.NewWriter(pw)
		if _, err := qw.Write([]byte(p.body)); err != nil {
			return nil, "", err
		}
		if err := qw.Close(); err != nil {
			return nil, "", err
		}
	}
	if err := aw.Close(); err != nil {
		return nil, "", err
	}
	return alt.Bytes(), aw.Boundary(), nil
}
