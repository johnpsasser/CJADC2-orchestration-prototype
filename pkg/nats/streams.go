// Package natsutil provides NATS JetStream configuration and helpers
package natsutil

import (
	"context"
	"time"

	"github.com/nats-io/nats.go/jetstream"
)

// StreamConfigs defines all streams used by the CJADC2 platform
var StreamConfigs = map[string]jetstream.StreamConfig{
	"DETECTIONS": {
		Name:              "DETECTIONS",
		Description:       "Raw sensor detection events",
		Subjects:          []string{"detect.>"},
		Retention:         jetstream.LimitsPolicy,
		MaxBytes:          1 * 1024 * 1024 * 1024, // 1GB
		MaxAge:            24 * time.Hour,
		Storage:           jetstream.FileStorage,
		Replicas:          1,
		Discard:           jetstream.DiscardOld,
		MaxMsgsPerSubject: 100000,
	},
	"TRACKS": {
		Name:        "TRACKS",
		Description: "Classified and correlated tracks",
		Subjects:    []string{"track.>"},
		Retention:   jetstream.LimitsPolicy,
		MaxBytes:    2 * 1024 * 1024 * 1024, // 2GB
		MaxAge:      72 * time.Hour,
		Storage:     jetstream.FileStorage,
		Replicas:    1,
		Discard:     jetstream.DiscardOld,
	},
	"PROPOSALS": {
		Name:        "PROPOSALS",
		Description: "Action proposals awaiting human approval",
		Subjects:    []string{"proposal.>"},
		Retention:   jetstream.WorkQueuePolicy, // Consume once
		MaxBytes:    512 * 1024 * 1024,         // 512MB
		MaxAge:      1 * time.Hour,
		Storage:     jetstream.FileStorage,
		Replicas:    1,
	},
	"DECISIONS": {
		Name:        "DECISIONS",
		Description: "Human decisions on proposals",
		Subjects:    []string{"decision.>"},
		Retention:   jetstream.LimitsPolicy,
		MaxBytes:    1 * 1024 * 1024 * 1024,
		MaxAge:      7 * 24 * time.Hour,
		Storage:     jetstream.FileStorage,
		Replicas:    1,
	},
	"EFFECTS": {
		Name:        "EFFECTS",
		Description: "Executed effect logs",
		Subjects:    []string{"effect.>"},
		Retention:   jetstream.LimitsPolicy,
		MaxBytes:    512 * 1024 * 1024,
		MaxAge:      30 * 24 * time.Hour,
		Storage:     jetstream.FileStorage,
		Replicas:    1,
	},
}

// ConsumerConfigs defines consumers for each agent type
var ConsumerConfigs = map[string]jetstream.ConsumerConfig{
	"classifier": {
		Durable:       "classifier",
		Description:   "Classifier agent consumer for detection events",
		FilterSubject: "detect.>",
		AckPolicy:     jetstream.AckExplicitPolicy,
		AckWait:       30 * time.Second,
		MaxDeliver:    3,
		MaxAckPending: 1000,
	},
	"correlator": {
		Durable:       "correlator",
		Description:   "Correlator agent consumer for classified tracks",
		FilterSubject: "track.classified.>",
		AckPolicy:     jetstream.AckExplicitPolicy,
		AckWait:       30 * time.Second,
		MaxDeliver:    3,
		MaxAckPending: 500,
	},
	"planner": {
		Durable:       "planner",
		Description:   "Planner agent consumer for correlated tracks",
		FilterSubject: "track.correlated.>",
		AckPolicy:     jetstream.AckExplicitPolicy,
		AckWait:       30 * time.Second,
		MaxDeliver:    3,
		MaxAckPending: 200,
	},
	"authorizer": {
		Durable:       "authorizer",
		Description:   "Authorizer agent consumer for proposals",
		FilterSubject: "proposal.>",
		AckPolicy:     jetstream.AckExplicitPolicy,
		AckWait:       300 * time.Second, // Longer wait for human decisions
		MaxDeliver:    1,                 // No retry for human decisions
		MaxAckPending: 100,
	},
	"effector": {
		Durable:       "effector",
		Description:   "Effector agent consumer for approved decisions",
		FilterSubject: "decision.approved.>",
		AckPolicy:     jetstream.AckExplicitPolicy,
		AckWait:       60 * time.Second,
		MaxDeliver:    5, // Higher retry for effects
		MaxAckPending: 50,
	},
}

// SetupStreams creates all required streams
func SetupStreams(ctx context.Context, js jetstream.JetStream) error {
	for name, cfg := range StreamConfigs {
		_, err := js.Stream(ctx, name)
		if err == nil {
			continue // Stream exists
		}

		_, err = js.CreateStream(ctx, cfg)
		if err != nil {
			return err
		}
	}
	return nil
}

// SetupConsumer creates a consumer for an agent
func SetupConsumer(ctx context.Context, js jetstream.JetStream, streamName, consumerName string) (jetstream.Consumer, error) {
	cfg, ok := ConsumerConfigs[consumerName]
	if !ok {
		cfg = jetstream.ConsumerConfig{
			Durable:       consumerName,
			AckPolicy:     jetstream.AckExplicitPolicy,
			AckWait:       30 * time.Second,
			MaxDeliver:    3,
			MaxAckPending: 100,
		}
	}

	stream, err := js.Stream(ctx, streamName)
	if err != nil {
		return nil, err
	}

	consumer, err := stream.Consumer(ctx, cfg.Durable)
	if err == nil {
		return consumer, nil
	}

	return stream.CreateConsumer(ctx, cfg)
}
