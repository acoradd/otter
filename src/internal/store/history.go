package store

import (
	"context"
	"database/sql"
	"errors"

	"github.com/acoradd/otter/src/internal/apperr"
)

// HistoryKeep is how many sends are kept per request; older ones go.
const HistoryKeep = 100

// HistoryEntry is a send as recorded. Headers are JSON; secrets are masked
// and files described by name and size before they reach it.
type HistoryEntry struct {
	ID          int64
	RequestID   int64
	WorkspaceID int64
	SentAt      int64 // epoch ms
	Method      string
	URL         string
	Status      int
	DurationMS  int64
	SizeBytes   int64
	ReqHeaders  string
	ReqBody     string
	ResHeaders  string
	ResBody     string
	// Error is the error code of a failed send, "" when a response came.
	Error string
}

// AddHistory records a send, then drops what is past the HistoryKeep newest
// sends of that request.
func (d *DB) AddHistory(ctx context.Context, e HistoryEntry) (int64, error) {
	var id int64
	err := d.tx(ctx, func(tx *sql.Tx) error {
		res, err := tx.ExecContext(ctx,
			`INSERT INTO history (request_id, workspace_id, sent_at, method, url, status, duration_ms, size_bytes,
			   req_headers, req_body, res_headers, res_body, error)
			 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			e.RequestID, e.WorkspaceID, e.SentAt, e.Method, e.URL, e.Status, e.DurationMS, e.SizeBytes,
			e.ReqHeaders, e.ReqBody, e.ResHeaders, e.ResBody, e.Error)
		if err != nil {
			return err
		}
		if id, err = res.LastInsertId(); err != nil {
			return err
		}
		_, err = tx.ExecContext(ctx,
			`DELETE FROM history WHERE request_id = ? AND id NOT IN (
			   SELECT id FROM history WHERE request_id = ? ORDER BY sent_at DESC, id DESC LIMIT ?)`,
			e.RequestID, e.RequestID, HistoryKeep)
		return err
	})
	return id, err
}

// History lists the sends of a request, newest first, without their bodies.
func (d *DB) History(ctx context.Context, requestID int64) ([]HistoryEntry, error) {
	rows, err := d.db.QueryContext(ctx,
		`SELECT id, request_id, workspace_id, sent_at, method, url, COALESCE(status, 0), COALESCE(duration_ms, 0),
		   COALESCE(size_bytes, 0), COALESCE(error, '')
		 FROM history WHERE request_id = ? ORDER BY sent_at DESC, id DESC`, requestID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	list := []HistoryEntry{}
	for rows.Next() {
		var e HistoryEntry
		if err := rows.Scan(&e.ID, &e.RequestID, &e.WorkspaceID, &e.SentAt, &e.Method, &e.URL, &e.Status,
			&e.DurationMS, &e.SizeBytes, &e.Error); err != nil {
			return nil, err
		}
		list = append(list, e)
	}
	return list, rows.Err()
}

// HistoryEntryByID returns a recorded send with everything in it.
func (d *DB) HistoryEntryByID(ctx context.Context, id int64) (HistoryEntry, error) {
	e := HistoryEntry{ID: id}
	err := d.db.QueryRowContext(ctx,
		`SELECT COALESCE(request_id, 0), COALESCE(workspace_id, 0), sent_at, COALESCE(method, ''), COALESCE(url, ''),
		   COALESCE(status, 0), COALESCE(duration_ms, 0), COALESCE(size_bytes, 0), COALESCE(req_headers, ''),
		   COALESCE(req_body, ''), COALESCE(res_headers, ''), COALESCE(res_body, ''), COALESCE(error, '')
		 FROM history WHERE id = ?`, id).
		Scan(&e.RequestID, &e.WorkspaceID, &e.SentAt, &e.Method, &e.URL, &e.Status, &e.DurationMS, &e.SizeBytes,
			&e.ReqHeaders, &e.ReqBody, &e.ResHeaders, &e.ResBody, &e.Error)
	if errors.Is(err, sql.ErrNoRows) {
		return HistoryEntry{}, apperr.New(apperr.NotFound)
	}
	return e, err
}
