package main

// --- Recovery Key Methods ---

// ValidateRecoveryKey checks if the given recovery key is valid
func (a *App) ValidateRecoveryKey(recoveryKey string) Response {
	if err := a.store.ValidateRecoveryKey(recoveryKey); err != nil {
		return errorResp(err.Error())
	}
	return successResp(true)
}

// ResetPasswordWithRecovery resets the vault password using the recovery key.
// Returns a new recovery key that the user must save.
func (a *App) ResetPasswordWithRecovery(recoveryKey, newPassword string) Response {
	if len(newPassword) < 8 {
		return errorResp("New password must be at least 8 characters")
	}

	newRecoveryKey, err := a.store.ResetPasswordWithRecovery(recoveryKey, newPassword)
	if err != nil {
		return errorResp(err.Error())
	}

	return successResp(map[string]string{
		"recovery_key": newRecoveryKey,
	})
}
