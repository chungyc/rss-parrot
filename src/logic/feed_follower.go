package logic

import (
	"github.com/PuerkitoBio/goquery"
	"github.com/microcosm-cc/bluemonday"
	"github.com/mmcdole/gofeed"
	"github.com/spaolacci/murmur3"
	"math/rand"
	"net/http"
	"net/url"
	"rss_parrot/dal"
	"rss_parrot/shared"
	"rss_parrot/texts"
	"sort"
	"strings"
	"time"
)

const feedCheckLoopIdleWakeSec = 60

type IFeedFollower interface {
	GetAccountForFeed(urlStr string) *dal.Account
}

type SiteInfo struct {
	Url          string
	ParrotHandle string
	FeedUrl      string
	LastUpdated  time.Time
	Title        string
	Description  string
}

type feedFollower struct {
	cfg        *shared.Config
	logger     shared.ILogger
	repo       dal.IRepo
	messenger  IMessenger
	txt        texts.ITexts
	keyHandler IKeyHandler
}

func NewFeedFollower(
	cfg *shared.Config,
	logger shared.ILogger,
	repo dal.IRepo,
	messenger IMessenger,
	txt texts.ITexts,
	keyHandler IKeyHandler,
) IFeedFollower {
	ff := feedFollower{
		cfg:        cfg,
		logger:     logger,
		repo:       repo,
		messenger:  messenger,
		txt:        txt,
		keyHandler: keyHandler,
	}
	go ff.feedCheckLoop()
	return &ff
}

func (ff *feedFollower) getFeedUrl(siteUrl *url.URL, doc *goquery.Document) string {
	var feedUrlStr string
	isFeedRss := false
	doc.Find("link[rel='alternate']").Each(func(_ int, s *goquery.Selection) {
		var aType, aHref string
		var ok bool
		if aType, ok = s.Attr("type"); !ok {
			return
		}
		if aHref, ok = s.Attr("href"); !ok {
			return
		}
		if aType == "application/atom+xml" && !isFeedRss && feedUrlStr == "" {
			feedUrlStr = aHref
		}
		if aType == "application/rss+xml" && (feedUrlStr == "" || !isFeedRss) {
			feedUrlStr = aHref
			isFeedRss = true
		}
	})
	// Make it absolute
	feedUrl, err := url.Parse(feedUrlStr)
	if err != nil {
		return ""
	}
	if !feedUrl.IsAbs() {
		feedUrl = siteUrl.ResolveReference(feedUrl)
	}
	// It's a keeper
	res := feedUrl.String()
	res = strings.TrimRight(res, "/")
	return res
}

func (ff *feedFollower) getMetas(siteUrl *url.URL, doc *goquery.Document, si *SiteInfo) {
	s := doc.Find("title").First()
	if s.Length() != 0 {
		si.Title = s.Text()
	}
	s = doc.Find("meta[name='description']").First()
	if s.Length() != 0 {
		si.Description = s.AttrOr("content", "")
	}
}

func getLastUpdated(feed *gofeed.Feed) time.Time {
	var t time.Time
	if feed.PublishedParsed != nil {
		t = *feed.PublishedParsed
	}
	if feed.UpdatedParsed != nil && feed.UpdatedParsed.After(t) {
		t = *feed.UpdatedParsed
	}
	for _, itm := range feed.Items {
		if itm.PublishedParsed != nil && itm.PublishedParsed.After(t) {
			t = *itm.PublishedParsed
		}
		if itm.UpdatedParsed != nil && itm.UpdatedParsed.After(t) {
			t = *itm.UpdatedParsed
		}
	}
	return t
}

func (ff *feedFollower) getSiteInfo(urlStr string) (*SiteInfo, *gofeed.Feed) {

	urlStr = strings.TrimRight(urlStr, "/")
	var res SiteInfo
	var err error

	// First, let's see if this is the feed itself
	fp := gofeed.NewParser()
	var feed *gofeed.Feed
	feed, err = fp.ParseURL(urlStr)
	if err == nil {
		res.FeedUrl = urlStr
		res.LastUpdated = getLastUpdated(feed)
		res.Title = feed.Title
		res.Description = feed.Description
		res.Url = feed.Link
		res.ParrotHandle = shared.GetHandleFromUrl(res.Url)
		return &res, feed
	}

	// Normalize URL
	siteUrl, err := url.Parse(urlStr)
	if err != nil {
		return nil, nil
	}
	res.Url = urlStr
	res.ParrotHandle = shared.GetHandleFromUrl(res.Url)

	// Get the page
	resp, err := http.Get(urlStr)
	if err != nil {
		ff.logger.Warnf("Failed to get %s: %v", siteUrl, err)
		return nil, nil
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		ff.logger.Warnf("Failed to get %s: status %d", siteUrl, resp.StatusCode)
		return nil, nil
	}

	// Load the HTML document
	doc, err := goquery.NewDocumentFromReader(resp.Body)
	if err != nil {
		ff.logger.Warnf("Failed to parse HTML from %s: %v", siteUrl, err)
		return nil, nil
	}

	// Pick out the data we're interested in
	res.FeedUrl = ff.getFeedUrl(siteUrl, doc)
	if res.FeedUrl == "" {
		ff.logger.Warnf("No feed URL found: %s", siteUrl)
		return nil, nil
	}
	ff.getMetas(siteUrl, doc, &res)

	// Get the feed to make sure it's there, and know when it's last changed
	feed, err = fp.ParseURL(res.FeedUrl)
	if err != nil {
		ff.logger.Warnf("Failed to retrieve and parse feed: %s, %v", res.FeedUrl, err)
		return nil, nil
	}
	res.LastUpdated = getLastUpdated(feed)

	return &res, feed
}

