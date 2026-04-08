package models

import (
	"time"
)

type StreamCfg struct {
	Name         string
	Retention    RetentionPolicy
	Subjects     []string
	MaxConsumers int
	MaxMessages  int64
	MaxBytes     int64
	Duplicates   Duration
	Storage      StorageType
	Replicas     int
}

type RetentionPolicy int

const (
	LimitsPolicy RetentionPolicy = iota
	WorkQueuePolicy
	SamplePolicy
)

type Consumer struct {
	Name          string
	StreamName    string
	AckPolicy     AckPolicy
	MaxDeliver    int
	Durable       bool
	ReplayPolicy  ReplayPolicy
	AckWait       Duration
	MaxAckPending int
	BackOff       []Duration
}

type AckPolicy int

const (
	AckNone AckPolicy = iota
	AckAll
	AckExplicit
)

type ReplayPolicy int

const (
	ReplayInstant ReplayPolicy = iota
	ReplayOriginal
)

type Duration time.Duration

type StorageType int

const (
	FileStorage StorageType = iota
	MemoryStorage
)
