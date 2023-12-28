package server

import (
	"encoding/json"
	"fmt"
	"github.com/gorilla/mux"
	"net/http"
	"regexp"
	"rss_parrot/dto"
	"rss_parrot/logic"
	"rss_parrot/shared"
)

// Groups together the handlers needed to implement an ActivityPub server.
type apubHandlerGroup struct {
	cfg        *shared.Config
	logger     shared.ILogger
	sender     logic.IActivitySender
	sigChecker logic.IHttpSigChecker
	udir       logic.IUserDirectory
	inbox      logic.IInbox
	reResource *regexp.Regexp
}

func NewApubHandlerGroup(
	cfg *shared.Config,
	logger shared.ILogger,
	sender logic.IActivitySender,
	sigChecker logic.IHttpSigChecker,
	udir logic.IUserDirectory,
	ibox logic.IInbox,
) IHandlerGroup {
	res := apubHandlerGroup{
		cfg:        cfg,
		logger:     logger,
		sender:     sender,
		sigChecker: sigChecker,
		udir:       udir,
		inbox:      ibox,
	}
	res.reResource = regexp.MustCompile("^acct:([^@]+)@([^@]+)$")
	return &res
}

func (hg *apubHandlerGroup) Prefix() string {
	return ""
}

func (hg *apubHandlerGroup) GroupDefs() []handlerDef {
	return []handlerDef{
		{"GET", "/.well-known/webfinger", func(w http.ResponseWriter, r *http.Request) { hg.getWebfinger(w, r) }},
		{"GET", "/u/{user}", func(w http.ResponseWriter, r *http.Request) { hg.getUser(w, r) }},
		{"GET", "/u/{user}/outbox", func(w http.ResponseWriter, r *http.Request) { hg.getUserOutbox(w, r) }},
		{"GET", "/u/{user}/followers", func(w http.ResponseWriter, r *http.Request) { hg.getUserFollowers(w, r) }},
		{"GET", "/u/{user}/following", func(w http.ResponseWriter, r *http.Request) { hg.getUserFollowing(w, r) }},
		{"POST", "/u/{user}/inbox", func(w http.ResponseWriter, r *http.Request) { hg.postInbox(w, r) }},
		{"POST", "/inbox", func(w http.ResponseWriter, r *http.Request) { hg.postInbox(w, r) }},
	}
}

func (hg *apubHandlerGroup) AuthMW() func(next http.Handler) http.Handler {
	return emptyMW
}

func (hg *apubHandlerGroup) getWebfinger(w http.ResponseWriter, r *http.Request) {

	hg.logger.Infof("Handling webfinger GET: %s", r.URL.Path)

	resourceParam := r.URL.Query().Get("resource")
	groups := hg.reResource.FindStringSubmatch(resourceParam)
	if groups == nil {
		hg.logger.Infof("Webfinger: Invalid request; 'resource' param is '%s'", resourceParam)
		writeErrorResponse(w, "Missing or invalid 'resource' param", http.StatusBadRequest)
		return
	}
	user, host := groups[1], groups[2]
	_ = host // TODO check is host for us?

	resp := hg.udir.GetWebfinger(user)

	if resp == nil {
		hg.logger.Infof("Webfinger: No such resource; 'resource' param is '%s'", resourceParam)
		writeErrorResponse(w, "No such resource", http.StatusNotFound)
		return
	}

	writeJsonResponse(hg.logger, w, resp)
}

func (hg *apubHandlerGroup) getUser(w http.ResponseWriter, r *http.Request) {

	hg.logger.Infof("Handling user GET: %s", r.URL.Path)
	userName := mux.Vars(r)["user"]

	userInfo := hg.udir.GetUserInfo(userName)

	if userInfo == nil {
		hg.logger.Infof("Info requested for unknown user: '%s'", userName)
		writeErrorResponse(w, "No such user", http.StatusNotFound)
		return
	}

	writeJsonResponse(hg.logger, w, userInfo)
}

func (hg *apubHandlerGroup) getUserOutbox(w http.ResponseWriter, r *http.Request) {

	hg.logger.Infof("Handling user outbox GET: %s", r.URL.Path)
	userName := mux.Vars(r)["user"]
	summary := hg.udir.GetOutboxSummary(userName)
	if summary == nil {
		hg.logger.Infof("Outbox requested for unknown user: '%s'", userName)
		writeErrorResponse(w, "No such user", http.StatusNotFound)
		return
	}
	writeJsonResponse(hg.logger, w, summary)
}

