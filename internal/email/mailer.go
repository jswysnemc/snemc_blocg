package email

import (
	"crypto/tls"
	"errors"
	"fmt"
	"log"
	"net"
	"net/smtp"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
)

type RuntimeConfig struct {
	SMTPHost     string
	SMTPPort     string
	SMTPUsername string
	SMTPPassword string
	SMTPFrom     string
}

type Mail struct {
	To       string
	Subject  string
	TextBody string
	HTMLBody string
	Done     func(error)
}

type Mailer struct {
	mu    sync.RWMutex
	cfg   RuntimeConfig
	queue chan Mail
}

var htmlTagPattern = regexp.MustCompile(`<[^>]+>`)

func New(cfg RuntimeConfig) *Mailer {
	m := &Mailer{
		cfg:   cfg,
		queue: make(chan Mail, 32),
	}
	go m.loop()
	return m
}

func (m *Mailer) Update(cfg RuntimeConfig) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.cfg = cfg
}

func (m *Mailer) Enqueue(mail Mail) bool {
	select {
	case m.queue <- mail:
		return true
	default:
		err := errors.New("mail queue full")
		log.Printf("mail queue full, dropping mail to %s", mail.To)
		if mail.Done != nil {
			mail.Done(err)
		}
		return false
	}
}

func (m *Mailer) loop() {
	for mail := range m.queue {
		err := m.send(mail)
		if err != nil {
			log.Printf("send mail failed: %v", err)
		}
		if mail.Done != nil {
			mail.Done(err)
		}
	}
}

func (m *Mailer) send(mail Mail) error {
	m.mu.RLock()
	cfg := m.cfg
	m.mu.RUnlock()

	if cfg.SMTPHost == "" || cfg.SMTPUsername == "" || cfg.SMTPPassword == "" {
		log.Printf("smtp not configured, simulated mail to %s: %s", mail.To, mail.Subject)
		return nil
	}

	addr := fmt.Sprintf("%s:%s", cfg.SMTPHost, cfg.SMTPPort)
	auth := smtp.PlainAuth("", cfg.SMTPUsername, cfg.SMTPPassword, cfg.SMTPHost)
	msg := strings.Join([]string{
		fmt.Sprintf("From: %s", cfg.SMTPFrom),
		fmt.Sprintf("To: %s", mail.To),
		fmt.Sprintf("Subject: %s", mail.Subject),
		"MIME-Version: 1.0",
	}, "\r\n") + "\r\n" + buildBody(mail)

	if cfg.SMTPPort == "465" {
		return sendImplicitTLS(addr, cfg.SMTPHost, auth, cfg.SMTPFrom, []string{mail.To}, []byte(msg))
	}
	return sendWithSTARTTLS(addr, cfg.SMTPHost, auth, cfg.SMTPFrom, []string{mail.To}, []byte(msg))
}

func buildBody(mail Mail) string {
	textBody := mail.TextBody
	if textBody == "" {
		textBody = stripHTML(mail.HTMLBody)
	}
	if mail.HTMLBody == "" {
		return strings.Join([]string{
			"Content-Type: text/plain; charset=UTF-8",
			"",
			textBody,
		}, "\r\n")
	}
	if textBody == "" {
		return strings.Join([]string{
			"Content-Type: text/html; charset=UTF-8",
			"",
			mail.HTMLBody,
		}, "\r\n")
	}

	boundary := "=_snemc_blog_" + strconv.FormatInt(time.Now().UnixNano(), 10)
	return strings.Join([]string{
		fmt.Sprintf("Content-Type: multipart/alternative; boundary=%q", boundary),
		"",
		"--" + boundary,
		"Content-Type: text/plain; charset=UTF-8",
		"",
		textBody,
		"--" + boundary,
		"Content-Type: text/html; charset=UTF-8",
		"",
		mail.HTMLBody,
		"--" + boundary + "--",
	}, "\r\n")
}

func stripHTML(input string) string {
	replacer := strings.NewReplacer(
		"<br>", "\n",
		"<br/>", "\n",
		"<br />", "\n",
		"</p>", "\n\n",
		"</div>", "\n",
		"</li>", "\n",
	)
	cleaned := replacer.Replace(input)
	cleaned = strings.ReplaceAll(cleaned, "&nbsp;", " ")
	cleaned = strings.ReplaceAll(cleaned, "&amp;", "&")
	cleaned = strings.ReplaceAll(cleaned, "&lt;", "<")
	cleaned = strings.ReplaceAll(cleaned, "&gt;", ">")
	cleaned = strings.ReplaceAll(cleaned, "&quot;", `"`)
	cleaned = strings.ReplaceAll(cleaned, "&#39;", "'")
	cleaned = htmlTagPattern.ReplaceAllString(cleaned, "")
	cleaned = strings.TrimSpace(cleaned)
	if cleaned == "" {
		return ""
	}
	return cleaned
}

func sendImplicitTLS(addr string, host string, auth smtp.Auth, from string, to []string, msg []byte) error {
	conn, err := tls.Dial("tcp", addr, &tls.Config{
		ServerName: host,
		MinVersion: tls.VersionTLS12,
	})
	if err != nil {
		return err
	}
	defer conn.Close()

	client, err := smtp.NewClient(conn, host)
	if err != nil {
		return err
	}
	defer client.Close()

	if err := client.Auth(auth); err != nil {
		return err
	}
	if err := client.Mail(from); err != nil {
		return err
	}
	for _, recipient := range to {
		if err := client.Rcpt(recipient); err != nil {
			return err
		}
	}
	writer, err := client.Data()
	if err != nil {
		return err
	}
	if _, err := writer.Write(msg); err != nil {
		return err
	}
	if err := writer.Close(); err != nil {
		return err
	}
	return client.Quit()
}

func sendWithSTARTTLS(addr string, host string, auth smtp.Auth, from string, to []string, msg []byte) error {
	conn, err := net.Dial("tcp", addr)
	if err != nil {
		return err
	}
	defer conn.Close()

	client, err := smtp.NewClient(conn, host)
	if err != nil {
		return err
	}
	defer client.Close()

	if ok, _ := client.Extension("STARTTLS"); ok {
		if err := client.StartTLS(&tls.Config{
			ServerName: host,
			MinVersion: tls.VersionTLS12,
		}); err != nil {
			return err
		}
	}
	if ok, _ := client.Extension("AUTH"); ok {
		if err := client.Auth(auth); err != nil {
			return err
		}
	}
	if err := client.Mail(from); err != nil {
		return err
	}
	for _, recipient := range to {
		if err := client.Rcpt(recipient); err != nil {
			return err
		}
	}
	writer, err := client.Data()
	if err != nil {
		return err
	}
	if _, err := writer.Write(msg); err != nil {
		return err
	}
	if err := writer.Close(); err != nil {
		return err
	}
	return client.Quit()
}
