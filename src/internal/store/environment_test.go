package store

import (
	"reflect"
	"testing"

	"github.com/acoradd/otter/src/internal/apperr"
)

func TestEnvironmentLifecycle(t *testing.T) {
	db := open(t)
	ws, _ := db.CreateWorkspace(ctx, "API")
	other, _ := db.CreateWorkspace(ctx, "Other")
	dev, err := db.CreateEnvironment(ctx, ws, " dev ")
	if err != nil {
		t.Fatal(err)
	}
	prod, _ := db.CreateEnvironment(ctx, ws, "prod")
	if _, err := db.CreateEnvironment(ctx, other, "elsewhere"); err != nil {
		t.Fatal(err)
	}

	list, err := db.Environments(ctx, ws)
	if err != nil {
		t.Fatal(err)
	}
	want := []Environment{{dev, ws, "dev"}, {prod, ws, "prod"}}
	if !reflect.DeepEqual(list, want) {
		t.Fatalf("environments = %+v, want %+v (in creation order)", list, want)
	}

	if err := db.RenameEnvironment(ctx, prod, "production"); err != nil {
		t.Fatal(err)
	}
	if e, err := db.Environment(ctx, prod); err != nil || e.Name != "production" || e.WorkspaceID != ws {
		t.Fatalf("Environment = %+v, %v", e, err)
	}

	if err := db.DeleteEnvironment(ctx, dev); err != nil {
		t.Fatal(err)
	}
	if list, _ := db.Environments(ctx, ws); len(list) != 1 || list[0].ID != prod {
		t.Fatalf("after delete: %+v", list)
	}

	// Deleting the workspace deletes its environments.
	if err := db.DeleteWorkspace(ctx, ws); err != nil {
		t.Fatal(err)
	}
	_, err = db.Environment(ctx, prod)
	wantCode(t, err, apperr.NotFound)
}

func TestEnvironmentErrors(t *testing.T) {
	db := open(t)
	ws, _ := db.CreateWorkspace(ctx, "API")
	env, _ := db.CreateEnvironment(ctx, ws, "dev")
	create := func(ws int64, name string) error { _, err := db.CreateEnvironment(ctx, ws, name); return err }
	tests := []struct {
		name string
		err  error
		want apperr.Code
	}{
		{"create without a name", create(ws, " "), CodeNameRequired},
		{"create in a missing workspace", create(999, "dev"), apperr.NotFound},
		{"rename to nothing", db.RenameEnvironment(ctx, env, ""), CodeNameRequired},
		{"rename a missing one", db.RenameEnvironment(ctx, 999, "x"), apperr.NotFound},
		{"delete a missing one", db.DeleteEnvironment(ctx, 999), apperr.NotFound},
		{"vars of a missing one", db.SetEnvVars(ctx, 999, nil), apperr.NotFound},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) { wantCode(t, tt.err, tt.want) })
	}
}

func TestSetEnvVars(t *testing.T) {
	db := open(t)
	ws, _ := db.CreateWorkspace(ctx, "API")
	env, _ := db.CreateEnvironment(ctx, ws, "dev")

	tests := []struct {
		name string
		in   []EnvVar
		want []EnvVar
		code apperr.Code
	}{
		{
			name: "kept in order, keys trimmed",
			in: []EnvVar{
				{Key: " baseUrl ", Value: "https://dev.api.test", Enabled: true},
				{Key: "token", Value: "s3cr3t", Secret: true, Enabled: true},
				{Key: "user.id-2", Value: "42"},
			},
			want: []EnvVar{
				{Key: "baseUrl", Value: "https://dev.api.test", Enabled: true},
				{Key: "token", Value: "s3cr3t", Secret: true, Enabled: true},
				{Key: "user.id-2", Value: "42"},
			},
		},
		{
			name: "blank rows dropped",
			in:   []EnvVar{{Key: "  "}, {Key: "a", Value: "1", Enabled: true}, {}},
			want: []EnvVar{{Key: "a", Value: "1", Enabled: true}},
		},
		{
			name: "empty list clears",
			in:   nil,
			want: []EnvVar{},
		},
		{
			name: "a value without a key is refused",
			in:   []EnvVar{{Key: "", Value: "orphan"}},
			code: CodeInvalidVarKey,
		},
		{
			name: "a key with a space is refused",
			in:   []EnvVar{{Key: "base url", Value: "x"}},
			code: CodeInvalidVarKey,
		},
		{
			name: "a key with braces is refused",
			in:   []EnvVar{{Key: "{{x}}", Value: "x"}},
			code: CodeInvalidVarKey,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			before, _ := db.EnvVars(ctx, env)
			err := db.SetEnvVars(ctx, env, tt.in)
			got, _ := db.EnvVars(ctx, env)
			if tt.code != "" {
				wantCode(t, err, tt.code)
				if !reflect.DeepEqual(got, before) {
					t.Errorf("a refused save changed the variables: %+v", got)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("EnvVars = %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestInvalidVarKeyNamesTheKey(t *testing.T) {
	db := open(t)
	ws, _ := db.CreateWorkspace(ctx, "API")
	env, _ := db.CreateEnvironment(ctx, ws, "dev")
	err := db.SetEnvVars(ctx, env, []EnvVar{{Key: "base url"}})
	var e *apperr.Error
	if !asErr(err, &e) || e.Params["key"] != "base url" {
		t.Fatalf("error = %v, want INVALID_VAR_KEY with key", err)
	}
}
