package logic

import (
	"fmt"
	"rss_parrot/dal"
	"rss_parrot/dto"
	"rss_parrot/shared"
	"rss_parrot/texts"
	"strings"
	"time"
)

const pageSize = 2
const websiteLinkTemplate = "<a href='%s' target='_blank' rel='nofollow noopener noreferrer me' translate='no'>%s</a>"

// TODO: return error in all of these

type IUserDirectory interface {
	GetWebfinger(user string) *dto.WebfingerResp
	GetUserInfo(user string) *dto.UserInfo
	GetOutboxSummary(user string) *dto.OrderedListSummary
	GetFollowersSummary(user string) *dto.OrderedListSummary
	GetFollowingSummary(user string) *dto.OrderedListSummary
	AcceptFollower(followActId, followerUserUrl, followerInbox, followedUser string) error
}

type userDirectory struct {
	cfg      *shared.Config
	logger   shared.ILogger
	repo     dal.IRepo
	idb      shared.IdBuilder
	keyStore IKeyStore
	sender   IActivitySender
	txt      texts.ITexts
}

func NewUserDirectory(
	cfg *shared.Config,
	logger shared.ILogger,
	repo dal.IRepo,
	keyStore IKeyStore,
	sender IActivitySender,
	txt texts.ITexts,
) IUserDirectory {
	return &userDirectory{
		cfg:      cfg,
		logger:   logger,
		repo:     repo,
		idb:      shared.IdBuilder{cfg.Host},
		keyStore: keyStore,
		sender:   sender,
		txt:      txt}
}

func (udir *userDirectory) GetWebfinger(user string) *dto.WebfingerResp {

	cfgHost := udir.cfg.Host
	acct, err := udir.repo.GetAccount(user)
	if err != nil || acct == nil {
		return nil // TODO errors
	}

	user = strings.ToLower(user)
	resp := dto.WebfingerResp{
		Subject: fmt.Sprintf("acct:%s@%s", user, cfgHost),
		Aliases: []string{
			udir.idb.UserProfile(user),
			udir.idb.UserUrl(user),
		},
		Links: []dto.WebfingerLink{
			{
				Rel:  "http://webfinger.net/rel/profile-page",
				Type: "text/html",
				Href: udir.idb.UserProfile(user),
			},
			{
				Rel:  "self",
				Type: "application/activity+json",
				Href: udir.idb.UserUrl(user),
			},
		},
	}
	return &resp
}

func (udir *userDirectory) patchSpecialAccount(acct *dal.Account) (bool, bool) {
	if acct.Handle == udir.cfg.Birb.User {
		acct.Name = udir.txt.Get("birb_name.txt")
		acct.Summary = udir.txt.Get("birb_bio.html")
		acct.PubKey = udir.cfg.Birb.PubKey
		acct.ProfileImageUrl = udir.cfg.Birb.ProfilePic
		acct.HeaderImageUrl = udir.cfg.Birb.HeaderPic
		return true, udir.cfg.Birb.ManuallyApprovesFollows
	}
	return false, false
}

func (udir *userDirectory) getWebsiteAttachment(acct *dal.Account) string {
	if acct.Handle == udir.cfg.Birb.User {
		return fmt.Sprintf(websiteLinkTemplate, "https://"+udir.cfg.Host, udir.cfg.Host)
	}
	justUrl := strings.TrimPrefix(acct.SiteUrl, "https://")
	return fmt.Sprintf(websiteLinkTemplate, acct.SiteUrl, justUrl)
}

