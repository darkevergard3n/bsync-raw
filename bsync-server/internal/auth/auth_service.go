package auth

import (
	"database/sql"
	"errors"
	"fmt"
	"regexp"
	"time"

	"bsync-server/internal/models"
	"bsync-server/internal/repository"

	"github.com/dgrijalva/jwt-go"
	"golang.org/x/crypto/bcrypt"
)

var (
	ErrInvalidCredentials = errors.New("invalid username or password")
	ErrUserNotActive      = errors.New("user account is not active")
	ErrWeakPassword       = errors.New("password does not meet requirements")
	ErrInvalidToken       = errors.New("invalid or expired token")
)

// AuthService handles authentication operations
type AuthService struct {
	userRepo      *repository.UserRepository
	jwtSecret     []byte
	tokenDuration time.Duration
}

// NewAuthService creates a new authentication service
func NewAuthService(userRepo *repository.UserRepository, jwtSecret string, tokenDuration time.Duration) *AuthService {
	return &AuthService{
		userRepo:      userRepo,
		jwtSecret:     []byte(jwtSecret),
		tokenDuration: tokenDuration,
	}
}

// Login authenticates a user and returns a JWT token
func (s *AuthService) Login(username, password string) (*models.LoginResponse, error) {
	// Try to find user by username
	user, err := s.userRepo.GetUserByUsername(username)
	if err != nil {
		// Try by email if username not found
		user, err = s.userRepo.GetUserByEmail(username)
		if err != nil {
			return nil, ErrInvalidCredentials
		}
	}

	// Check if user is active
	if user.Status != models.StatusActive {
		return nil, ErrUserNotActive
	}

	// Verify password
	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(password)); err != nil {
		return nil, ErrInvalidCredentials
	}

	// Get user's assigned agents
	agentIDs, err := s.userRepo.GetUserAgents(user.ID)
	if err != nil {
		return nil, err
	}

	// Update last login
	_ = s.userRepo.UpdateLastLogin(user.ID)

	// Generate JWT token
	token, expiresIn, err := s.GenerateToken(user, agentIDs)
	if err != nil {
		return nil, err
	}

	return &models.LoginResponse{
		AccessToken: token,
		TokenType:   "Bearer",
		ExpiresIn:   expiresIn,
		User: models.UserInfo{
			ID:             user.ID,
			Username:       user.Username,
			Email:          user.Email,
			Fullname:       user.Fullname,
			Role:           user.Role,
			Status:         user.Status,
			AssignedAgents: agentIDs,
			LastLogin:      user.LastLogin,
		},
	}, nil
}

// GenerateToken generates a JWT token for a user
func (s *AuthService) GenerateToken(user *models.User, agentIDs []string) (string, int64, error) {
	now := time.Now()
	expiresAt := now.Add(s.tokenDuration)

	claims := models.JWTClaims{
		UserID:         user.ID,
		Username:       user.Username,
		Email:          user.Email,
		Fullname:       user.Fullname,
		Role:           user.Role,
		AssignedAgents: agentIDs,
		ExpiresAt:      expiresAt.Unix(),
		IssuedAt:       now.Unix(),
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"user_id":         claims.UserID,
		"username":        claims.Username,
		"email":           claims.Email,
		"fullname":        claims.Fullname,
		"role":            claims.Role,
		"assigned_agents": claims.AssignedAgents,
		"exp":             claims.ExpiresAt,
		"iat":             claims.IssuedAt,
	})

	tokenString, err := token.SignedString(s.jwtSecret)
	if err != nil {
		return "", 0, err
	}

	expiresIn := int64(s.tokenDuration.Seconds())
	return tokenString, expiresIn, nil
}

// ValidateToken validates a JWT token and returns the claims
func (s *AuthService) ValidateToken(tokenString string) (*models.JWTClaims, error) {
	token, err := jwt.Parse(tokenString, func(token *jwt.Token) (interface{}, error) {
		// Validate signing method
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}
		return s.jwtSecret, nil
	})

	if err != nil {
		return nil, ErrInvalidToken
	}

	if !token.Valid {
		return nil, ErrInvalidToken
	}

	claims, ok := token.Claims.(jwt.MapClaims)
	if !ok {
		return nil, ErrInvalidToken
	}

	// Extract claims
	jwtClaims := &models.JWTClaims{}

	if userID, ok := claims["user_id"].(float64); ok {
		jwtClaims.UserID = int(userID)
	}
	if username, ok := claims["username"].(string); ok {
		jwtClaims.Username = username
	}
	if email, ok := claims["email"].(string); ok {
		jwtClaims.Email = email
	}
	if fullname, ok := claims["fullname"].(string); ok {
		jwtClaims.Fullname = fullname
	}
	if role, ok := claims["role"].(string); ok {
		jwtClaims.Role = role
	}
	if exp, ok := claims["exp"].(float64); ok {
		jwtClaims.ExpiresAt = int64(exp)
	}
	if iat, ok := claims["iat"].(float64); ok {
		jwtClaims.IssuedAt = int64(iat)
	}
	if agents, ok := claims["assigned_agents"].([]interface{}); ok {
		for _, agent := range agents {
			if agentStr, ok := agent.(string); ok {
				jwtClaims.AssignedAgents = append(jwtClaims.AssignedAgents, agentStr)
			}
		}
	}

	// Check if token is expired
	if time.Now().Unix() > jwtClaims.ExpiresAt {
		return nil, ErrInvalidToken
	}

	return jwtClaims, nil
}

