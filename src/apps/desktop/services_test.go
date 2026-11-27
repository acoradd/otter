package main

import (
	"context"
	"errors"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/acoradd/otter/src/internal/apperr"
	"github.com/acoradd/otter/src/internal/store"
)

var ctx = context.Background()

// emitted records the events a service emits.
type emitted struct{ events []event }

type event struct {
	Name string
	Data any
}

func (e *emitted) emit(name string, data any) { e.events = append(e.events, event{name, data}) }

func (e *emitted) names() []string {
	names := make([]string, len(e.events))
	for i, ev := range e.events {
		names[i] = ev.Name
	}
	return names
}

func openDB(t *testing.T) *store.DB {
	t.Helper()
	db, err := store.Open(ctx, filepath.Join(t.TempDir(), store.FileName))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

func wantCode(t *testing.T, err error, code apperr.Code) {
	t.Helper()
	var e *apperr.Error
	if !errors.As(err, &e) || e.Code != code {
		t.Fatalf("error = %v, want %s", err, code)
	}
}

// must returns v, and fails the test (panics) on err.
func must[T any](v T, err error) T {
	if err != nil {
		panic(err)
	}
	return v
}

func TestSettingsDefaultsAndChanges(t *testing.T) {
	db := openDB(t)
	rec := &emitted{}
	s := NewSettingsService(db, rec.emit)

	if got := must(s.GetSettings(ctx)); got != (Settings{Theme: "system"}) {
		t.Fatalf("defaults = %+v", got)
	}
	if err := s.SetLanguage(ctx, "fr"); err != nil {
		t.Fatal(err)
	}
	if err := s.SetTheme(ctx, "dark"); err != nil {
		t.Fatal(err)
	}
	want := Settings{Language: "fr", Theme: "dark"}
	if got := must(s.GetSettings(ctx)); got != want {
		t.Fatalf("settings = %+v, want %+v", got, want)
	}
	last := rec.events[len(rec.events)-1]
	if last.Name != EventSettingsChanged || last.Data != want {
		t.Fatalf("last event = %+v, want settings.changed with %+v", last, want)
	}

	wantCode(t, s.SetLanguage(ctx, "de"), apperr.InvalidInput)
	wantCode(t, s.SetTheme(ctx, "blue"), apperr.InvalidInput)
	if len(rec.events) != 2 {
		t.Fatalf("refused changes emitted events: %v", rec.names())
	}
	// Back to following the system language.
	if err := s.SetLanguage(ctx, ""); err != nil {
		t.Fatal(err)
	}
}

func TestWorkspaceCommandsEmitAndOpen(t *testing.T) {
	db := openDB(t)
	rec := &emitted{}
	s := NewWorkspaceService(db, rec.emit)
	settings := NewSettingsService(db, rec.emit)

	id := must(s.CreateWorkspace(ctx, "Payments API"))
	if got := rec.names(); !reflect.DeepEqual(got, []string{EventWorkspacesChanged, EventSettingsChanged}) {
		t.Fatalf("events = %v", got)
	}
	if got := must(settings.GetSettings(ctx)).LastWorkspaceID; got != id {
		t.Fatalf("a new workspace should be opened: last = %d, want %d", got, id)
	}

	other := must(s.CreateWorkspace(ctx, "Auth"))
	if err := s.OpenWorkspace(ctx, id); err != nil {
		t.Fatal(err)
	}
	wantCode(t, s.OpenWorkspace(ctx, 999), apperr.NotFound)

	if err := s.RenameWorkspace(ctx, other, "Identity"); err != nil {
		t.Fatal(err)
	}
	list := must(s.ListWorkspaces(ctx))
	want := []WorkspaceItem{{other, "Identity"}, {id, "Payments API"}}
	if !reflect.DeepEqual(list, want) {
		t.Fatalf("list = %+v, want %+v", list, want)
	}

	rec.events = nil
	if err := s.DeleteWorkspace(ctx, other); err != nil {
		t.Fatal(err)
	}
	if len(rec.events) != 1 || rec.events[0] != (event{EventWorkspacesChanged, other}) {
		t.Fatalf("events = %+v", rec.events)
	}
	wantCode(t, s.RenameWorkspace(ctx, other, "x"), apperr.NotFound)
	_, err := s.CreateWorkspace(ctx, " ")
	wantCode(t, err, store.CodeNameRequired)
}

func TestEnvironmentCommands(t *testing.T) {
	db := openDB(t)
	rec := &emitted{}
	s := NewWorkspaceService(db, rec.emit)
	ws := must(s.CreateWorkspace(ctx, "API"))
	rec.events = nil

	dev := must(s.CreateEnvironment(ctx, ws, "dev"))
	prod := must(s.CreateEnvironment(ctx, ws, "prod"))
	if err := s.SetActiveEnvironment(ctx, ws, prod); err != nil {
		t.Fatal(err)
	}
	if err := s.RenameEnvironment(ctx, prod, "production"); err != nil {
		t.Fatal(err)
	}
	vars := []EnvVarRow{
		{Key: "baseUrl", Value: "https://dev.api.test", Enabled: true},
		{Key: "token", Value: "s3cr3t", Secret: true, Enabled: true},
	}
	if err := s.SaveEnvironmentVars(ctx, dev, vars); err != nil {
		t.Fatal(err)
	}
	for _, ev := range rec.events {
		if ev != (event{EventEnvironmentChanged, ws}) {
			t.Fatalf("event %+v: want environment.changed for workspace %d", ev, ws)
		}
	}
	if len(rec.events) != 5 { // two creates, activate, rename, save
		t.Fatalf("events = %v", rec.names())
	}

	view := must(s.GetWorkspaceView(ctx, ws))
	wantView := WorkspaceView{ID: ws, Name: "API", ActiveEnvironmentID: prod, Environments: []EnvironmentItem{{dev, "dev"}, {prod, "production"}}}
	if !reflect.DeepEqual(view, wantView) {
		t.Fatalf("workspace view = %+v, want %+v", view, wantView)
	}
	env := must(s.GetEnvironmentView(ctx, dev))
	wantEnv := EnvironmentView{ID: dev, WorkspaceID: ws, Name: "dev", Vars: vars}
	if !reflect.DeepEqual(env, wantEnv) {
		t.Fatalf("environment view = %+v, want %+v", env, wantEnv)
	}

	rec.events = nil
	wantCode(t, s.SaveEnvironmentVars(ctx, dev, []EnvVarRow{{Key: "bad key"}}), store.CodeInvalidVarKey)
	wantCode(t, s.DeleteEnvironment(ctx, 999), apperr.NotFound)
	_, err := s.GetEnvironmentView(ctx, 999)
	wantCode(t, err, apperr.NotFound)
	if len(rec.events) != 0 {
		t.Fatalf("failed commands emitted: %v", rec.names())
	}

	if err := s.DeleteEnvironment(ctx, prod); err != nil {
		t.Fatal(err)
	}
	if view := must(s.GetWorkspaceView(ctx, ws)); view.ActiveEnvironmentID != 0 || len(view.Environments) != 1 {
		t.Fatalf("after deleting the active environment: %+v", view)
	}
}
