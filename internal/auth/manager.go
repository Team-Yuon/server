package auth

import (
	"crypto/subtle"
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
}

type SignupToken struct {
	Token string
	Role  string
	Used  bool
}

type Manager struct {
	rootSecret string
	jwtSecret  []byte

	mu           sync.RWMutex
	users        map[string]*User
	emailToID    map[string]string
	signupTokens map[string]*SignupToken
}

func NewManager(rootSecret, jwtSecret string) *Manager {
	return &Manager{
		rootSecret:   rootSecret,
		jwtSecret:    []byte(jwtSecret),
		users:        make(map[string]*User),
		emailToID:    make(map[string]string),
		signupTokens: make(map[string]*SignupToken),
	}
}

func (m *Manager) IssueSignupToken(rootPassword, role string) (string, error) {
	if err := m.authenticateRoot(rootPassword); err != nil {
		return "", err
	}
	if role == "" {
		role = "user"
	}
	token := uuid.New().String()
	m.mu.Lock()
	m.signupTokens[token] = &SignupToken{Token: token, Role: role}
	m.mu.Unlock()
	return token, nil
}

func (m *Manager) Signup(signupToken, email, password string) (*User, error) {
	if signupToken == "" || email == "" || password == "" {
		return nil, errors.New("token, email, password are required")
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	token, ok := m.signupTokens[signupToken]
	if !ok || token.Used {
		return nil, errors.New("invalid or expired signup token")
	}

	if _, exists := m.emailToID[email]; exists {
		return nil, errors.New("email already registered")
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return nil, err
	}

	user := &User{
		ID:           uuid.New().String(),
		Email:        email,
		PasswordHash: hash,
		Role:         token.Role,
	}

	m.users[user.ID] = user
	m.emailToID[email] = user.ID
	token.Used = true

	return user, nil
}

func (m *Manager) Login(email, password string) (string, *User, error) {
	m.mu.RLock()
	userID, ok := m.emailToID[email]
	if !ok {
		m.mu.RUnlock()
		return "", nil, errors.New("invalid credentials")
	}
	user := m.users[userID]
	m.mu.RUnlock()

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

	m.mu.RLock()
	_, ok := m.users[claims.Subject]
	m.mu.RUnlock()
	if !ok {
		return nil, errors.New("user not found")
	}

	return claims, nil
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

func (m *Manager) authenticateRoot(password string) error {
	if m.rootSecret == "" {
		return errors.New("root secret is not configured")
	}
	if subtle.ConstantTimeCompare([]byte(password), []byte(m.rootSecret)) != 1 {
		return errors.New("invalid root password")
	}
	return nil
}
