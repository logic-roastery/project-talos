package auth

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/logic-roastery/project-talos/internal/domain"
	"github.com/logic-roastery/project-talos/internal/store"
)

type Service struct {
	users  store.UserStore
	secret []byte
	maxAge time.Duration
}

func NewService(users store.UserStore, secret string, maxAge time.Duration) *Service {
	return &Service{
		users:  users,
		secret: []byte(secret),
		maxAge: maxAge,
	}
}

func (s *Service) SetupRequired(ctx context.Context) (bool, error) {
	has, err := s.users.HasUsers(ctx)
	if err != nil {
		return false, err
	}
	return !has, nil
}

func (s *Service) CreateUser(ctx context.Context, username, password string) (*domain.User, error) {
	hash, err := HashPassword(password)
	if err != nil {
		return nil, fmt.Errorf("hash password: %w", err)
	}

	user := &domain.User{
		Username:     username,
		PasswordHash: hash,
	}
	if err := s.users.CreateUser(ctx, user); err != nil {
		return nil, err
	}
	return user, nil
}

func (s *Service) Authenticate(ctx context.Context, username, password string) (*domain.User, error) {
	user, err := s.users.GetUserByUsername(ctx, username)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			return nil, domain.ErrUnauthorized
		}
		return nil, err
	}

	if !CheckPassword(password, user.PasswordHash) {
		return nil, domain.ErrUnauthorized
	}

	return user, nil
}

func (s *Service) CreateSession(user *domain.User) (string, error) {
	expires := time.Now().Add(s.maxAge).Unix()
	payload := fmt.Sprintf("%d:%d", user.ID, expires)

	mac := hmac.New(sha256.New, s.secret)
	mac.Write([]byte(payload))
	sig := base64.URLEncoding.EncodeToString(mac.Sum(nil))

	token := base64.URLEncoding.EncodeToString([]byte(payload)) + "." + sig
	return token, nil
}

func (s *Service) ValidateSession(ctx context.Context, token string) (*domain.User, error) {
	parts := strings.SplitN(token, ".", 2)
	if len(parts) != 2 {
		return nil, domain.ErrUnauthorized
	}

	payloadBytes, err := base64.URLEncoding.DecodeString(parts[0])
	if err != nil {
		return nil, domain.ErrUnauthorized
	}
	payload := string(payloadBytes)

	mac := hmac.New(sha256.New, s.secret)
	mac.Write([]byte(payload))
	expectedSig := base64.URLEncoding.EncodeToString(mac.Sum(nil))

	if !hmac.Equal([]byte(parts[1]), []byte(expectedSig)) {
		return nil, domain.ErrUnauthorized
	}

	pieces := strings.SplitN(payload, ":", 2)
	if len(pieces) != 2 {
		return nil, domain.ErrUnauthorized
	}

	userID, err := strconv.ParseInt(pieces[0], 10, 64)
	if err != nil {
		return nil, domain.ErrUnauthorized
	}

	expires, err := strconv.ParseInt(pieces[1], 10, 64)
	if err != nil {
		return nil, domain.ErrUnauthorized
	}

	if time.Now().Unix() > expires {
		return nil, domain.ErrUnauthorized
	}

	user, err := s.users.GetUserByUsername(ctx, "")
	if err != nil {
		return nil, domain.ErrUnauthorized
	}

	if user.ID != userID {
		return nil, domain.ErrUnauthorized
	}

	return user, nil
}

func generateRandomString(length int) (string, error) {
	b := make([]byte, length)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.URLEncoding.EncodeToString(b), nil
}
