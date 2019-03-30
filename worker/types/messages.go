package types

import (
	"crypto/x509"
	"time"

	"github.com/emersion/go-imap"
	"github.com/mohamedattahri/mail"

	"git.sr.ht/~sircmpwn/aerc2/config"
)

type WorkerMessage interface {
	InResponseTo() WorkerMessage
}

type Message struct {
	inResponseTo WorkerMessage
}

func RespondTo(msg WorkerMessage) Message {
	return Message{
		inResponseTo: msg,
	}
}

func (m Message) InResponseTo() WorkerMessage {
	return m.inResponseTo
}

// Meta-messages

type Done struct {
	Message
}

type Error struct {
	Message
	Error error
}

type Unsupported struct {
	Message
}

// Actions

type ApproveCertificate struct {
	Message
	Approved bool
}

type Configure struct {
	Message
	Config *config.AccountConfig
}

type Connect struct {
	Message
}

type Disconnect struct {
	Message
}

type ListDirectories struct {
	Message
}

type OpenDirectory struct {
	Message
	Directory string
}

type FetchDirectoryContents struct {
	Message
}

type FetchMessageHeaders struct {
	Message
	Uids imap.SeqSet
}

type FetchMessageBodies struct {
	Message
	Uids imap.SeqSet
}

type DeleteMessages struct {
	Message
	Uids imap.SeqSet
}

// Messages

type CertificateApprovalRequest struct {
	Message
	CertPool *x509.CertPool
}

type Directory struct {
	Message
	Attributes []string
	Name       string
}

type DirectoryInfo struct {
	Message
	Flags    []string
	Name     string
	ReadOnly bool

	Exists, Recent, Unseen int
}

type DirectoryContents struct {
	Message
	Uids []uint32
}

type MessageInfo struct {
	Message
	Envelope     *imap.Envelope
	Flags        []string
	InternalDate time.Time
	Size         uint32
	Uid          uint32
}

type MessageBody struct {
	Message
	Mail *mail.Message
	Uid  uint32
}

type MessagesDeleted struct {
	Message
	Uids []uint32
}
