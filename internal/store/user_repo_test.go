package store

import (
	"context"
	"errors"
	"testing"

	"github.com/logic-roastery/project-talos/internal/domain"
)

func TestCreateAndGetUser(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	user := &domain.User{
		Username:     "alice",
		PasswordHash: "hash123",
	}

	if err := s.CreateUser(ctx, user); err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	if user.ID == 0 {
		t.Fatal("expected user ID to be set after CreateUser")
	}

	got, err := s.GetUserByID(ctx, user.ID)
	if err != nil {
		t.Fatalf("GetUserByID: %v", err)
	}
	if got.Username != user.Username {
		t.Errorf("username = %q, want %q", got.Username, user.Username)
	}
	if got.PasswordHash != user.PasswordHash {
		t.Errorf("password_hash = %q, want %q", got.PasswordHash, user.PasswordHash)
	}
}

func TestGetUserByUsername(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	user := &domain.User{
		Username:     "bob",
		PasswordHash: "hash456",
	}

	if err := s.CreateUser(ctx, user); err != nil {
		t.Fatalf("CreateUser: %v", err)
	}

	got, err := s.GetUserByUsername(ctx, "bob")
	if err != nil {
		t.Fatalf("GetUserByUsername: %v", err)
	}
	if got.ID != user.ID {
		t.Errorf("ID = %d, want %d", got.ID, user.ID)
	}
	if got.Username != user.Username {
		t.Errorf("username = %q, want %q", got.Username, user.Username)
	}
}

func TestGetUserNotFound(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	_, err := s.GetUserByID(ctx, 9999)
	if !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("expected domain.ErrNotFound, got: %v", err)
	}
}

func TestGetUserByUsernameNotFound(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	_, err := s.GetUserByUsername(ctx, "nonexistent")
	if !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("expected domain.ErrNotFound, got: %v", err)
	}
}

func TestHasUsers(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	has, err := s.HasUsers(ctx)
	if err != nil {
		t.Fatalf("HasUsers (empty): %v", err)
	}
	if has {
		t.Error("expected HasUsers to return false on empty database")
	}

	user := &domain.User{
		Username:     "charlie",
		PasswordHash: "hash789",
	}
	if err := s.CreateUser(ctx, user); err != nil {
		t.Fatalf("CreateUser: %v", err)
	}

	has, err = s.HasUsers(ctx)
	if err != nil {
		t.Fatalf("HasUsers (after insert): %v", err)
	}
	if !has {
		t.Error("expected HasUsers to return true after creating a user")
	}
}

func TestCreateDuplicateUser(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	user1 := &domain.User{
		Username:     "duplicate",
		PasswordHash: "hash1",
	}
	if err := s.CreateUser(ctx, user1); err != nil {
		t.Fatalf("CreateUser (first): %v", err)
	}

	user2 := &domain.User{
		Username:     "duplicate",
		PasswordHash: "hash2",
	}
	if err := s.CreateUser(ctx, user2); err == nil {
		t.Error("expected error when creating user with duplicate username")
	}
}
