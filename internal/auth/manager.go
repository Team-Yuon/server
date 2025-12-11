package auth

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
)

type User struct {
	ID           string
	Email        string
	PasswordHash []byte
	Role         string
	CreatedAt    time.Time
}

type Manager struct {
	jwtSecret []byte

	mu    sync.RWMutex
	store UserStore
}

func NewManager(jwtSecret string, store UserStore) *Manager {
	return &Manager{
		jwtSecret: []byte(jwtSecret),
		store:     store,
	}
}

func (m *Manager) EnsureRootUser(email, password string) error {
	if email == "" || password == "" {
		return errors.New("root email/password required")
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return err
	}

	user := &User{
		ID:           uuid.New().String(),
		Email:        email,
		PasswordHash: hash,
		Role:         "root",
	}

	if m.store == nil {
		return errors.New("user store is not configured")
	}
	return m.store.Upsert(context.Background(), user)
}

func (m *Manager) Signup(email, password, role string) (string, *User, error) {
	if email == "" || password == "" {
		return "", nil, errors.New("email and password are required")
	}

	if role == "" {
		role = "user"
	}

	if m.store == nil {
		return "", nil, errors.New("user store is not configured")
	}

	if existing, err := m.store.FindByEmail(context.Background(), email); err == nil && existing != nil {
		return "", nil, errors.New("email already registered")
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return "", nil, err
	}

	user := &User{
		ID:           uuid.New().String(),
		Email:        email,
		PasswordHash: hash,
		Role:         role,
	}

	if err := m.store.Create(context.Background(), user); err != nil {
		return "", nil, err
	}

	token, err := m.generateJWT(user)
	if err != nil {
		return "", nil, err
	}

	return token, user, nil
}

func (m *Manager) Login(email, password string) (string, *User, error) {
	if m.store == nil {
		return "", nil, errors.New("user store is not configured")
	}

	user, err := m.store.FindByEmail(context.Background(), email)
	if err != nil {
		return "", nil, errors.New("invalid credentials")
	}

	if err := bcrypt.CompareHashAndPassword(user.PasswordHash, []byte(password)); err != nil {
		return "", nil, errors.New("invalid credentials")
	}

	token, err := m.generateJWT(user)
	if err != nil {
		return "", nil, err
	}

	return token, user, nil
}

func (m *Manager) ValidateJWT(token string) (*Claims, error) {
	claims := &Claims{}
	parsed, err := jwt.ParseWithClaims(token, claims, func(t *jwt.Token) (interface{}, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, errors.New("unexpected signing method")
		}
		return m.jwtSecret, nil
	})
	if err != nil || !parsed.Valid {
		return nil, errors.New("invalid token")
	}

	if m.store != nil {
		if _, err := m.store.FindByID(context.Background(), claims.Subject); err != nil {
			return nil, errors.New("user not found")
		}
	}

	return claims, nil
}

// AllUsers returns a shallow copy of users for read-only purposes.
func (m *Manager) AllUsers() []*User {
	if m.store == nil {
		return nil
	}
	users, err := m.store.List(context.Background())
	if err != nil {
		return nil
	}
	return users
}

type Claims struct {
	jwt.RegisteredClaims
	Email string `json:"email"`
	Role  string `json:"role"`
}

func (m *Manager) generateJWT(user *User) (string, error) {
	claims := Claims{
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   user.ID,
			IssuedAt:  jwt.NewNumericDate(time.Now()),
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(24 * time.Hour)),
		},
		Email: user.Email,
		Role:  user.Role,
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString(m.jwtSecret)
}
