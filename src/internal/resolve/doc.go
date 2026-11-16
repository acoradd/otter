// Package resolve turns loaded layers (runtime variables, request, folder
// chain, environment, automatic headers) into the Preview shown before a
// request is sent and used to send it: interpolation, variable and header
// resolution, warnings.
//
// It is pure: no I/O, no SQLite, no Wails, no clock. Every rule is covered by
// table-driven tests (docs/specs/scope-v1.md, sections 4 to 6).
package resolve
