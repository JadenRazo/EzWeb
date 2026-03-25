package health

import (
	"fmt"
	"net/smtp"
	"strings"
)

type EmailSender struct {
	Host     string
	Port     int
	From     string
	To       []string
	Username string
	Password string
}

func NewEmailSender(host string, port int, from string, to string, username string, password string) *EmailSender {
	if host == "" || from == "" || to == "" {
		return nil
	}
	recipients := strings.Split(to, ",")
	for i := range recipients {
		recipients[i] = strings.TrimSpace(recipients[i])
	}
	return &EmailSender{
		Host:     host,
		Port:     port,
		From:     from,
		To:       recipients,
		Username: username,
		Password: password,
	}
}

func (es *EmailSender) SendAlert(domain string, failures int, lastError string) error {
	subject := fmt.Sprintf("Site Down: %s", domain)
	body := fmt.Sprintf("Site %s is DOWN.\n\n%d consecutive health check failures.\n\nLast error: %s", domain, failures, lastError)
	return es.send(subject, body)
}

func (es *EmailSender) SendRecovery(domain string) error {
	subject := fmt.Sprintf("Site Recovered: %s", domain)
	body := fmt.Sprintf("Site %s is back UP and responding normally.", domain)
	return es.send(subject, body)
}

func (es *EmailSender) send(subject, body string) error {
	msg := fmt.Sprintf("From: %s\r\nTo: %s\r\nSubject: %s\r\nMIME-Version: 1.0\r\nContent-Type: text/plain; charset=UTF-8\r\n\r\n%s",
		es.From, strings.Join(es.To, ", "), subject, body)

	addr := fmt.Sprintf("%s:%d", es.Host, es.Port)

	var auth smtp.Auth
	if es.Username != "" {
		auth = smtp.PlainAuth("", es.Username, es.Password, es.Host)
	}

	return smtp.SendMail(addr, auth, es.From, es.To, []byte(msg))
}
