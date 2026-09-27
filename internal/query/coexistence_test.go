package query_test

import (
	"context"
	"reflect"
	"testing"

	"github.com/suwa68/signal-watch/internal/application"
	"github.com/suwa68/signal-watch/internal/config"
	"github.com/suwa68/signal-watch/internal/notification"
	"github.com/suwa68/signal-watch/internal/query"
	"github.com/suwa68/signal-watch/internal/source"
	"github.com/suwa68/signal-watch/internal/state"
)

type changingCollector struct{ items []source.MonitorItem }

func (c *changingCollector) Run(context.Context, config.SourceConfig) ([]source.MonitorItem, error) {
	return append([]source.MonitorItem(nil), c.items...), nil
}

func TestQueryDoesNotConsumeMonitoringItems(t *testing.T) {
	t.Parallel()
	definition := query.Definition{Config: config.SourceConfig{ID: "news", Name: "News", Type: "fake"}, DefinitionRevision: "r1"}
	collector := &changingCollector{items: []source.MonitorItem{
		{SourceID: "news", ExternalID: "a", Title: "A"},
		{SourceID: "news", ExternalID: "b", Title: "B"},
	}}
	service, _ := query.NewService(collector)
	store := state.NewMemoryStateStore()
	var sent []string
	pipeline, err := application.NewPipeline(collector, store, application.NotificationSenderFunc(func(_ context.Context, message notification.Notification) error {
		sent = append(sent, message.Title)
		return nil
	}))
	if err != nil {
		t.Fatal(err)
	}

	if snapshot, err := service.Collect(context.Background(), definition, query.DefaultLimits()); err != nil || len(snapshot.Items) != 2 {
		t.Fatalf("first query = %#v, %v", snapshot, err)
	}
	if err := pipeline.RunOnce(context.Background(), definition.Config); err != nil {
		t.Fatal(err)
	}
	if len(sent) != 0 {
		t.Fatalf("baseline sent %v", sent)
	}

	collector.items = append(collector.items, source.MonitorItem{SourceID: "news", ExternalID: "c", Title: "C"})
	if snapshot, err := service.Collect(context.Background(), definition, query.DefaultLimits()); err != nil || len(snapshot.Items) != 3 {
		t.Fatalf("second query = %#v, %v", snapshot, err)
	}
	if err := pipeline.RunOnce(context.Background(), definition.Config); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(sent, []string{"C"}) {
		t.Fatalf("sent = %v, want C", sent)
	}
}
