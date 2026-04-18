package models

const (
	MsgStreamEvents  = "astro_events"
	MsgStreamDLQ     = "astro_dlq"
	MsgProfileWrk    = "astro-profile-worker"
	MsgRecommendWrk  = "astro-recommend-worker"
	MsgDQLViewer     = "astro-dlq-viewer"
	MsgSMaxRetries   = 5
	MsgProfileSubj   = "astro.events.profile"
	MsgRecommendSubj = "astro.events.recommend"
)

type Message struct {
	Subject string
	Headers map[string][]string
	Data    []byte
}