func (udir *userDirectory) GetUserInfo(user string) *dto.UserInfo {

	user = strings.ToLower(user)
	userUrl := udir.idb.UserUrl(user)
	acct, err := udir.repo.GetAccount(user)
	if err != nil || acct == nil {
		return nil // TODO errors
	}
	_, manuallyApproves := udir.patchSpecialAccount(acct)

	resp := dto.UserInfo{
		Context: []string{
			"https://www.w3.org/ns/activitystreams",
			"https://w3id.org/security/v1",
		},
		Id:                userUrl,
		Type:              "Service",
		PreferredUserName: user,
		Name:              acct.Name,
		Summary:           acct.Summary,
		ManuallyApproves:  manuallyApproves,
		Published:         acct.CreatedAt.Format(time.RFC3339),
		Inbox:             udir.idb.UserInbox(user),
		Outbox:            udir.idb.UserOutbox(user),
		Followers:         udir.idb.UserFollowers(user),
		Following:         udir.idb.UserFollowing(user),
		Endpoints:         dto.UserEndpoints{SharedInbox: udir.idb.SharedInbox()},
		PublicKey: dto.PublicKey{
			Id:           udir.idb.UserKeyId(user),
			Owner:        userUrl,
			PublicKeyPem: acct.PubKey,
		},
		Attachments: []dto.Attachment{
			{
				Type:  "PropertyValue",
				Name:  "Website",
				Value: udir.getWebsiteAttachment(acct),
			},
		},
		Icon: dto.Image{
			Type: "Image",
			Url:  acct.ProfileImageUrl,
		},
		Image: dto.Image{
			Type: "Image",
			Url:  acct.HeaderImageUrl,
		},
	}
	return &resp
}

func (udir *userDirectory) GetOutboxSummary(user string) *dto.OrderedListSummary {

	var err error
	var exists bool
	user = strings.ToLower(user)
	exists, err = udir.repo.DoesAccountExist(user)
	if err != nil || !exists {
		return nil // TODO errors
	}

	var postCount uint
	postCount, err = udir.repo.GetTootCount(user) // TODO errors

	resp := dto.OrderedListSummary{
		Context:    "https://www.w3.org/ns/activitystreams",
		Id:         udir.idb.UserUrl(user),
		Type:       "OrderedCollection",
		TotalItems: postCount,
	}
	return &resp
}

func (udir *userDirectory) GetFollowersSummary(user string) *dto.OrderedListSummary {

	var err error
	var exists bool
	user = strings.ToLower(user)
	exists, err = udir.repo.DoesAccountExist(user)
	if err != nil || !exists {
		return nil // TODO errors
	}

	var followerCount uint
	followerCount, err = udir.repo.GetApprovedFollowerCount(user) // TODO errors

	resp := dto.OrderedListSummary{
		Context:    "https://www.w3.org/ns/activitystreams",
		Id:         udir.idb.UserFollowers(user),
		Type:       "OrderedCollection",
		TotalItems: followerCount,
	}
	return &resp
}

func (udir *userDirectory) GetFollowingSummary(user string) *dto.OrderedListSummary {

	var err error
	var exists bool
	user = strings.ToLower(user)
	exists, err = udir.repo.DoesAccountExist(user)
	if err != nil || !exists {
		return nil // TODO errors
	}

	resp := dto.OrderedListSummary{
		Context:    "https://www.w3.org/ns/activitystreams",
		Id:         udir.idb.UserFollowers(user),
		Type:       "OrderedCollection",
		TotalItems: 0,
	}
	return &resp
}

func (udir *userDirectory) AcceptFollower(followActId, followerUserUrl, followerInbox, followedUser string) error {

	udir.logger.Infof("Accepting follow %s", followerInbox)

	privKey, err := udir.keyStore.GetPrivKey(followedUser)
	if err != nil {
		err = fmt.Errorf("failed to get private key for user %s: %v", followedUser, err)
		return err
	}

	acceptId := udir.repo.GetNextId()

	actAccept := dto.ActivityOut{
		Context: "https://www.w3.org/ns/activitystreams",
		Id:      udir.idb.ActivityUrl(acceptId),
		Type:    "Accept",
		Actor:   udir.idb.UserUrl(followedUser),
		Object: dto.ActivityOut{
			Id:     followActId,
			Type:   "Follow",
			Actor:  followerUserUrl,
			Object: udir.idb.UserUrl(followedUser),
		},
	}

	if err = udir.sender.Send(privKey, followedUser, followerInbox, &actAccept); err != nil {
		err = fmt.Errorf("failed to send 'Accept' activity: %v", err)
		return err
	}

	if err = udir.repo.SetFollowerApproveStatus(followedUser, followerUserUrl, 1); err != nil {
		err = fmt.Errorf("failed set follower approve status: %v", err)
		return err
	}

	return nil
}