// HashPassword hashes a password using bcrypt
func (s *AuthService) HashPassword(password string) (string, error) {
	// Validate password strength
	if err := ValidatePasswordStrength(password); err != nil {
		return "", err
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return "", err
	}
	return string(hash), nil
}

// VerifyPassword verifies a password against a hash
func (s *AuthService) VerifyPassword(password, hash string) error {
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(password))
}

// ChangePassword changes a user's password
func (s *AuthService) ChangePassword(userID int, oldPassword, newPassword string) error {
	// Get user
	user, err := s.userRepo.GetUserByID(userID)
	if err != nil {
		return err
	}

	// Verify old password
	if err := s.VerifyPassword(oldPassword, user.PasswordHash); err != nil {
		return errors.New("incorrect old password")
	}

	// Validate new password
	if err := ValidatePasswordStrength(newPassword); err != nil {
		return err
	}

	// Hash new password
	newHash, err := s.HashPassword(newPassword)
	if err != nil {
		return err
	}

	// Update password
	updates := map[string]interface{}{
		"password_hash": newHash,
	}
	return s.userRepo.UpdateUser(userID, updates, userID)
}

// ValidatePasswordStrength validates password meets requirements
// Requirements:
// - Minimum 8 characters
// - At least 1 uppercase letter
// - At least 1 number
// - At least 1 special character
func ValidatePasswordStrength(password string) error {
	if len(password) < 8 {
		return fmt.Errorf("%w: minimum 8 characters required", ErrWeakPassword)
	}

	hasUpper := regexp.MustCompile(`[A-Z]`).MatchString(password)
	if !hasUpper {
		return fmt.Errorf("%w: must contain at least 1 uppercase letter", ErrWeakPassword)
	}

	hasNumber := regexp.MustCompile(`[0-9]`).MatchString(password)
	if !hasNumber {
		return fmt.Errorf("%w: must contain at least 1 number", ErrWeakPassword)
	}

	hasSpecial := regexp.MustCompile(`[!@#$%^&*()_+\-=\[\]{};':"\\|,.<>\/?~]`).MatchString(password)
	if !hasSpecial {
		return fmt.Errorf("%w: must contain at least 1 special character", ErrWeakPassword)
	}

	return nil
}

// RefreshToken generates a new token for a user (useful after agent assignments change)
func (s *AuthService) RefreshToken(userID int) (*models.LoginResponse, error) {
	user, err := s.userRepo.GetUserByID(userID)
	if err != nil {
		return nil, err
	}

	if user.Status != models.StatusActive {
		return nil, ErrUserNotActive
	}

	agentIDs, err := s.userRepo.GetUserAgents(user.ID)
	if err != nil {
		return nil, err
	}

	token, expiresIn, err := s.GenerateToken(user, agentIDs)
	if err != nil {
		return nil, err
	}

	return &models.LoginResponse{
		AccessToken: token,
		TokenType:   "Bearer",
		ExpiresIn:   expiresIn,
		User: models.UserInfo{
			ID:             user.ID,
			Username:       user.Username,
			Email:          user.Email,
			Fullname:       user.Fullname,
			Role:           user.Role,
			Status:         user.Status,
			AssignedAgents: agentIDs,
			LastLogin:      user.LastLogin,
		},
	}, nil
}

// LogActivity is a helper to log user activity
func (s *AuthService) LogActivity(userID int, username, action, resourceType, resourceID, ipAddress, userAgent string, details interface{}) error {
	log := &models.UserActivityLog{
		UserID:       sql.NullInt64{Int64: int64(userID), Valid: true},
		Username:     username,
		Action:       action,
		ResourceType: sql.NullString{String: resourceType, Valid: resourceType != ""},
		ResourceID:   sql.NullString{String: resourceID, Valid: resourceID != ""},
		IPAddress:    sql.NullString{String: ipAddress, Valid: ipAddress != ""},
		UserAgent:    sql.NullString{String: userAgent, Valid: userAgent != ""},
		Details:      details,
	}
	return s.userRepo.LogActivity(log)
}
