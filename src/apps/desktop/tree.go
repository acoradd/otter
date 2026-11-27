package main

import (
	"context"

	"github.com/acoradd/otter/src/internal/apperr"
	"github.com/acoradd/otter/src/internal/store"
)

// EventTreeChanged is emitted after a change to the tree, with the folders
// whose children changed: the frontend refetches those it has loaded.
const EventTreeChanged = "tree.changed"

// TreeChange is the payload of tree.changed.
type TreeChange struct {
	WorkspaceID int64 `json:"workspaceId"`
	// FolderIDs are the folders whose children changed; 0 is the root.
	FolderIDs []int64 `json:"folderIds"`
}

// NodeKind tells a folder from a request.
type NodeKind string

const (
	NodeFolder  NodeKind = "folder"
	NodeRequest NodeKind = "request"
)

// TreeNode is a row of the tree.
type TreeNode struct {
	Kind NodeKind `json:"kind"`
	ID   int64    `json:"id"`
	Name string   `json:"name"`
	// Method is the HTTP method of a request, "" for a folder.
	Method string `json:"method"`
	// HasChildren: a folder that can be expanded.
	HasChildren bool `json:"hasChildren"`
}

// FolderChoice is a folder offered by "Move to", with its path from the root.
type FolderChoice struct {
	ID   int64    `json:"id"`
	Path []string `json:"path"`
}

// TreeService manages the folders and requests of a workspace.
type TreeService struct {
	db   *store.DB
	emit func(name string, data any)
}

func NewTreeService(db *store.DB, emit func(string, any)) *TreeService {
	return &TreeService{db: db, emit: emit}
}

// GetChildren returns the direct children of a folder (0: the workspace
// root), folders first. Expanding a folder loads only this.
func (s *TreeService) GetChildren(ctx context.Context, workspaceID, folderID int64) ([]TreeNode, error) {
	nodes, err := s.db.Children(ctx, workspaceID, folderID)
	if err != nil {
		return nil, err
	}
	out := make([]TreeNode, len(nodes))
	for i, n := range nodes {
		out[i] = TreeNode{Kind: NodeKind(n.Kind), ID: n.ID, Name: n.Name, Method: n.Method, HasChildren: n.HasChildren}
	}
	return out, nil
}

// GetFolderChoices lists every folder of a workspace with its path, sorted
// by path.
func (s *TreeService) GetFolderChoices(ctx context.Context, workspaceID int64) ([]FolderChoice, error) {
	paths, err := s.db.FolderPaths(ctx, workspaceID)
	if err != nil {
		return nil, err
	}
	out := make([]FolderChoice, len(paths))
	for i, p := range paths {
		out[i] = FolderChoice{ID: p.ID, Path: p.Path}
	}
	return out, nil
}

// CreateFolder adds a folder at the end of a folder (0: the root) and
// returns its ID.
func (s *TreeService) CreateFolder(ctx context.Context, workspaceID, parentID int64, name string) (int64, error) {
	id, err := s.db.CreateFolder(ctx, workspaceID, parentID, name)
	if err != nil {
		return 0, err
	}
	s.changed(workspaceID, parentID)
	return id, nil
}

// CreateRequest adds a GET request at the end of a folder and returns its ID.
func (s *TreeService) CreateRequest(ctx context.Context, folderID int64, name string) (int64, error) {
	f, err := s.db.Folder(ctx, folderID)
	if err != nil {
		return 0, err
	}
	id, err := s.db.CreateRequest(ctx, folderID, name)
	if err != nil {
		return 0, err
	}
	s.changed(f.WorkspaceID, folderID)
	return id, nil
}

// RenameNode renames a folder or a request.
func (s *TreeService) RenameNode(ctx context.Context, kind NodeKind, id int64, name string) error {
	ws, parent, err := s.locate(ctx, kind, id)
	if err != nil {
		return err
	}
	if kind == NodeFolder {
		err = s.db.RenameFolder(ctx, id, name)
	} else {
		err = s.db.RenameRequest(ctx, id, name)
	}
	if err != nil {
		return err
	}
	s.changed(ws, parent)
	return nil
}

// DuplicateNode copies a folder (with everything in it) or a request, names
// the copy, places it right after the original and returns its ID.
func (s *TreeService) DuplicateNode(ctx context.Context, kind NodeKind, id int64, name string) (int64, error) {
	ws, parent, err := s.locate(ctx, kind, id)
	if err != nil {
		return 0, err
	}
	var copyID int64
	if kind == NodeFolder {
		copyID, err = s.db.DuplicateFolder(ctx, id, name)
	} else {
		copyID, err = s.db.DuplicateRequest(ctx, id, name)
	}
	if err != nil {
		return 0, err
	}
	s.changed(ws, parent)
	return copyID, nil
}

// DeleteNode deletes a request, or a folder with everything in it.
func (s *TreeService) DeleteNode(ctx context.Context, kind NodeKind, id int64) error {
	ws, parent, err := s.locate(ctx, kind, id)
	if err != nil {
		return err
	}
	if kind == NodeFolder {
		if err := s.db.DeleteFolder(ctx, id); err != nil {
			return err
		}
		s.changed(ws, parent, id)
		return nil
	}
	if err := s.db.DeleteRequest(ctx, id); err != nil {
		return err
	}
	s.changed(ws, parent)
	return nil
}

// MoveNode moves a folder or a request into parentID (0: the root, folders
// only) at index among the nodes of its kind there (-1: last). A folder
// cannot go into itself or a descendant (FOLDER_CYCLE).
func (s *TreeService) MoveNode(ctx context.Context, kind NodeKind, id, parentID int64, index int) error {
	ws, from, err := s.locate(ctx, kind, id)
	if err != nil {
		return err
	}
	if kind == NodeFolder {
		err = s.db.MoveFolder(ctx, id, parentID, index)
	} else {
		err = s.db.MoveRequest(ctx, id, parentID, index)
	}
	if err != nil {
		return err
	}
	if from == parentID {
		s.changed(ws, from)
	} else {
		s.changed(ws, from, parentID)
	}
	return nil
}

// locate returns the workspace and the parent folder of a node.
func (s *TreeService) locate(ctx context.Context, kind NodeKind, id int64) (workspaceID, parentID int64, err error) {
	switch kind {
	case NodeFolder:
		f, err := s.db.Folder(ctx, id)
		return f.WorkspaceID, f.ParentID, err
	case NodeRequest:
		r, err := s.db.Request(ctx, id)
		return r.WorkspaceID, r.FolderID, err
	}
	return 0, 0, apperr.New(apperr.InvalidInput)
}

func (s *TreeService) changed(workspaceID int64, folderIDs ...int64) {
	s.emit(EventTreeChanged, TreeChange{WorkspaceID: workspaceID, FolderIDs: folderIDs})
}
