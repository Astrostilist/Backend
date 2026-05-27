package models

import "encoding/json"

const (
	MsgStreamEvents  = "astro_events"
	MsgStreamDLQ     = "astro_dlq"
	MsgProfileWrk    = "astro-profile-worker"
	MsgRecommendWrk  = "astro-recommend-worker"
	MsgDLQViewer     = "astro-dlq-viewer"
	MsgDLQSubj       = "astro.dlq."
	MsgSMaxRetries   = 5
	MsgProfileSubj   = "astro.events.profile"
	MsgRecommendSubj = "astro.events.recommend"
)

type MessageWithTrace struct {
	TraceContext map[string]string `json:"trace_context"`
	Payload      json.RawMessage   `json:"payload"`
}

type Message struct {
	Subject string              `json:"subject"`
	Headers map[string][]string `json:"headers"`
	Data    string              `json:"data"`
}

var DLQSubjectMap = map[string]string{
	MsgProfileSubj:   "astro.dlq.profile",
	MsgRecommendSubj: "astro.dlq.recommend"}
