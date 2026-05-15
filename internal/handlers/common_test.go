package handlers

import (
	"astroapi/internal/requests"
	"context"
)

var validUserID = "123e4567-e89b-12d3-a456-426614174000"

type mockRequestsRepo struct {
	requests.Repository
	mockGet    func(ctx context.Context, id string) (requests.Request, error)
	mockCreate func(ctx context.Context, req requests.Request) error
}

func (m *mockRequestsRepo) Get(ctx context.Context, id string) (requests.Request, error) {
	if m.mockGet != nil {
		return m.mockGet(ctx, id)
	}
	return requests.Request{}, nil
}

func (m *mockRequestsRepo) Create(ctx context.Context, req requests.Request) error {
	if m.mockCreate != nil {
		return m.mockCreate(ctx, req)
	}
	return nil
}

type mockMsgPublisher struct {
	mockPublish func(ctx context.Context, streamName, subject string, payload any) error
}

func (m *mockMsgPublisher) PublishMessage(ctx context.Context, streamName, subject string, payload any) error {
	if m.mockPublish != nil {
		return m.mockPublish(ctx, streamName, subject, payload)
	}
	return nil
}
