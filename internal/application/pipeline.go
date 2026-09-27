// Package application connects collection, item state, and notification delivery.
package application

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"sync"
	"unicode/utf8"

	"github.com/suwa68/signal-watch/internal/config"
	"github.com/suwa68/signal-watch/internal/monitoring"
	"github.com/suwa68/signal-watch/internal/notification"
	"github.com/suwa68/signal-watch/internal/source"
	"github.com/suwa68/signal-watch/internal/state"
)

// ErrRunInProgress indicates another run owns this pipeline, before collection.
var ErrRunInProgress = errors.New("pipeline run already in progress")

// Collector normalizes source data without exposing adapter implementation details.
type Collector interface {
	Run(context.Context, config.SourceConfig) ([]source.MonitorItem, error)
}

// NotificationSender delivers to the destination bound by application assembly.
type NotificationSender interface {
	Send(context.Context, notification.Notification) error
}

// NotificationSenderFunc binds an existing delivery API to a stable destination.
type NotificationSenderFunc func(context.Context, notification.Notification) error

func (f NotificationSenderFunc) Send(ctx context.Context, message notification.Notification) error {
	return f(ctx, message)
}

// Pipeline reuses one store for discovery and successful delivery completion.
// Reuse the pipeline across calls and keep its destination binding stable.
// A Pipeline must not be copied after first use. Its guard coordinates only
// this instance, not other pipelines that happen to share the same store.
type Pipeline struct {
	collector Collector
	store     state.StateStore
	processor *monitoring.Processor
	sender    NotificationSender
	run       sync.Mutex
}

func NewPipeline(collector Collector, store state.StateStore, sender NotificationSender) (*Pipeline, error) {
	for _, dependency := range []struct {
		name  string
		value any
	}{
		{"collector", collector},
		{"state store", store},
		{"notification sender", sender},
	} {
		if nilDependency(dependency.value) {
			return nil, fmt.Errorf("%s is required", dependency.name)
		}
	}
	return &Pipeline{
		collector: collector,
		store:     store,
		processor: monitoring.NewProcessor(store),
		sender:    sender,
	}, nil
}

// RunOnce silently establishes the first successful baseline, then sends later
// unseen items sequentially and marks each only after successful delivery.
// Item failures are joined and later items continue while the caller's ctx is
// active. Sender-local cancellation/timeout errors alone never stop the run.
func (p *Pipeline) RunOnce(ctx context.Context, sourceConfig config.SourceConfig) (result error) {
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("source %q: caller context: %w", sourceConfig.ID, err)
	}
	if strings.TrimSpace(sourceConfig.ID) == "" {
		return fmt.Errorf("source %q: validate config: source ID is required", sourceConfig.ID)
	}
	if err := validateText("source name", sourceConfig.Name); err != nil {
		return fmt.Errorf("source %q: validate config: %w", sourceConfig.ID, err)
	}
	if p == nil || p.processor == nil {
		return fmt.Errorf("source %q: pipeline is not initialized", sourceConfig.ID)
	}
	if !p.run.TryLock() {
		return fmt.Errorf("source %q: %w", sourceConfig.ID, ErrRunInProgress)
	}
	defer p.run.Unlock()
	// Check on every exit, including a final failed send or an empty result.
	// Preserve stage/item errors even when they do not wrap caller cancellation.
	defer func() {
		if err := ctx.Err(); err != nil {
			result = errors.Join(result, fmt.Errorf("source %q: caller context: %w", sourceConfig.ID, err))
		}
	}()

	items, err := p.collector.Run(ctx, sourceConfig)
	if err != nil {
		return fmt.Errorf("source %q: collect: %w", sourceConfig.ID, err)
	}
	unseen, err := p.processor.ProcessSuccessfulCollection(ctx, sourceConfig.ID, items)
	if err != nil {
		return fmt.Errorf("source %q: process collection: %w", sourceConfig.ID, err)
	}
	var failures []error
	for _, candidate := range unseen {
		if ctx.Err() != nil {
			break
		}
		message, err := mapNotification(sourceConfig.Name, candidate.Item)
		if err != nil {
			failures = append(failures, fmt.Errorf("source %q item %q: map notification: %w", sourceConfig.ID, candidate.Key, err))
			continue
		}
		if err := p.sender.Send(ctx, message); err != nil {
			failures = append(failures, fmt.Errorf("source %q item %q: send notification: %w", sourceConfig.ID, candidate.Key, err))
			continue
		}
		if err := p.store.MarkItemSeen(ctx, sourceConfig.ID, candidate.Key); err != nil {
			failures = append(failures, fmt.Errorf("source %q item %q: mark seen: %w", sourceConfig.ID, candidate.Key, err))
		}
	}
	return errors.Join(failures...)
}

func mapNotification(sourceName string, item source.MonitorItem) (notification.Notification, error) {
	if err := validateText("title", item.Title); err != nil {
		return notification.Notification{}, err
	}
	if err := validateText("source name", sourceName); err != nil {
		return notification.Notification{}, err
	}
	return notification.Notification{
		Title:       item.Title,
		SourceName:  sourceName,
		URL:         item.URL,
		PublishedAt: item.PublishedAt,
	}, nil
}

func validateText(name, value string) error {
	if strings.TrimSpace(value) == "" {
		return fmt.Errorf("%s is required", name)
	}
	if !utf8.ValidString(value) {
		return fmt.Errorf("%s must be valid UTF-8", name)
	}
	return nil
}

// Interfaces containing nil pointers or function adapters are also missing.
func nilDependency(value any) bool {
	if value == nil {
		return true
	}
	v := reflect.ValueOf(value)
	switch v.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return v.IsNil()
	default:
		return false
	}
}
