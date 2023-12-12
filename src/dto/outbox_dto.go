package dto

type OutboxSummary struct {
	Context    any    `json:"@context"`
	Id         string `json:"id"`
	Type       string `json:"type"`
	TotalItems uint   `json:"totalItems"`
	First      string `json:"first"`
	Last       string `json:"last"`
}
