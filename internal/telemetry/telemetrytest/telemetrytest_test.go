package telemetrytest_test

import (
	"context"
	"strings"
	"testing"

	"github.com/henvic/pgxtutorial/internal/telemetry/telemetrytest"
	"go.opentelemetry.io/otel/propagation"
)

func TestProvider(t *testing.T) {
	tel, mem := telemetrytest.Provider()
	tel.Logger().Info("an example")

	want := `"msg":"an example"`
	if got := mem.Log(); !strings.Contains(got, want) {
		t.Errorf("mem.Log() = %v doesn't contain %v", got, want)
	}

	_, span := tel.Tracer().Start(context.Background(), "span")
	span.AddEvent("an_event")
	span.End()
	spans := mem.Trace()
	if len(spans) != 1 {
		t.Fatalf("len(spans) = %v, want 1", len(spans))
	}
	if got, want := spans[0].Name(), "span"; got != want {
		t.Errorf("spans[0].Name() = %v, want %v", got, want)
	}
	i, err := tel.Meter().Int64Counter("onecounter")
	if err != nil {
		t.Errorf("tel.Meter().Int64Counter() = %v, want nil", err)
	}
	i.Add(context.Background(), 63)
	if !strings.Contains(mem.Meter(), "onecounter") {
		t.Errorf("mem.Meter() = %v, want to contain %v", mem.Meter(), "onecounter")
	}
	mem.Reset()
	i.Add(context.Background(), 1337)
	if mem.Log() != "" {
		t.Errorf("mem.Log() = %v, want empty", mem.Log())
	}
	if len(mem.Trace()) != 0 {
		t.Errorf("len(mem.Trace()) = %v, want 0", len(mem.Trace()))
	}
	if !strings.Contains(mem.Meter(), `"Value":1400`) {
		t.Errorf("mem.Meter() = %v, want to contain %v", mem.Meter(), `"Value":1400`)
	}
	tel.Propagator().Inject(context.Background(), propagation.HeaderCarrier{
		"abc": []string{"def"},
	})
	if len(tel.Propagator().Fields()) != 3 {
		t.Errorf("len(tel.Propagator().Fields()) = %v, want 4", len(tel.Propagator().Fields()))
	}
}
