package logic

import (
	"fmt"
	"rss_parrot/dal"
	"rss_parrot/dto"
	"rss_parrot/shared"
	"strings"
)

const pageSize = 2

type IUserDirectory interface {
	GetWebfinger(user, instance string) *dto.WebfingerResp
	GetUserInfo(user string) *dto.UserInfo
	GetOutboxSummary(user string) *dto.OrderedListSummary
	GetFollowersSummary(user string) *dto.OrderedListSummary
	GetFollowingSummary(user string) *dto.OrderedListSummary
}

type userDirectory struct {
	cfg  *shared.Config
	repo dal.IRepo
	idb  idBuilder
}

func NewUserDirectory(cfg *shared.Config, repo dal.IRepo) IUserDirectory {
	return &userDirectory{cfg, repo, idBuilder{cfg.Host}}
}

func (udir *userDirectory) GetWebfinger(user, host string) *dto.WebfingerResp {
	cfgHost := udir.cfg.Host
	cfgBirb := udir.cfg.BirbName

	if !strings.EqualFold(host, cfgHost) || !strings.EqualFold(user, cfgBirb) {
		return nil
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

func (udir *userDirectory) GetUserInfo(user string) *dto.UserInfo {

	cfgBirb := udir.cfg.BirbName
	if !strings.EqualFold(user, cfgBirb) {
		return nil
	}

	user = strings.ToLower(user)
	userUrl := udir.idb.UserUrl(user)

	resp := dto.UserInfo{
		Context: []string{
			"https://www.w3.org/ns/activitystreams",
			"https://w3id.org/security/v1",
		},
		Id:                userUrl,
		Type:              "Person",
		PreferredUserName: user,
		Name:              "Birby Mc Birb",
		Summary:           "Psittaciform diversity in South America and Australasia suggests that the order may have evolved in Gondwana, centred in Australasia.",
		ManuallyApproves:  false,
		Published:         "2018-04-23T22:05:35Z",
		Inbox:             udir.idb.UserInbox(user),
		Outbox:            udir.idb.UserOutbox(user),
		Followers:         udir.idb.UserFollowers(user),
		Following:         udir.idb.UserFollowing(user),
		Endpoints:         dto.UserEndpoints{SharedInbox: udir.idb.SharedInbox()},
		PublicKey: dto.PublicKey{
			Id:           udir.idb.UserKeyId(user),
			Owner:        userUrl,
			PublicKeyPem: udir.cfg.BirbPubkey,
		},
	}
	return &resp
}

func (udir *userDirectory) GetOutboxSummary(user string) *dto.OrderedListSummary {

	cfgBirb := udir.cfg.BirbName
	if !strings.EqualFold(user, cfgBirb) {
		return nil
	}

	user = strings.ToLower(user)

	resp := dto.OrderedListSummary{
		Context:    "https://www.w3.org/ns/activitystreams",
		Id:         udir.idb.UserUrl(user),
		Type:       "OrderedCollection",
		TotalItems: udir.repo.GetPostCount(),
	}
	return &resp
}

func (udir *userDirectory) GetFollowersSummary(user string) *dto.OrderedListSummary {

	cfgBirb := udir.cfg.BirbName
	if !strings.EqualFold(user, cfgBirb) {
		return nil
	}

	user = strings.ToLower(user)

	resp := dto.OrderedListSummary{
		Context:    "https://www.w3.org/ns/activitystreams",
		Id:         udir.idb.UserFollowers(user),
		Type:       "OrderedCollection",
		TotalItems: udir.repo.GetPostCount(),
	}
	return &resp
}

func (udir *userDirectory) GetFollowingSummary(user string) *dto.OrderedListSummary {

	cfgBirb := udir.cfg.BirbName
	if !strings.EqualFold(user, cfgBirb) {
		return nil
	}

	user = strings.ToLower(user)

	resp := dto.OrderedListSummary{
		Context:    "https://www.w3.org/ns/activitystreams",
		Id:         udir.idb.UserFollowers(user),
		Type:       "OrderedCollection",
		TotalItems: udir.repo.GetPostCount(),
	}
	return &resp
}
