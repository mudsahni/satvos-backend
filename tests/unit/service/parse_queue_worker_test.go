package service_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"

	"satvos/internal/domain"
	"satvos/internal/service"
	"satvos/mocks"
)

func TestParseQueueWorker_PollsAndDispatchesParsing(t *testing.T) {
	docRepo := new(mocks.MockDocumentRepo)
	docSvc := new(mocks.MockDocumentService)

	tenantID := uuid.New()
	docID := uuid.New()
	fileID := uuid.New()

	doc := domain.Document{
		ID: docID, TenantID: tenantID, FileID: fileID,
		DocumentType:     "invoice",
		ParseAttempts:    1,
		ParsingStatus:    domain.ParsingStatusProcessing,
		StructuredData:   json.RawMessage("{}"),
		ConfidenceScores: json.RawMessage("{}"),
	}

	// First poll returns one doc, subsequent polls return empty
	docRepo.On("ResetStaleProcessing", mock.Anything, mock.AnythingOfType("time.Time")).Return(0, nil).Maybe()
	docRepo.On("ClaimQueued", mock.Anything, mock.AnythingOfType("int")).
		Return([]domain.Document{doc}, nil).Once()
	docRepo.On("ClaimQueued", mock.Anything, mock.AnythingOfType("int")).
		Return([]domain.Document{}, nil).Maybe()

	docSvc.On("ParseDocument", mock.Anything, mock.AnythingOfType("*domain.Document"), 5).
		Return().Maybe()

	cfg := service.ParseQueueConfig{
		PollInterval: 50 * time.Millisecond,
		MaxRetries:   5,
		Concurrency:  2,
	}
	worker := service.NewParseQueueWorker(docRepo, docSvc, cfg)

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		worker.Start(ctx)
		close(done)
	}()

	// Wait for at least one poll cycle
	time.Sleep(200 * time.Millisecond)
	cancel()
	<-done

	docRepo.AssertCalled(t, "ClaimQueued", mock.Anything, mock.AnythingOfType("int"))
	docSvc.AssertCalled(t, "ParseDocument", mock.Anything, mock.AnythingOfType("*domain.Document"), 5)
}

func TestParseQueueWorker_RespectsConcurrencyCap(t *testing.T) {
	docRepo := new(mocks.MockDocumentRepo)
	docSvc := new(mocks.MockDocumentService)

	cfg := service.ParseQueueConfig{
		PollInterval: 50 * time.Millisecond,
		MaxRetries:   5,
		Concurrency:  2,
	}

	docRepo.On("ResetStaleProcessing", mock.Anything, mock.AnythingOfType("time.Time")).Return(0, nil).Maybe()
	// Return empty to verify the limit parameter
	docRepo.On("ClaimQueued", mock.Anything, mock.AnythingOfType("int")).
		Return([]domain.Document{}, nil).Maybe()
	docSvc.On("ParseDocument", mock.Anything, mock.AnythingOfType("*domain.Document"), 5).
		Return().Maybe()

	worker := service.NewParseQueueWorker(docRepo, docSvc, cfg)

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		worker.Start(ctx)
		close(done)
	}()

	time.Sleep(150 * time.Millisecond)
	cancel()
	<-done

	// Verify ClaimQueued was called with limit <= concurrency
	for _, call := range docRepo.Calls {
		if call.Method == "ClaimQueued" {
			limit := call.Arguments.Get(1).(int)
			assert.LessOrEqual(t, limit, cfg.Concurrency)
		}
	}
}

func TestParseQueueWorker_CleanShutdown(t *testing.T) {
	docRepo := new(mocks.MockDocumentRepo)
	docSvc := new(mocks.MockDocumentService)

	docRepo.On("ResetStaleProcessing", mock.Anything, mock.AnythingOfType("time.Time")).Return(0, nil).Maybe()
	docRepo.On("ClaimQueued", mock.Anything, mock.AnythingOfType("int")).
		Return([]domain.Document{}, nil).Maybe()

	cfg := service.ParseQueueConfig{
		PollInterval: 50 * time.Millisecond,
		MaxRetries:   5,
		Concurrency:  5,
	}
	worker := service.NewParseQueueWorker(docRepo, docSvc, cfg)

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		worker.Start(ctx)
		close(done)
	}()

	// Cancel immediately
	cancel()

	select {
	case <-done:
		// Success — Start returned promptly
	case <-time.After(2 * time.Second):
		t.Fatal("Start did not return after context cancellation")
	}
}

func TestParseQueueWorker_EmptyQueueDoesNothing(t *testing.T) {
	docRepo := new(mocks.MockDocumentRepo)
	docSvc := new(mocks.MockDocumentService)

	docRepo.On("ResetStaleProcessing", mock.Anything, mock.AnythingOfType("time.Time")).Return(0, nil).Maybe()
	docRepo.On("ClaimQueued", mock.Anything, mock.AnythingOfType("int")).
		Return([]domain.Document{}, nil).Maybe()

	cfg := service.ParseQueueConfig{
		PollInterval: 50 * time.Millisecond,
		MaxRetries:   5,
		Concurrency:  5,
	}
	worker := service.NewParseQueueWorker(docRepo, docSvc, cfg)

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		worker.Start(ctx)
		close(done)
	}()

	time.Sleep(200 * time.Millisecond)
	cancel()
	<-done

	// ParseDocument should never have been called
	docSvc.AssertNotCalled(t, "ParseDocument", mock.Anything, mock.Anything, mock.Anything)
}

func TestParseQueueWorker_ClaimQueuedError(t *testing.T) {
	docRepo := new(mocks.MockDocumentRepo)
	docSvc := new(mocks.MockDocumentService)

	docRepo.On("ResetStaleProcessing", mock.Anything, mock.AnythingOfType("time.Time")).Return(0, nil).Maybe()
	// Return an error on poll
	docRepo.On("ClaimQueued", mock.Anything, mock.AnythingOfType("int")).
		Return(nil, errors.New("db connection error")).Maybe()

	cfg := service.ParseQueueConfig{
		PollInterval: 50 * time.Millisecond,
		MaxRetries:   5,
		Concurrency:  5,
	}
	worker := service.NewParseQueueWorker(docRepo, docSvc, cfg)

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		worker.Start(ctx)
		close(done)
	}()

	// Let a few poll cycles happen with errors
	time.Sleep(200 * time.Millisecond)
	cancel()

	select {
	case <-done:
		// Success — no panic, no goroutine leak
	case <-time.After(2 * time.Second):
		t.Fatal("Start did not return after context cancellation")
	}

	// ParseDocument should never have been called
	docSvc.AssertNotCalled(t, "ParseDocument", mock.Anything, mock.Anything, mock.Anything)
}
