package models

const (
	MsgStreamEvents  = "astro_events"
	MsgStreamDLQ     = "astro_dlq"
	MsgProfileWrk    = "astro-profile-worker"
	MsgRecommendWrk  = "astro-recommend-worker"
	MsgSMaxRetries   = 5
	MsgProfileSubj   = "astro.events.profile"
	MsgRecommendSubj = "astro.events.recommend"
)