func getGuidHash(guid string) uint {
	hasher := murmur3.New32()
	_, _ = hasher.Write([]byte(guid))
	return uint(hasher.Sum32())
}

func (ff *feedFollower) updateAccountPosts(
	accountId int,
	accountHandle string,
	feed *gofeed.Feed,
	tootNew bool,
) (err error) {
	err = nil
	var lastKnownFeedUpdated time.Time

	if lastKnownFeedUpdated, err = ff.repo.GetFeedLastUpdated(accountId); err != nil {
		return
	}

	// Deal with feed items newer than our last seen
	// This goes from older to newer
	keepers, newLastUpdated := getSortedPosts(feed.Items, lastKnownFeedUpdated)
	for _, k := range keepers {
		if err = ff.storePostIfNew(accountId, accountHandle, k.postTime, k.itm, tootNew); err != nil {
			return
		}
	}

	nextCheckDue := ff.getNextCheckTime(newLastUpdated)
	if err = ff.repo.UpdateAccountFeedTimes(accountId, newLastUpdated, nextCheckDue); err != nil {
		return
	}
	return
}

type sortedPost struct {
	itm      *gofeed.Item
	postTime time.Time
}

func getSortedPosts(items []*gofeed.Item, lastKnownFeedUpdated time.Time) ([]sortedPost, time.Time) {
	var keepers []sortedPost
	newLastUpdated := lastKnownFeedUpdated

	for _, itm := range items {
		keeper, postTime := checkItemTime(itm, lastKnownFeedUpdated)
		if !keeper {
			continue
		}
		if postTime.After(newLastUpdated) {
			newLastUpdated = postTime
		}
		keepers = append(keepers, sortedPost{itm, postTime})
	}

	sort.Slice(keepers, func(i, j int) bool {
		return keepers[i].postTime.Before(keepers[j].postTime)
	})

	return keepers, newLastUpdated
}

func checkItemTime(itm *gofeed.Item, latestKown time.Time) (keeper bool, postTime time.Time) {
	keeper = false
	postTime = time.Time{}
	if itm.PublishedParsed != nil && itm.PublishedParsed.After(latestKown) {
		keeper = true
		postTime = *itm.PublishedParsed
	}
	if itm.UpdatedParsed != nil && itm.UpdatedParsed.After(latestKown) {
		keeper = true
		if itm.UpdatedParsed.After(postTime) {
			postTime = *itm.UpdatedParsed
		}
	}
	return
}

func (ff *feedFollower) getNextCheckTime(lastChanged time.Time) time.Time {

	// Active in the last day: 1 hour
	// Active in the last week: 3 hours
	// Active in the last 4 weeks: 6 hours
	// Inactive for over 4 weeks: 12 hours
	var hours float64 = 1
	idleFor := time.Now().Sub(lastChanged)
	if idleFor.Hours() > 24 {
		hours = 3
	}
	if idleFor.Hours() > 168 {
		hours = 6
	}
	if idleFor.Hours() > 168*4 {
		hours = 12
	}

	hours = hours * (0.8 + 0.4*rand.Float64()) // 0.8 - 1.2 random band around targeted value
	res := time.Now().Add(time.Duration(float64(time.Hour) * hours))
	return res
}

func stripHtml(htm string) string {
	p := bluemonday.StripTagsPolicy()
	plain := p.Sanitize(htm)
	return plain
}

func (ff *feedFollower) storePostIfNew(
	accountId int,
	accountHandle string,
	postTime time.Time,
	itm *gofeed.Item,
	tootNew bool,
) (err error) {
	var isNew bool
	plainTitle := stripHtml(itm.Title)
	plainDescription := stripHtml(itm.Description)
	isNew, err = ff.repo.AddFeedPostIfNew(accountId, &dal.FeedPost{
		PostGuidHash: int64(getGuidHash(itm.GUID)),
		PostTime:     postTime,
		Link:         itm.Link,
		Title:        plainTitle,
		Desription:   plainDescription,
	})
	if err != nil {
		return
	}
	if isNew {
		if err = ff.createToot(accountId, accountHandle, itm, tootNew); err != nil {
			return
		}
	}
	return
}

