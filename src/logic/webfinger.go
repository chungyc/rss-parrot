package logic

import (
	"fmt"
	"rss_parrot/config"
	"rss_parrot/dto"
	"strings"
)

type IWebfinger interface {
	MakeResponse(user, instance string) *dto.WebfingerResp
}

type Webfinger struct {
	cfg *config.Config
}

func NewWebfinger(cfg *config.Config) IWebfinger {
	return &Webfinger{cfg}
}

func (wf *Webfinger) MakeResponse(user, instance string) *dto.WebfingerResp {
	cfgInstance := wf.cfg.InstanceName
	cfgBirb := wf.cfg.BirbName

	if !strings.EqualFold(instance, cfgInstance) || !strings.EqualFold(user, cfgBirb) {
		return nil
	}

	user = strings.ToLower(user)
	resp := dto.WebfingerResp{
		Subject: fmt.Sprintf("acct:%s@%s", user, cfgInstance),
		Aliases: []string{
			fmt.Sprintf("https://%s/@%s", cfgInstance, user),
			fmt.Sprintf("https://%s/users/%s", cfgInstance, user),
		},
		Links: []dto.WebfingerLink{
			{
				Rel:  "http://webfinger.net/rel/profile-page",
				Type: "text/html",
				Href: fmt.Sprintf("https://%s/@%s", cfgInstance, user),
			},
			{
				Rel:  "self",
				Type: "application/activity+json",
				Href: fmt.Sprintf("https://%s/users/%s", cfgInstance, user),
			},
		},
	}
	return &resp
}
