package features_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/cucumber/godog"

	"github.com/andob/jirlab/internal/service"
	"github.com/andob/jirlab/internal/tui"
)

// ---------------------------------------------------------------------------
// Context keys
// ---------------------------------------------------------------------------

type (
	worklogsKey    struct{}
	todayKey       struct{}
	startHourKey   struct{}
	logResultKey   struct{}
	logErrKey      struct{}
	numInputKey    struct{}
	logFromKey     struct{}
	logToKey       struct{}
)

// ---------------------------------------------------------------------------
// Step definitions — Tracker log
// ---------------------------------------------------------------------------

func todayIs(ctx context.Context, dateStr string) (context.Context, error) {
	t, err := time.Parse("2006-01-02", dateStr)
	if err != nil {
		return ctx, fmt.Errorf("parse date %q: %w", dateStr, err)
	}
	return context.WithValue(ctx, todayKey{}, t), nil
}

func thereAreNoWorklogsForToday(ctx context.Context) (context.Context, error) {
	return context.WithValue(ctx, worklogsKey{}, []service.TimeLog{}), nil
}

func thereIsAWorklogFromTo(ctx context.Context, fromStr, toStr string) (context.Context, error) {
	today, _ := ctx.Value(todayKey{}).(time.Time)
	if today.IsZero() {
		today = time.Now().Truncate(24 * time.Hour)
	}
	from, err := parseTimeOnDate(today, fromStr)
	if err != nil {
		return ctx, fmt.Errorf("parse from time %q: %w", fromStr, err)
	}
	to, err := parseTimeOnDate(today, toStr)
	if err != nil {
		return ctx, fmt.Errorf("parse to time %q: %w", toStr, err)
	}
	existing, _ := ctx.Value(worklogsKey{}).([]service.TimeLog)
	newLog := service.TimeLog{
		From:  from,
		To:    to,
		Hours: to.Sub(from).Hours(),
	}
	return context.WithValue(ctx, worklogsKey{}, append(existing, newLog)), nil
}

func iCalculateTheLogStartHour(ctx context.Context) (context.Context, error) {
	today, _ := ctx.Value(todayKey{}).(time.Time)
	if today.IsZero() {
		today = time.Now().Truncate(24 * time.Hour)
	}
	logs, _ := ctx.Value(worklogsKey{}).([]service.TimeLog)
	startH := tui.LatestLogEndHour(logs, today)
	return context.WithValue(ctx, startHourKey{}, startH), nil
}

func theStartHourShouldBe(ctx context.Context, expected int) error {
	got, _ := ctx.Value(startHourKey{}).(int)
	if got != expected {
		return fmt.Errorf("expected start hour %d, got %d", expected, got)
	}
	return nil
}

// ---------------------------------------------------------------------------
// Single-char numeric input steps
// ---------------------------------------------------------------------------

func theLogInputHasDefaultValue(ctx context.Context, def string) (context.Context, error) {
	return context.WithValue(ctx, numInputKey{}, def), nil
}

func theUserTypes(ctx context.Context, char string) (context.Context, error) {
	current, _ := ctx.Value(numInputKey{}).(string)
	updated := tui.NumericInputHandleKey(current, char)
	return context.WithValue(ctx, numInputKey{}, updated), nil
}

func theInputValueShouldBe(ctx context.Context, expected string) error {
	got, _ := ctx.Value(numInputKey{}).(string)
	if got != expected {
		return fmt.Errorf("expected input %q, got %q", expected, got)
	}
	return nil
}

// ---------------------------------------------------------------------------
// Log creation steps
// ---------------------------------------------------------------------------

