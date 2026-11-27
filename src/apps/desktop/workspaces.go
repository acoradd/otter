package main

import (
	"context"
	"strconv"

	"github.com/acoradd/otter/src/internal/store"
)

// Events of this service. Their payload is the workspace ID.
const (
	// EventWorkspacesChanged: a workspace was created, renamed or deleted.
	EventWorkspacesChanged = "workspaces.changed"
	// EventRuntimeVarsChanged: a runtime variable of the workspace was set
	// (by a script) or deleted.
	EventRuntimeVarsChanged = "runtimeVars.changed"
	// EventEnvironmentChanged: an environment of the workspace was created,
	// renamed, deleted, edited, or the active one changed.
	EventEnvironmentChanged = "environment.changed"
)

// WorkspaceItem is a workspace in the workspace selector.
type WorkspaceItem struct {
	ID   int64  `json:"id"`
	Name string `json:"name"`
}

// EnvironmentItem is an environment in the environment selector.
type EnvironmentItem struct {
	ID   int64  `json:"id"`
	Name string `json:"name"`
}

// WorkspaceView is what the sidebar shows of the open workspace.
type WorkspaceView struct {
	ID   int64  `json:"id"`
	Name string `json:"name"`
	// ActiveEnvironmentID is 0 when no environment is active.
	ActiveEnvironmentID int64             `json:"activeEnvironmentId"`
	Environments        []EnvironmentItem `json:"environments"`
}

// EnvVarRow is a row of the environment editor.
type EnvVarRow struct {
	Key     string `json:"key"`
	Value   string `json:"value"`
	Secret  bool   `json:"secret"`
	Enabled bool   `json:"enabled"`
}

// EnvironmentView is the environment editor.
type EnvironmentView struct {
	ID          int64       `json:"id"`
	WorkspaceID int64       `json:"workspaceId"`
	Name        string      `json:"name"`
	Vars        []EnvVarRow `json:"vars"`
}

// WorkspaceService manages workspaces and their environments.
type WorkspaceService struct {
	db   *store.DB
	emit func(name string, data any)
}

func NewWorkspaceService(db *store.DB, emit func(string, any)) *WorkspaceService {
	return &WorkspaceService{db: db, emit: emit}
}

// ListWorkspaces lists the workspaces by name.
func (s *WorkspaceService) ListWorkspaces(ctx context.Context) ([]WorkspaceItem, error) {
	list, err := s.db.Workspaces(ctx)
	if err != nil {
		return nil, err
	}
	items := make([]WorkspaceItem, len(list))
	for i, w := range list {
		items[i] = WorkspaceItem{ID: w.ID, Name: w.Name}
	}
	return items, nil
}

// GetWorkspaceView returns a workspace with its environments.
func (s *WorkspaceService) GetWorkspaceView(ctx context.Context, id int64) (WorkspaceView, error) {
	w, err := s.db.Workspace(ctx, id)
	if err != nil {
		return WorkspaceView{}, err
	}
	envs, err := s.db.Environments(ctx, id)
	if err != nil {
		return WorkspaceView{}, err
	}
	view := WorkspaceView{ID: w.ID, Name: w.Name, ActiveEnvironmentID: w.ActiveEnvironmentID, Environments: make([]EnvironmentItem, len(envs))}
	for i, e := range envs {
		view.Environments[i] = EnvironmentItem{ID: e.ID, Name: e.Name}
	}
	return view, nil
}

// CreateWorkspace creates a workspace, opens it, and returns its ID.
func (s *WorkspaceService) CreateWorkspace(ctx context.Context, name string) (int64, error) {
	id, err := s.db.CreateWorkspace(ctx, name)
	if err != nil {
		return 0, err
	}
	s.emit(EventWorkspacesChanged, id)
	return id, s.OpenWorkspace(ctx, id)
}

// RenameWorkspace renames a workspace.
func (s *WorkspaceService) RenameWorkspace(ctx context.Context, id int64, name string) error {
	if err := s.db.RenameWorkspace(ctx, id, name); err != nil {
		return err
	}
	s.emit(EventWorkspacesChanged, id)
	return nil
}

