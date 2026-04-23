package nats_test

import (
	"context"
	"encoding/json"
	"net"
	"sync/atomic"
	"testing"
	"time"

	"astroapi/config"
	natsinfra "astroapi/internal/infrastructure/nats"
	"astroapi/internal/models"

	"github.com/nats-io/nats-server/v2/server"
	natsgo "github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

// startEmbeddedNATS поднимает in-process NATS server с JetStream для тестов.
func startEmbeddedNATS(t *testing.T) (*server.Server, string, string) {
	t.Helper()

	l, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	host, portStr, err := net.SplitHostPort(l.Addr().String())
	require.NoError(t, err)
	require.NoError(t, l.Close())

	opts := &server.Options{
		Host:      host,
		Port:      parsePort(t, portStr),
		JetStream: true,
		StoreDir:  t.TempDir(),
		NoLog:     true,
		NoSigs:    true,
	}
	ns, err := server.NewServer(opts)
	require.NoError(t, err)
	go ns.Start()
	t.Cleanup(func() {
		ns.Shutdown()
		ns.WaitForShutdown()
	})

	require.Truef(t, ns.ReadyForConnections(5*time.Second), "embedded NATS not ready")
	return ns, host, portStr
}

func parsePort(t *testing.T, s string) int {
	t.Helper()
	p, err := net.LookupPort("tcp", s)
	require.NoError(t, err)
	return p
}

func bootstrapPipeline(t *testing.T) (*natsinfra.JetStreamAdapter, *natsinfra.MessagePublisher, *natsgo.Conn) {
	t.Helper()
	_, host, port := startEmbeddedNATS(t)

	logger := zap.NewNop()
	cfg := &config.Config{NATSHost: host, NATSPort: port}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	conn, err := natsinfra.InitNATS(ctx, logger, cfg)
	require.NoError(t, err)
	t.Cleanup(func() { conn.DrainNATS() })

	js, err := jetstream.New(conn.Conn)
	require.NoError(t, err)

	adapter := natsinfra.NewJetStreamRepository(js, logger)
	require.NoError(t, adapter.InitializeStreams(ctx))

	return adapter, natsinfra.NewMessagePublisher(adapter, logger), conn.Conn
}

// TestPipeline_PublishFlatSubject_ReachesConsumer — TDD regression test
// на баг с FilterSubjects: handlers публикуют в astro.events.profile,
// consumer должен доставить сообщение воркеру.
func TestPipeline_PublishFlatSubject_ReachesConsumer(t *testing.T) {
	adapter, publisher, _ := bootstrapPipeline(t)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	var received atomic.Int32
	done := make(chan []byte, 1)

	consumer := natsinfra.NewMessageConsumer(adapter, zap.NewNop())
	err := consumer.ConsumeWithHandler(ctx, models.MsgStreamEvents, models.MsgProfileWrk,
		func(_ context.Context, msg jetstream.Msg) error {
			if received.Add(1) == 1 {
				done <- msg.Data()
			}
			return nil
		},
	)
	require.NoError(t, err)

	payload := map[string]string{"request_id": "req-1", "user_id": "u-1"}
	require.NoError(t, publisher.PublishMessage(ctx, models.MsgStreamEvents, models.MsgProfileSubj, payload))

	select {
	case data := <-done:
		var got map[string]string
		require.NoError(t, json.Unmarshal(data, &got))
		require.Equal(t, "req-1", got["request_id"])
	case <-time.After(3 * time.Second):
		t.Fatalf("consumer did not receive message within timeout (filter mismatch?)")
	}
}

func TestPipeline_RecommendFlatSubject_ReachesConsumer(t *testing.T) {
	adapter, publisher, _ := bootstrapPipeline(t)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	done := make(chan struct{}, 1)
	consumer := natsinfra.NewMessageConsumer(adapter, zap.NewNop())
	require.NoError(t, consumer.ConsumeWithHandler(ctx, models.MsgStreamEvents, models.MsgRecommendWrk,
		func(_ context.Context, _ jetstream.Msg) error {
			done <- struct{}{}
			return nil
		},
	))

	require.NoError(t, publisher.PublishMessage(ctx, models.MsgStreamEvents, models.MsgRecommendSubj, map[string]string{"k": "v"}))

	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("recommend consumer did not receive message")
	}
}

func TestPipeline_PublishToDLQStreamIsRejected(t *testing.T) {
	_, publisher, _ := bootstrapPipeline(t)

	err := publisher.PublishMessage(context.Background(), models.MsgStreamDLQ, "astro.dlq.profile", map[string]string{})
	require.Error(t, err)
	require.Contains(t, err.Error(), "wrong stream name")
}