func iRequestToLogHoursStartingFromTheLatestEnd(ctx context.Context, hours int) (context.Context, error) {
	today, _ := ctx.Value(todayKey{}).(time.Time)
	if today.IsZero() {
		today = time.Now().Truncate(24 * time.Hour)
	}
	logs, _ := ctx.Value(worklogsKey{}).([]service.TimeLog)
	startH := tui.LatestLogEndHour(logs, today)
	endH := startH + hours

	from := time.Date(today.Year(), today.Month(), today.Day(), startH, 0, 0, 0, today.Location())
	to := time.Date(today.Year(), today.Month(), today.Day(), endH, 0, 0, 0, today.Location())

	if endH > 16 {
		err := fmt.Errorf("log would exceed 16:00 (end=%d:00)", endH)
		ctx = context.WithValue(ctx, logErrKey{}, err)
		return ctx, nil
	}
	ctx = context.WithValue(ctx, logFromKey{}, from)
	ctx = context.WithValue(ctx, logToKey{}, to)
	return ctx, nil
}

func theLogShouldBeCreatedFromTo(ctx context.Context, fromStr, toStr string) error {
	today, _ := ctx.Value(todayKey{}).(time.Time)
	if today.IsZero() {
		today = time.Now().Truncate(24 * time.Hour)
	}
	expectedFrom, err := parseTimeOnDate(today, fromStr)
	if err != nil {
		return err
	}
	expectedTo, err := parseTimeOnDate(today, toStr)
	if err != nil {
		return err
	}
	gotFrom, _ := ctx.Value(logFromKey{}).(time.Time)
	gotTo, _ := ctx.Value(logToKey{}).(time.Time)

	if !gotFrom.Equal(expectedFrom) {
		return fmt.Errorf("expected from %v, got %v", expectedFrom, gotFrom)
	}
	if !gotTo.Equal(expectedTo) {
		return fmt.Errorf("expected to %v, got %v", expectedTo, gotTo)
	}
	return nil
}

func anErrorShouldIndicateTheDayIsAlreadyFullyLogged(ctx context.Context) error {
	err, _ := ctx.Value(logErrKey{}).(error)
	if err == nil {
		return fmt.Errorf("expected an error for exceeding 16:00, but got none")
	}
	return nil
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func parseTimeOnDate(date time.Time, timeStr string) (time.Time, error) {
	var h, m int
	if _, err := fmt.Sscanf(timeStr, "%d:%d", &h, &m); err != nil {
		return time.Time{}, fmt.Errorf("invalid time format %q: %w", timeStr, err)
	}
	return time.Date(date.Year(), date.Month(), date.Day(), h, m, 0, 0, date.Location()), nil
}

// ---------------------------------------------------------------------------
// Suite wiring
// ---------------------------------------------------------------------------

func InitializeTrackerLogScenario(sc *godog.ScenarioContext) {
	sc.Step(`^today is "([^"]*)"$`, todayIs)
	sc.Step(`^there are no worklogs for today$`, thereAreNoWorklogsForToday)
	sc.Step(`^there is a worklog from "([^"]*)" to "([^"]*)" for today$`, thereIsAWorklogFromTo)
	sc.Step(`^I calculate the log start hour$`, iCalculateTheLogStartHour)
	sc.Step(`^the start hour should be (\d+)$`, theStartHourShouldBe)
	sc.Step(`^the log input has default value "([^"]*)"$`, theLogInputHasDefaultValue)
	sc.Step(`^the user types "([^"]*)"$`, theUserTypes)
	sc.Step(`^the input value should be "([^"]*)"$`, theInputValueShouldBe)
	sc.Step(`^I request to log (\d+) hours starting from the latest end$`, iRequestToLogHoursStartingFromTheLatestEnd)
	sc.Step(`^the log should be created from "([^"]*)" to "([^"]*)"$`, theLogShouldBeCreatedFromTo)
	sc.Step(`^an error should indicate the day is already fully logged$`, anErrorShouldIndicateTheDayIsAlreadyFullyLogged)
}

func TestTrackerLog(t *testing.T) {
	suite := godog.TestSuite{
		ScenarioInitializer: InitializeTrackerLogScenario,
		Options: &godog.Options{
			Format:   "pretty",
			Paths:    []string{"tracker_log.feature"},
			TestingT: t,
		},
	}
	if suite.Run() != 0 {
		t.Fatal("non-zero status: failed acceptance tests")
	}
}