// DeleteWorkspace deletes a workspace with everything in it.
func (s *WorkspaceService) DeleteWorkspace(ctx context.Context, id int64) error {
	if err := s.db.DeleteWorkspace(ctx, id); err != nil {
		return err
	}
	s.emit(EventWorkspacesChanged, id)
	return nil
}

// OpenWorkspace makes a workspace the one shown, now and at the next start.
func (s *WorkspaceService) OpenWorkspace(ctx context.Context, id int64) error {
	if _, err := s.db.Workspace(ctx, id); err != nil {
		return err
	}
	return setAndBroadcast(ctx, s.db, s.emit, settingLastWorkspace, strconv.FormatInt(id, 10))
}

// SetActiveEnvironment makes an environment of the workspace the active one,
// or none with envID 0.
func (s *WorkspaceService) SetActiveEnvironment(ctx context.Context, workspaceID, envID int64) error {
	if err := s.db.SetActiveEnvironment(ctx, workspaceID, envID); err != nil {
		return err
	}
	s.emit(EventEnvironmentChanged, workspaceID)
	return nil
}

// CreateEnvironment adds an empty environment to a workspace and returns its ID.
func (s *WorkspaceService) CreateEnvironment(ctx context.Context, workspaceID int64, name string) (int64, error) {
	id, err := s.db.CreateEnvironment(ctx, workspaceID, name)
	if err != nil {
		return 0, err
	}
	s.emit(EventEnvironmentChanged, workspaceID)
	return id, nil
}

// RenameEnvironment renames an environment.
func (s *WorkspaceService) RenameEnvironment(ctx context.Context, id int64, name string) error {
	return s.changeEnvironment(ctx, id, func() error { return s.db.RenameEnvironment(ctx, id, name) })
}

// DeleteEnvironment deletes an environment and its variables.
func (s *WorkspaceService) DeleteEnvironment(ctx context.Context, id int64) error {
	return s.changeEnvironment(ctx, id, func() error { return s.db.DeleteEnvironment(ctx, id) })
}

// GetEnvironmentView returns an environment with its variables.
func (s *WorkspaceService) GetEnvironmentView(ctx context.Context, id int64) (EnvironmentView, error) {
	e, err := s.db.Environment(ctx, id)
	if err != nil {
		return EnvironmentView{}, err
	}
	vars, err := s.db.EnvVars(ctx, id)
	if err != nil {
		return EnvironmentView{}, err
	}
	view := EnvironmentView{ID: e.ID, WorkspaceID: e.WorkspaceID, Name: e.Name, Vars: make([]EnvVarRow, len(vars))}
	for i, v := range vars {
		view.Vars[i] = EnvVarRow(v)
	}
	return view, nil
}

// SaveEnvironmentVars replaces the variables of an environment.
func (s *WorkspaceService) SaveEnvironmentVars(ctx context.Context, id int64, vars []EnvVarRow) error {
	list := make([]store.EnvVar, len(vars))
	for i, v := range vars {
		list[i] = store.EnvVar(v)
	}
	return s.changeEnvironment(ctx, id, func() error { return s.db.SetEnvVars(ctx, id, list) })
}

// changeEnvironment runs a change to an environment, then tells its workspace.
func (s *WorkspaceService) changeEnvironment(ctx context.Context, id int64, change func() error) error {
	e, err := s.db.Environment(ctx, id)
	if err != nil {
		return err
	}
	if err := change(); err != nil {
		return err
	}
	s.emit(EventEnvironmentChanged, e.WorkspaceID)
	return nil
}

// DeleteRuntimeVar deletes a runtime variable of the workspace, for its
// active environment.
func (s *WorkspaceService) DeleteRuntimeVar(ctx context.Context, workspaceID int64, key string) error {
	w, err := s.db.Workspace(ctx, workspaceID)
	if err != nil {
		return err
	}
	if err := s.db.DeleteRuntimeVar(ctx, workspaceID, w.ActiveEnvironmentID, key); err != nil {
		return err
	}
	s.emit(EventRuntimeVarsChanged, workspaceID)
	return nil
}