func (hg *apubHandlerGroup) getUserFollowers(w http.ResponseWriter, r *http.Request) {

	hg.logger.Infof("Handling user followers GET: %s", r.URL.Path)
	userName := mux.Vars(r)["user"]
	summary := hg.udir.GetFollowersSummary(userName)
	if summary == nil {
		hg.logger.Infof("Followers requested for unknown user: '%s'", userName)
		writeErrorResponse(w, "No such user", http.StatusNotFound)
		return
	}
	writeJsonResponse(hg.logger, w, summary)
}

func (hg *apubHandlerGroup) getUserFollowing(w http.ResponseWriter, r *http.Request) {

	hg.logger.Infof("Handling user following GET: %s", r.URL.Path)
	userName := mux.Vars(r)["user"]
	summary := hg.udir.GetFollowingSummary(userName)
	if summary == nil {
		hg.logger.Infof("Following requested for unknown user: '%s'", userName)
		writeErrorResponse(w, "No such user", http.StatusNotFound)
		return
	}
	writeJsonResponse(hg.logger, w, summary)
}

func (hg *apubHandlerGroup) postInbox(w http.ResponseWriter, r *http.Request) {

	var err error
	hg.logger.Infof("Handling user inbox POST: %s", r.URL.Path)
	userName := mux.Vars(r)["user"]
	bodyBytes := readBody(hg.logger, w, r)
	if bodyBytes == nil {
		hg.logger.Info("Empty request body")
		writeErrorResponse(w, "Request body must not be empty", http.StatusBadRequest)
		return
	}

	// DBG
	//hg.logger.Debug(string(bodyBytes))

	// First, parse a rudimentary version of the activity to check signature, find out activity type
	var act dto.ActivityInBase
	if err = json.Unmarshal(bodyBytes, &act); err != nil {
		hg.logger.Infof("Invalid JSON in request body: %v: %s", err, string(bodyBytes))
		writeErrorResponse(w, "Request body is not valid JSON", http.StatusBadRequest)
		return
	}

	// Verify signature
	var senderInfo *dto.UserInfo
	var sigProblem string
	senderInfo, sigProblem, err = hg.sigChecker.Check(w, r)

	if err != nil {
		hg.logger.Errorf("Unexpected error trying to verify signature: %v", err)
		writeErrorResponse(w, internalErrorStr, http.StatusInternalServerError)
		return
	}

	if sigProblem != "" {
		if act.Type == "Delete" {
			hg.logger.Infof("Ignoring Delete request with unverified actor signature")
			writeJsonResponse(hg.logger, w, "OK")
		} else {
			hg.logger.Warnf("Incorrectly signed inbox POST request: %s", sigProblem)
			msg := fmt.Sprintf("Invalid HTTP signature: %s", sigProblem)
			writeErrorResponse(w, msg, http.StatusUnauthorized)
		}
		return
	}

	// Does signer match actor?
	if senderInfo.Id != act.Actor {
		hg.logger.Warnf("Activity signed by %s, but actor is %s", senderInfo.Id, act.Actor)
		writeErrorResponse(w, "Signer does not match actor", http.StatusUnauthorized)
	}

	hg.processActivity(userName, bodyBytes, senderInfo, act, w)
}

func (hg *apubHandlerGroup) processActivity(
	userName string,
	bodyBytes []byte,
	senderInfo *dto.UserInfo,
	act dto.ActivityInBase,
	w http.ResponseWriter,
) {

	var err error

	// Find out Object's type if object field is, well, an object
	// If yes, grab object type field
	objectType := ""
	if objMap, ok := act.Object.(map[string]interface{}); ok {
		if objTypeStr, ok := objMap["type"].(string); ok {
			objectType = objTypeStr
		}
	}

	// Handle different activities
	var reqProblem string
	if act.Type == "Follow" {
		reqProblem, err = hg.inbox.HandleFollow(userName, senderInfo, bodyBytes)
	} else if act.Type == "Undo" {
		reqProblem, err = hg.inbox.HandleUndo(userName, senderInfo, bodyBytes)
	} else if act.Type == "Create" {
		if objectType == "Note" {
			reqProblem, err = hg.inbox.HandleCreateNote(act, senderInfo, bodyBytes)
		}
	}

	if err != nil {
		hg.logger.Errorf("Error handling inbox activity: %v", err)
		writeErrorResponse(w, internalErrorStr, http.StatusInternalServerError)
		return
	}

	if reqProblem != "" {
		hg.logger.Infof("Invalid '%s' request: %s", act.Type, reqProblem)
		msg := fmt.Sprintf("Bad request: %s", reqProblem)
		writeErrorResponse(w, msg, http.StatusBadRequest)
		return
	}

	writeJsonResponse(hg.logger, w, "OK")
}