func (ff *feedFollower) createToot(accountId int, accountHandle string, itm *gofeed.Item, sendToot bool) error {
	prettyUrl := itm.Link
	prettyUrl = strings.TrimPrefix(prettyUrl, "http://")
	prettyUrl = strings.TrimPrefix(prettyUrl, "https://")
	prettyUrl = strings.TrimRight(prettyUrl, "/")
	plainTitle := stripHtml(itm.Title)
	plainDescription := stripHtml(itm.Description)
	content := ff.txt.WithVals("toot_new_post.html", map[string]string{
		"title":       plainTitle,
		"url":         itm.Link,
		"prettyUrl":   prettyUrl,
		"description": plainDescription,
	})
	idb := shared.IdBuilder{ff.cfg.Host}
	id := ff.repo.GetNextId()
	statusId := idb.UserStatus(accountHandle, id)
	tootedAt := time.Now()
	err := ff.repo.AddToot(accountId, &dal.Toot{
		PostGuidHash: int64(getGuidHash(itm.GUID)),
		TootedAt:     tootedAt,
		StatusId:     statusId,
		Content:      content,
	})
	if err != nil {
		return err
	}
	if sendToot {
		if err = ff.messenger.EnqueueBroadcast(accountHandle, statusId, tootedAt, content); err != nil {
			return err
		}
	}
	return nil
}

func (ff *feedFollower) GetAccountForFeed(urlStr string) *dal.Account {

	ff.logger.Infof("Retrieving site information: %s", urlStr)

	var err error

	si, feed := ff.getSiteInfo(urlStr)
	if si == nil {
		return nil
	}

	idb := shared.IdBuilder{ff.cfg.Host}

	var pubKey string
	var privKey string
	pubKey, privKey, err = ff.keyHandler.MakeKeyPair()
	if err != nil {
		ff.logger.Errorf("Failed to create key pair: %v", err)
		return nil
	}

	isNew, err := ff.repo.AddAccountIfNotExist(&dal.Account{
		CreatedAt: time.Now(),
		Handle:    si.ParrotHandle,
		UserUrl:   idb.UserUrl(si.ParrotHandle),
		Name:      shared.GetNameWithParrot(si.Title),
		Summary:   ff.txt.WithVals("acct_bio.html", map[string]string{"description": si.Description}),
		SiteUrl:   si.Url,
		FeedUrl:   si.FeedUrl,
		PubKey:    pubKey,
	}, privKey)

	if err != nil {
		ff.logger.Errorf("Failed to create/get account for %s: %v", si.ParrotHandle, isNew)
		return nil
	}

	ff.logger.Infof("Account is %s; newly created: %v", si.ParrotHandle, isNew)

	var acct *dal.Account
	acct, err = ff.repo.GetAccount(si.ParrotHandle)
	if err != nil {
		ff.logger.Errorf("Failed to load account for %s; was newly created: %v", si.ParrotHandle, isNew)
		return nil
	}

	err = ff.updateAccountPosts(acct.Id, si.ParrotHandle, feed, !isNew)
	if err != nil {
		ff.logger.Errorf("Failed to update account's posts: %s: %v", acct.Handle, err)
		return nil
	}

	return acct
}

func (ff *feedFollower) updateFeed(acct *dal.Account) error {

	var err error
	ff.logger.Infof("Updating account %s: %s", acct.Handle, acct.FeedUrl)

	fp := gofeed.NewParser()
	var feed *gofeed.Feed
	if feed, err = fp.ParseURL(acct.FeedUrl); err != nil {
		return err
	}

	if err = ff.updateAccountPosts(acct.Id, acct.Handle, feed, true); err != nil {
		return err
	}

	return nil
}

func (ff *feedFollower) feedCheckLoop() {
	for {
		var err error
		var acct *dal.Account
		if acct, err = ff.repo.GetAccountToCheck(time.Now()); err != nil {
			ff.logger.Errorf("Failed to get next feed due for checking: %v", err)
			time.Sleep(feedCheckLoopIdleWakeSec * time.Second)
			continue
		}
		if acct == nil {
			ff.logger.Debugf("No feeds to check; sleeping %d seconds", feedCheckLoopIdleWakeSec)
			time.Sleep(feedCheckLoopIdleWakeSec * time.Second)
			continue
		}
		lastUpdated := acct.FeedLastUpdated
		err = ff.updateFeed(acct)
		if err != nil {
			ff.logger.Errorf("Error updating feed: %s: %v", acct.Handle, err)
			// Reschedule for updating as if there was no new post
			nextCheckDue := ff.getNextCheckTime(lastUpdated)
			if err = ff.repo.UpdateAccountFeedTimes(acct.Id, lastUpdated, nextCheckDue); err != nil {
				ff.logger.Errorf("Failed to reschedule for checking after error: %s: %v", acct.Handle, err)
			}
		}
		// If no error, updateFeed has set next due date for checking
	}
}
