package auth

import (
	"context"
	"testing"
	"time"

	"github.com/logic-roastery/project-talos/internal/domain"
)

type mockUserStore struct {
	users      map[int64]*domain.User
	byUsername map[string]*domain.User
	nextID     int64
}

func newMockUserStore() *mockUserStore {
	return &mockUserStore{
		users:      make(map[int64]*domain.User),
		byUsername: make(map[string]*domain.User),
		nextID:     1,
	}
}

func (m *mockUserStore) CreateUser(_ context.Context, user *domain.User) error {
	user.ID = m.nextID
	m.nextID++
	m.users[user.ID] = user
	m.byUsername[user.Username] = user
	return nil
}

func (m *mockUserStore) GetUserByID(_ context.Context, id int64) (*domain.User, error) {
	user, ok := m.users[id]
	if !ok {
		return nil, domain.ErrNotFound
	}
	return user, nil
}

func (m *mockUserStore) GetUserByUsername(_ context.Context, username string) (*domain.User, error) {
	user, ok := m.byUsername[username]
	if !ok {
		return nil, domain.ErrNotFound
	}
	return user, nil
}

func (m *mockUserStore) HasUsers(_ context.Context) (bool, error) {
	return len(m.users) > 0, nil
}

func TestHashPassword(t *testing.T) {
	hash, err := HashPassword("password123")
	if err != nil {
		t.Fatalf("HashPassword returned error: %v", err)
	}
	if hash == "" {
		t.Fatal("HashPassword returned empty hash")
	}
	if hash == "password123" {
		t.Fatal("HashPassword returned plaintext password")
	}
}

func TestCheckPasswordCorrect(t *testing.T) {
	hash, err := HashPassword("password123")
	if err != nil {
		t.Fatalf("HashPassword returned error: %v", err)
	}
	if !CheckPassword("password123", hash) {
		t.Fatal("CheckPassword returned false for correct password")
	}
}

func TestCheckPasswordWrong(t *testing.T) {
	hash, err := HashPassword("password123")
	if err != nil {
		t.Fatalf("HashPassword returned error: %v", err)
	}
	if CheckPassword("wrong", hash) {
		t.Fatal("CheckPassword returned true for wrong password")
	}
}

func TestCreateAndValidateSession(t *testing.T) {
	store := newMockUserStore()
	svc := NewService(store, "test-secret", time.Hour)

	user, err := svc.CreateUser(context.Background(), "testuser", "password123")
	if err != nil {
		t.Fatalf("CreateUser returned error: %v", err)
	}

	token, err := svc.CreateSession(user)
	if err != nil {
		t.Fatalf("CreateSession returned error: %v", err)
	}

	validated, err := svc.ValidateSession(context.Background(), token)
	if err != nil {
		t.Fatalf("ValidateSession returned error: %v", err)
	}
	if validated.ID != user.ID {
		t.Fatalf("ValidateSession returned user ID %d, want %d", validated.ID, user.ID)
	}
}

func TestValidateSessionExpired(t *testing.T) {
	store := newMockUserStore()
	svc := NewService(store, "test-secret", -time.Second)

	user, err := svc.CreateUser(context.Background(), "testuser", "password123")
	if err != nil {
		t.Fatalf("CreateUser returned error: %v", err)
	}

	token, err := svc.CreateSession(user)
	if err != nil {
		t.Fatalf("CreateSession returned error: %v", err)
	}

	_, err = svc.ValidateSession(context.Background(), token)
	if err != domain.ErrUnauthorized {
		t.Fatalf("ValidateSession returned %v, want ErrUnauthorized", err)
	}
}

func TestValidateSessionTampered(t *testing.T) {
	store := newMockUserStore()
	svc := NewService(store, "test-secret", time.Hour)

	user, err := svc.CreateUser(context.Background(), "testuser", "password123")
	if err != nil {
		t.Fatalf("CreateUser returned error: %v", err)
	}

	token, err := svc.CreateSession(user)
	if err != nil {
		t.Fatalf("CreateSession returned error: %v", err)
	}

	tampered := token + "tampered"
	_, err = svc.ValidateSession(context.Background(), tampered)
	if err != domain.ErrUnauthorized {
		t.Fatalf("ValidateSession returned %v, want ErrUnauthorized", err)
	}
}

func TestValidateSessionBadFormat(t *testing.T) {
	store := newMockUserStore()
	svc := NewService(store, "test-secret", time.Hour)

	_, err := svc.ValidateSession(context.Background(), "not-a-token")
	if err != domain.ErrUnauthorized {
		t.Fatalf("ValidateSession returned %v, want ErrUnauthorized", err)
	}
}
