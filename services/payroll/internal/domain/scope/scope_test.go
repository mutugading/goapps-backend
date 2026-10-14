package scope

import (
	"context"
	"errors"
	"testing"
)

func TestNew_Validation(t *testing.T) {
	tests := []struct {
		name     string
		company  int16
		division string
		wantErr  bool
	}{
		{"valid", 1, "SOLO", false},
		{"zero company", 0, "SOLO", true},
		{"empty division", 1, "", true},
		{"comma in division", 1, "SO,LO", true},
		{"colon in division", 1, "SO:LO", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := New(tt.company, tt.division)
			if (err != nil) != tt.wantErr {
				t.Fatalf("New(%d,%q) err=%v, wantErr=%v", tt.company, tt.division, err, tt.wantErr)
			}
			if err != nil && !errors.Is(err, ErrInvalid) {
				t.Fatalf("err = %v, want ErrInvalid", err)
			}
		})
	}
}

func TestJoin_Format(t *testing.T) {
	solo, _ := New(1, "SOLO")      //nolint:errcheck // valid literal
	jkt, _ := New(1, "JAKARTA")    //nolint:errcheck // valid literal
	other, _ := New(2, "SURABAYA") //nolint:errcheck // valid literal
	tests := []struct {
		name   string
		scopes []Scope
		want   string
	}{
		{"none", nil, ""},
		{"one", []Scope{solo}, "1:SOLO"},
		{"many", []Scope{solo, jkt, other}, "1:SOLO,1:JAKARTA,2:SURABAYA"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := Join(tt.scopes); got != tt.want {
				t.Fatalf("Join = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestIdentity_Context(t *testing.T) {
	if _, ok := FromContext(context.Background()); ok {
		t.Fatal("empty context must not carry an identity")
	}
	solo, _ := New(1, "SOLO") //nolint:errcheck // valid literal
	ctx := WithIdentity(context.Background(), Identity{UserID: "u1", Scopes: []Scope{solo}})
	id, ok := FromContext(ctx)
	if !ok || id.UserID != "u1" || Join(id.Scopes) != "1:SOLO" {
		t.Fatalf("FromContext = %+v, %v", id, ok)
	}
}
