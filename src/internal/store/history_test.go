package store

import (
	"testing"

	"github.com/acoradd/otter/src/internal/apperr"
)

func TestHistory(t *testing.T) {
	f := newTree(t)
	add := func(req int64, at int64, status int) int64 {
		t.Helper()
		id, err := f.db.AddHistory(ctx, HistoryEntry{
			RequestID: req, WorkspaceID: f.ws, SentAt: at, Method: "GET", URL: "https://h/x", Status: status,
			DurationMS: 12, SizeBytes: 34, ReqHeaders: `[{"key":"Authorization","value":"••••••"}]`, ResBody: "ok",
		})
		if err != nil {
			t.Fatal(err)
		}
		return id
	}
	first := add(f.list, 1000, 200)
	add(f.list, 3000, 500)
	add(f.list, 2000, 404)
	add(f.create, 4000, 201)

	list, err := f.db.History(ctx, f.list)
	if err != nil {
		t.Fatal(err)
	}
	var statuses []int
	for _, e := range list {
		statuses = append(statuses, e.Status)
		if e.ResBody != "" || e.ReqHeaders != "" {
			t.Errorf("the list carries no bodies: %+v", e)
		}
	}
	if len(statuses) != 3 || statuses[0] != 500 || statuses[1] != 404 || statuses[2] != 200 {
		t.Fatalf("statuses = %v: newest first, this request only", statuses)
	}

	e, err := f.db.HistoryEntryByID(ctx, first)
	if err != nil || e.ResBody != "ok" || e.ReqHeaders == "" || e.SentAt != 1000 {
		t.Fatalf("entry = %+v, %v", e, err)
	}
	_, err = f.db.HistoryEntryByID(ctx, 999)
	wantCode(t, err, apperr.NotFound)

	// Deleting the request keeps its history (request_id becomes NULL).
	if err := f.db.DeleteRequest(ctx, f.create); err != nil {
		t.Fatal(err)
	}
	var orphans int
	f.db.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM history WHERE request_id IS NULL`).Scan(&orphans)
	if orphans != 1 {
		t.Errorf("orphans = %d", orphans)
	}
}

func TestHistoryKeepsTheNewest(t *testing.T) {
	f := newTree(t)
	for i := 0; i < HistoryKeep+15; i++ {
		if _, err := f.db.AddHistory(ctx, HistoryEntry{RequestID: f.list, WorkspaceID: f.ws, SentAt: int64(i)}); err != nil {
			t.Fatal(err)
		}
	}
	list, _ := f.db.History(ctx, f.list)
	if len(list) != HistoryKeep || list[len(list)-1].SentAt != 15 {
		t.Fatalf("%d kept, oldest at %d: want %d, oldest 15", len(list), list[len(list)-1].SentAt, HistoryKeep)
	}
}
