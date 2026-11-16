// Package store is the SQLite access of Otter: opening otter.db (WAL,
// foreign keys, application_id OTTR), append-only migrations versioned by
// user_version, queries and full-text search (FTS5).
//
// Every change comes with tests.
package store
