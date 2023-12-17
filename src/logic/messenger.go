package logic

import (
	"rss_parrot/dal"
	"rss_parrot/dto"
	"rss_parrot/shared"
	"time"
)

type IMessenger interface {
	SendReply(byUser, toMoniker, toUserUrl, toInbox, msg string)
	Broadcast(user string, published, message string) error
}

type messenger struct {
	cfg        *shared.Config
	logger     shared.ILogger
	repo       dal.IRepo
	keyHandler IKeyHandler
	sender     IActivitySender
	idb        shared.IdBuilder
}

func NewMessenger(
	cfg *shared.Config,
	logger shared.ILogger,
	repo dal.IRepo,
	keyHandler IKeyHandler,
	sender IActivitySender,
) IMessenger {
	return &messenger{
		cfg:        cfg,
		logger:     logger,
		repo:       repo,
		keyHandler: keyHandler,
		sender:     sender,
		idb:        shared.IdBuilder{cfg.Host},
	}
}

func (m *messenger) SendReply(byUser, toMoniker, toUserUrl, toInbox, msg string) {
	to := []string{toUserUrl}
	cc := []string{}
	published := time.Now().UTC().Format(time.RFC3339)
	tag := dto.Tag{Type: "Mention", Href: toUserUrl, Name: toMoniker}
	err := m.sendToInbox(byUser, to, cc, toInbox, published, msg, &[]dto.Tag{tag})
	if err != nil {
		m.logger.Errorf("Failed to send reply to %s", toUserUrl)
	}
}

func (m *messenger) Broadcast(user, published, message string) error {

	followers, err := m.repo.GetFollowers(user)
	if err != nil {
		return err
	}

	// Collect distinct shared inboxes
	inboxes := make(map[string]struct{})
	for _, f := range followers {
		if _, exists := inboxes[f.SharedInbox]; !exists {
			inboxes[f.SharedInbox] = struct{}{}
		}
	}

	to := []string{shared.ActivityPublic}
	for inboxUrl := range inboxes {
		userFollowers := m.idb.UserFollowers(user)
		err = m.sendToInbox(user, to, []string{userFollowers}, inboxUrl, published, message, nil)
		if err != nil {
			return err
		}
	}

	return nil
}

func (m *messenger) sendToInbox(byUser string, to, cc []string, toInbox, published,
	message string, tag *[]dto.Tag) error {

	m.logger.Infof("Sending to inbox: %s", toInbox)

	privKey, err := m.keyHandler.GetPrivKey(byUser)
	if err != nil {
		return err
	}

	id := m.repo.GetNextId()
	note := &dto.Note{
		Id:           m.idb.UserStatus(byUser, id),
		Type:         "Note",
		Published:    published,
		Summary:      nil,
		AttributedTo: m.idb.UserUrl(byUser),
		InReplyTo:    nil,
		Content:      message,
		To:           to,
		Cc:           cc,
		Tag:          tag,
	}
	act := &dto.ActivityOut{
		Context: "https://www.w3.org/ns/activitystreams",
		Id:      m.idb.UserStatusActivity(byUser, id),
		Type:    "Create",
		Actor:   m.idb.UserUrl(byUser),
		To:      &to,
		Cc:      &cc,
		Object:  note,
	}

	m.sender.Send(privKey, byUser, toInbox, act)

	return nil
}
