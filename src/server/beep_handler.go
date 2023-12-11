package server

import (
	"fmt"
	"log"
	"net/http"
	"rss_parrot/dto"
	"rss_parrot/logic"
)

type BeepHandler struct {
	sender logic.IActivitySender
}

func NewBeepHandler(sender logic.IActivitySender) *BeepHandler {
	return &BeepHandler{sender}
}

func (*BeepHandler) Def() (string, string) {
	return "GET", "/beep"
}

func (h *BeepHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {

	log.Printf("Beep: Request received")

	activity := dto.Activity{
		Context: "https://www.w3.org/ns/activitystreams",
		Id:      "https://rss-parrot.zydeo.net/users/birb/statuses/43/activity",
		Type:    "Create",
		Actor:   "https://rss-parrot.zydeo.net/users/birb",
		Object: dto.Note{
			Id:           "https://rss-parrot.zydeo.net/users/birb/statuses/43",
			Type:         "Note",
			Published:    "2023-12-10T21:15:31Z",
			AttributedTo: "https://rss-parrot.zydeo.net/users/birb",
			Content:      "<p><span class='h-card' translate='no'><a href='https://toot.community/@gaborparrot' class='u-url mention'>@<span>gaborparrot</span></a></span> Brezel boom boom</p>",
			To:           []string{"https://toot.community/users/gaborparrot"},
			Cc:           []string{},
		},
	}

	//activity := dto.Activity{
	//	Context: "https://www.w3.org/ns/activitystreams",
	//	Id:      "https://rss-parrot.zydeo.net/follow-42",
	//	Type:    "Follow",
	//	Actor:   "https://rss-parrot.zydeo.net/users/birb",
	//	Object:  "https://toot.community/users/gaborparrot",
	//}

	err := h.sender.Send("https://toot.community/inbox", activity)
	if err != nil {
		w.WriteHeader(500)
		fmt.Fprintln(w, "Failed to post activity")
		fmt.Fprintln(w, err)
	}

	fmt.Fprintln(w, "Activity posted")
}
