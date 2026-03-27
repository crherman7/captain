package resolver

import (
	"testing"
)

func mockEnv(vars map[string]string) EnvFunc {
	return func(key string) (string, bool) {
		v, ok := vars[key]
		return v, ok
	}
}

func TestResolveString_EnvVar(t *testing.T) {
	r := New(mockEnv(map[string]string{"DB_PASSWORD": "secret"}))
	ctx := ResolveContext{ServiceName: "postgres"}

	got, err := r.ResolveString("password=${env:DB_PASSWORD}", ctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "password=secret" {
		t.Errorf("got %q, want %q", got, "password=secret")
	}
}

func TestResolveString_Name(t *testing.T) {
	r := New(mockEnv(nil))
	ctx := ResolveContext{ServiceName: "postgres", Namespace: "fivei"}

	got, err := r.ResolveString("http://{name}:5432", ctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "http://postgres:5432" {
		t.Errorf("got %q, want %q", got, "http://postgres:5432")
	}
}

func TestResolveString_Combined(t *testing.T) {
	r := New(mockEnv(map[string]string{"DB_PASSWORD": "secret"}))
	ctx := ResolveContext{ServiceName: "postgres"}

	got, err := r.ResolveString("postgresql://user:${env:DB_PASSWORD}@{name}:5432/db", ctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := "postgresql://user:secret@postgres:5432/db"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestResolveString_MissingEnv(t *testing.T) {
	r := New(mockEnv(nil))
	ctx := ResolveContext{ServiceName: "test"}

	_, err := r.ResolveString("${env:MISSING}", ctx)
	if err == nil {
		t.Fatal("expected error for missing env var")
	}
}

func TestResolveMap_Nested(t *testing.T) {
	r := New(mockEnv(map[string]string{"PASS": "s3cret"}))
	ctx := ResolveContext{ServiceName: "redis"}

	input := map[string]interface{}{
		"auth": map[string]interface{}{
			"password": "${env:PASS}",
			"port":     6379,
		},
		"host": "{name}",
	}

	got, err := r.ResolveMap(input, ctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	auth := got["auth"].(map[string]interface{})
	if auth["password"] != "s3cret" {
		t.Errorf("password = %v, want s3cret", auth["password"])
	}
	if auth["port"] != 6379 {
		t.Errorf("port = %v, want 6379", auth["port"])
	}
	if got["host"] != "redis" {
		t.Errorf("host = %v, want redis", got["host"])
	}
}

func TestResolveMap_Slice(t *testing.T) {
	r := New(mockEnv(map[string]string{"A": "x", "B": "y"}))
	ctx := ResolveContext{ServiceName: "svc"}

	input := map[string]interface{}{
		"items": []interface{}{"${env:A}", "${env:B}", 42},
	}

	got, err := r.ResolveMap(input, ctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	items := got["items"].([]interface{})
	if items[0] != "x" || items[1] != "y" || items[2] != 42 {
		t.Errorf("items = %v, want [x y 42]", items)
	}
}

func TestResolveStringMap(t *testing.T) {
	r := New(mockEnv(map[string]string{"KEY": "val"}))
	ctx := ResolveContext{ServiceName: "svc"}

	input := map[string]string{
		"SECRET": "${env:KEY}",
		"HOST":   "{name}",
	}

	got, err := r.ResolveStringMap(input, ctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got["SECRET"] != "val" {
		t.Errorf("SECRET = %q, want %q", got["SECRET"], "val")
	}
	if got["HOST"] != "svc" {
		t.Errorf("HOST = %q, want %q", got["HOST"], "svc")
	}
}
