package store

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"
)

const (
	MaxTOTPAttempts = 5
	LockoutDuration = 24 * time.Hour
)

// LockoutState persists TOTP failure tracking across app restarts.
type LockoutState struct {
	TOTPFailures int       `json:"totp_failures"` // consecutive wrong TOTP codes
	LockedUntil  time.Time `json:"locked_until"`  // zero-value = not locked
}

func lockoutPath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".secretvault", "lockout.json")
}

// LoadLockout reads the persisted lockout state (returns zero-value if absent).
func LoadLockout() LockoutState {
	raw, err := os.ReadFile(lockoutPath())
	if err != nil {
		return LockoutState{}
	}
	var s LockoutState
	if err := json.Unmarshal(raw, &s); err != nil {
		return LockoutState{}
	}
	// Auto-clear if lockout has expired
	if !s.LockedUntil.IsZero() && time.Now().After(s.LockedUntil) {
		s = LockoutState{}
		_ = saveLockout(s)
	}
	return s
}

func saveLockout(s LockoutState) error {
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(lockoutPath(), data, 0600)
}

// RecordTOTPFailure increments the failure counter and locks if threshold reached.
// Returns the updated state.
func RecordTOTPFailure() LockoutState {
	s := LoadLockout()
	s.TOTPFailures++
	if s.TOTPFailures >= MaxTOTPAttempts {
		s.LockedUntil = time.Now().Add(LockoutDuration)
	}
	_ = saveLockout(s)
	return s
}

// ResetTOTPFailures clears the failure counter (call on successful unlock).
func ResetTOTPFailures() {
	_ = saveLockout(LockoutState{})
}

// IsLockedOut returns true + remaining duration if the app is locked.
func IsLockedOut() (bool, time.Duration) {
	s := LoadLockout()
	if s.LockedUntil.IsZero() {
		return false, 0
	}
	remaining := time.Until(s.LockedUntil)
	if remaining <= 0 {
		// Expired — clear lockout
		ResetTOTPFailures()
		return false, 0
	}
	return true, remaining
}

// RemainingAttempts returns how many TOTP attempts are left before lockout.
func RemainingAttempts() int {
	s := LoadLockout()
	r := MaxTOTPAttempts - s.TOTPFailures
	if r < 0 {
		return 0
	}
	return r
}
