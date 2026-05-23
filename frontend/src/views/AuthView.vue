<script lang="ts" setup>
import { ref, onMounted, nextTick } from 'vue'
import { useRouter } from 'vue-router'
import { CheckVaultExists, CreateVault, UnlockVault, UnlockVaultReadOnly, ImportBackup, ValidateRecoveryKey, ResetPasswordWithRecovery, RestoreFromBackup } from '../../wailsjs/go/app/App'

const router = useRouter()
const vaultExists = ref(false)
const password = ref('')
const confirmPassword = ref('')
const totpCode = ref('')
const totpDigits = ref(['', '', '', '', '', ''])
const totpRefs = ref<HTMLInputElement[]>([])

function onTotpInput(index: number, e: Event) {
  const input = e.target as HTMLInputElement
  const val = input.value.replace(/\D/g, '').slice(-1)
  totpDigits.value[index] = val
  totpCode.value = totpDigits.value.join('')
  if (val && index < 5) {
    nextTick(() => totpRefs.value[index + 1]?.focus())
  }
}

function onTotpKeydown(index: number, e: KeyboardEvent) {
  if (e.key === 'Backspace') {
    if (totpDigits.value[index]) {
      totpDigits.value[index] = ''
      totpCode.value = totpDigits.value.join('')
    } else if (index > 0) {
      nextTick(() => totpRefs.value[index - 1]?.focus())
    }
  } else if (e.key === 'ArrowLeft' && index > 0) {
    totpRefs.value[index - 1]?.focus()
  } else if (e.key === 'ArrowRight' && index < 5) {
    totpRefs.value[index + 1]?.focus()
  }
}

function onTotpPaste(e: ClipboardEvent) {
  e.preventDefault()
  const text = e.clipboardData?.getData('text').replace(/\D/g, '').slice(0, 6) ?? ''
  text.split('').forEach((ch, i) => { totpDigits.value[i] = ch })
  totpCode.value = totpDigits.value.join('')
  nextTick(() => {
    const nextEmpty = totpDigits.value.findIndex(d => !d)
    totpRefs.value[nextEmpty === -1 ? 5 : nextEmpty]?.focus()
  })
}
const showTotp = ref(false)
const error = ref('')
const loading = ref(false)
const showPassword = ref(false)

// TOTP lockout state
const totpRemainingAttempts = ref(5)
const totpLocked = ref(false)
const totpLockedMessage = ref('')
const totpLockedSeconds = ref(0)
const totpCountdown = ref('')
let countdownTimer: ReturnType<typeof setInterval> | null = null

// Recovery key modal state (shown after vault creation)
const recoveryKeyModal = ref(false)
const recoveryKey = ref('')
const recoveryKeyCopied = ref(false)

// Forgot password flow state
const forgotPasswordMode = ref(false)
const recoveryKeyInput = ref('')
const newPassword = ref('')
const confirmNewPassword = ref('')
const recoveryValidated = ref(false)
const forgotError = ref('')
const forgotLoading = ref(false)

// New recovery key shown after password reset
const newRecoveryKeyModal = ref(false)
const newRecoveryKey = ref('')
const newRecoveryKeyCopied = ref(false)

// Tamper detection modal state
const tamperModal = ref(false)
const tamperDetail = ref('')
const tamperPassword = ref('') // saved from the failed unlock attempt
const tamperLoading = ref<'restore'|'open'|null>(null)
const tamperError = ref('')
const tamperRecoveryKey = ref('')
const showTamperRecoveryInput = ref(false)

// Restore from backup flow (first launch, no vault exists)
const restoreMode = ref(false)
const restoreRecoveryKey = ref('')
const restorePassword = ref('')
const restoreConfirmPassword = ref('')
const restoreError = ref('')
const restoreLoading = ref(false)

onMounted(async () => {
  const res = await CheckVaultExists()
  if (res.success) {
    vaultExists.value = res.data as boolean
  }
})

async function handleCreate() {
  error.value = ''
  if (password.value.length < 8) {
    error.value = 'Password must be at least 8 characters'
    return
  }
  if (password.value !== confirmPassword.value) {
    error.value = 'Passwords do not match'
    return
  }
  loading.value = true
  try {
    const res = await CreateVault(password.value)
    if (res.success) {
      const data = res.data as { recovery_key: string }
      recoveryKey.value = data.recovery_key
      recoveryKeyModal.value = true
    } else {
      error.value = res.error || 'Failed to create vault'
    }
  } catch (e: any) {
    error.value = e.message || 'An error occurred'
  }
  loading.value = false
}

function copyRecoveryKey() {
  navigator.clipboard.writeText(recoveryKey.value)
  recoveryKeyCopied.value = true
  setTimeout(() => { recoveryKeyCopied.value = false }, 2000)
}

function handleRecoveryKeyDismiss() {
  recoveryKeyModal.value = false
  router.push('/vault')
}

// --- Forgot Password Flow ---

function enterForgotPassword() {
  forgotPasswordMode.value = true
  recoveryValidated.value = false
  recoveryKeyInput.value = ''
  newPassword.value = ''
  confirmNewPassword.value = ''
  forgotError.value = ''
}

function exitForgotPassword() {
  forgotPasswordMode.value = false
  forgotError.value = ''
}

async function handleValidateRecoveryKey() {
  forgotError.value = ''
  forgotLoading.value = true
  try {
    const res = await ValidateRecoveryKey(recoveryKeyInput.value)
    if (res.success) {
      recoveryValidated.value = true
    } else {
      forgotError.value = res.error || 'Invalid recovery key'
    }
  } catch (e: any) {
    forgotError.value = e.message || 'An error occurred'
  }
  forgotLoading.value = false
}

async function handleResetPassword() {
  forgotError.value = ''
  if (newPassword.value.length < 8) {
    forgotError.value = 'Password must be at least 8 characters'
    return
  }
  if (newPassword.value !== confirmNewPassword.value) {
    forgotError.value = 'Passwords do not match'
    return
  }
  forgotLoading.value = true
  try {
    const res = await ResetPasswordWithRecovery(recoveryKeyInput.value, newPassword.value)
    if (res.success) {
      const data = res.data as { recovery_key: string }
      newRecoveryKey.value = data.recovery_key
      newRecoveryKeyCopied.value = false
      forgotPasswordMode.value = false
      newRecoveryKeyModal.value = true
    } else {
      forgotError.value = res.error || 'Failed to reset password'
    }
  } catch (e: any) {
    forgotError.value = e.message || 'An error occurred'
  }
  forgotLoading.value = false
}

function copyNewRecoveryKey() {
  navigator.clipboard.writeText(newRecoveryKey.value)
  newRecoveryKeyCopied.value = true
  setTimeout(() => { newRecoveryKeyCopied.value = false }, 2000)
}

function handleNewRecoveryKeyDismiss() {
  newRecoveryKeyModal.value = false
  router.push('/vault')
}

function startLockoutCountdown(seconds: number) {
  totpLocked.value = true
  totpLockedSeconds.value = seconds
  if (countdownTimer) clearInterval(countdownTimer)
  const updateCountdown = () => {
    const h = Math.floor(totpLockedSeconds.value / 3600)
    const m = Math.floor((totpLockedSeconds.value % 3600) / 60)
    const s = totpLockedSeconds.value % 60
    totpCountdown.value = h > 0 ? `${h}h ${m}m ${s}s` : m > 0 ? `${m}m ${s}s` : `${s}s`
  }
  updateCountdown()
  countdownTimer = setInterval(() => {
    totpLockedSeconds.value--
    if (totpLockedSeconds.value <= 0) {
      totpLocked.value = false
      totpLockedMessage.value = ''
      totpCountdown.value = ''
      totpRemainingAttempts.value = 5
      if (countdownTimer) clearInterval(countdownTimer)
      countdownTimer = null
      return
    }
    updateCountdown()
  }, 1000)
}

async function handleUnlock() {
  error.value = ''
  loading.value = true
  try {
    const res = await UnlockVault(password.value, totpCode.value)
    if (res.success) {
      if (countdownTimer) clearInterval(countdownTimer)
      // Check if vault was migrated from v1 → v2
      const data = res.data as any
      if (data?.migrated && data?.recovery_key) {
        recoveryKey.value = data.recovery_key
        recoveryKeyModal.value = true
      } else {
        router.push('/vault')
      }
    } else if (res.error === 'TOTP_REQUIRED') {
      showTotp.value = true
      totpDigits.value = ['', '', '', '', '', '']
      totpCode.value = ''
      error.value = ''
      const data = res.data as any
      if (data?.remaining_attempts !== undefined) {
        totpRemainingAttempts.value = data.remaining_attempts
      }
      nextTick(() => totpRefs.value[0]?.focus())
    } else if (res.error === 'TOTP_INVALID') {
      const data = res.data as any
      totpRemainingAttempts.value = data?.remaining_attempts ?? 0
      totpDigits.value = ['', '', '', '', '', '']
      totpCode.value = ''
      error.value = 'Invalid TOTP code'
      nextTick(() => totpRefs.value[0]?.focus())
    } else if (res.error === 'TOTP_LOCKED') {
      const data = res.data as any
      totpLockedMessage.value = data?.message ?? 'App is locked.'
      startLockoutCountdown(data?.remaining_seconds ?? 86400)
      error.value = ''
    } else if (res.error === 'VAULT_TAMPERED') {
      tamperPassword.value = password.value
      tamperDetail.value = (res.data as any)?.detail ?? ''
      tamperModal.value = true
      tamperError.value = ''
    } else {
      error.value = res.error || 'Failed to unlock vault'
    }
  } catch (e: any) {
    error.value = e.message || 'An error occurred'
  }
  loading.value = false
}

async function handleOpenReadOnly() {
  tamperLoading.value = 'open'
  tamperError.value = ''
  const res = await UnlockVaultReadOnly(tamperPassword.value)
  tamperLoading.value = null
  if (res.success) {
    tamperModal.value = false
    // Check if vault was migrated from v1 → v2
    const data = res.data as any
    if (data?.migrated && data?.recovery_key) {
      recoveryKey.value = data.recovery_key
      recoveryKeyModal.value = true
    } else {
      router.push('/vault')
    }
  } else {
    tamperError.value = res.error || 'Failed to open vault'
  }
}

async function handleRestoreFromBackup() {
  if (!showTamperRecoveryInput.value) {
    showTamperRecoveryInput.value = true
    tamperRecoveryKey.value = ''
    return
  }
  if (!tamperRecoveryKey.value.trim()) {
    tamperError.value = 'Please enter your Recovery Key'
    return
  }
  tamperLoading.value = 'restore'
  tamperError.value = ''
  const res = await ImportBackup(tamperRecoveryKey.value.trim())
  tamperLoading.value = null
  if (res.success) {
    tamperModal.value = false
    showTamperRecoveryInput.value = false
    router.push('/vault')
  } else if (res.error !== 'No file selected') {
    tamperError.value = res.error || 'Restore failed'
  }
}

// --- Restore from Backup (first launch) ---

function enterRestoreMode() {
  restoreMode.value = true
  restoreRecoveryKey.value = ''
  restorePassword.value = ''
  restoreConfirmPassword.value = ''
  restoreError.value = ''
}

function exitRestoreMode() {
  restoreMode.value = false
  restoreError.value = ''
}

async function handleRestoreFromBackupFirstLaunch() {
  restoreError.value = ''
  if (!restoreRecoveryKey.value.trim()) {
    restoreError.value = 'Recovery Key is required'
    return
  }
  if (restorePassword.value.length < 8) {
    restoreError.value = 'Password must be at least 8 characters'
    return
  }
  if (restorePassword.value !== restoreConfirmPassword.value) {
    restoreError.value = 'Passwords do not match'
    return
  }
  restoreLoading.value = true
  try {
    const res = await RestoreFromBackup(restorePassword.value, restoreRecoveryKey.value.trim())
    if (res.success) {
      const data = res.data as any
      recoveryKey.value = data.recovery_key
      recoveryKeyModal.value = true
      restoreMode.value = false
    } else {
      restoreError.value = res.error || 'Restore failed'
    }
  } catch (e: any) {
    restoreError.value = e.message || 'An error occurred'
  }
  restoreLoading.value = false
}

function handleSubmit() {
  if (vaultExists.value) {
    handleUnlock()
  } else {
    handleCreate()
  }
}
</script>

<template>
  <div class="min-h-screen flex items-center justify-center bg-vault-bg p-4">
    <div class="w-full max-w-md">
      <!-- Logo & Header -->
      <div class="text-center mb-8">
        <div class="w-20 h-20 mx-auto mb-4 rounded-2xl bg-gradient-to-br from-vault-accent to-purple-500 flex items-center justify-center shadow-lg shadow-vault-accent/20">
          <svg class="w-10 h-10 text-white" fill="none" stroke="currentColor" viewBox="0 0 24 24">
            <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M12 15v2m-6 4h12a2 2 0 002-2v-6a2 2 0 00-2-2H6a2 2 0 00-2 2v6a2 2 0 002 2zm10-10V7a4 4 0 00-8 0v4h8z" />
          </svg>
        </div>
        <h1 class="text-3xl font-bold text-vault-text tracking-tight">Secret Vault</h1>
        <p class="text-vault-text-secondary mt-2 text-sm">
          {{ restoreMode
            ? 'Restore your vault from a backup file'
            : forgotPasswordMode
              ? (recoveryValidated ? 'Set your new password' : 'Enter your Recovery Key')
              : (vaultExists ? 'Enter your password to unlock' : 'Create a new vault to get started') }}
        </p>
      </div>

      <!-- Card -->
      <div class="bg-vault-surface border border-vault-border rounded-2xl p-8 shadow-2xl shadow-black/20">

        <!-- ─── Restore from Backup Flow (first launch) ─── -->
        <template v-if="restoreMode">
          <div class="space-y-5">
            <div>
              <label class="block text-sm font-medium text-vault-text-secondary mb-2">Recovery Key</label>
              <input
                type="text"
                v-model="restoreRecoveryKey"
                placeholder="XXXX-XXXX-XXXX-XXXX-XXXX-XXXX"
                class="w-full px-4 py-3 bg-vault-card border border-vault-border rounded-xl text-vault-text placeholder-vault-text-secondary/50 focus:outline-none focus:border-vault-accent focus:ring-1 focus:ring-vault-accent transition-all font-mono tracking-wider text-center uppercase"
                autofocus
              />
              <p class="text-[11px] text-vault-text-secondary/60 mt-1.5">Enter the Recovery Key used when the backup was exported</p>
            </div>

            <div>
              <label class="block text-sm font-medium text-vault-text-secondary mb-2">New Password</label>
              <input
                type="password"
                v-model="restorePassword"
                placeholder="Minimum 8 characters"
                class="w-full px-4 py-3 bg-vault-card border border-vault-border rounded-xl text-vault-text placeholder-vault-text-secondary/50 focus:outline-none focus:border-vault-accent focus:ring-1 focus:ring-vault-accent transition-all"
              />
            </div>

            <div>
              <label class="block text-sm font-medium text-vault-text-secondary mb-2">Confirm Password</label>
              <input
                type="password"
                v-model="restoreConfirmPassword"
                placeholder="Confirm your new password"
                class="w-full px-4 py-3 bg-vault-card border border-vault-border rounded-xl text-vault-text placeholder-vault-text-secondary/50 focus:outline-none focus:border-vault-accent focus:ring-1 focus:ring-vault-accent transition-all"
              />
            </div>

            <div v-if="restoreError" class="bg-vault-danger/10 border border-vault-danger/20 text-vault-danger text-sm rounded-xl px-4 py-3">
              {{ restoreError }}
            </div>

            <button
              @click="handleRestoreFromBackupFirstLaunch"
              :disabled="restoreLoading"
              class="w-full py-3 bg-gradient-to-r from-vault-accent to-purple-500 text-white font-semibold rounded-xl hover:from-vault-accent-hover hover:to-purple-400 transition-all duration-200 disabled:opacity-50 disabled:cursor-not-allowed shadow-lg shadow-vault-accent/20 active:scale-[0.98]"
            >
              <span v-if="restoreLoading" class="flex items-center justify-center gap-2">
                <svg class="animate-spin w-5 h-5" fill="none" viewBox="0 0 24 24">
                  <circle class="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" stroke-width="4"></circle>
                  <path class="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4z"></path>
                </svg>
                Restoring...
              </span>
              <span v-else>Select Backup & Restore</span>
            </button>

            <button
              @click="exitRestoreMode"
              class="w-full py-2.5 text-vault-text-secondary hover:text-vault-text text-sm transition-colors"
            >
              ← Back
            </button>
          </div>
        </template>

        <!-- ─── Forgot Password Flow ─── -->
        <template v-else-if="forgotPasswordMode">
          <!-- Step 1: Recovery Key Input -->
          <div v-if="!recoveryValidated" class="space-y-5">
            <div>
              <label class="block text-sm font-medium text-vault-text-secondary mb-2">Recovery Key</label>
              <input
                type="text"
                v-model="recoveryKeyInput"
                placeholder="XXXX-XXXX-XXXX-XXXX-XXXX-XXXX"
                class="w-full px-4 py-3 bg-vault-card border border-vault-border rounded-xl text-vault-text placeholder-vault-text-secondary/50 focus:outline-none focus:border-vault-accent focus:ring-1 focus:ring-vault-accent transition-all font-mono tracking-wider text-center"
                autofocus
              />
            </div>

            <div v-if="forgotError" class="bg-vault-danger/10 border border-vault-danger/20 text-vault-danger text-sm rounded-xl px-4 py-3">
              {{ forgotError }}
            </div>

            <button
              @click="handleValidateRecoveryKey"
              :disabled="forgotLoading || !recoveryKeyInput.trim()"
              class="w-full py-3 bg-gradient-to-r from-vault-accent to-purple-500 text-white font-semibold rounded-xl hover:from-vault-accent-hover hover:to-purple-400 transition-all duration-200 disabled:opacity-50 disabled:cursor-not-allowed shadow-lg shadow-vault-accent/20 active:scale-[0.98]"
            >
              <span v-if="forgotLoading" class="flex items-center justify-center gap-2">
                <svg class="animate-spin w-5 h-5" fill="none" viewBox="0 0 24 24">
                  <circle class="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" stroke-width="4"></circle>
                  <path class="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4z"></path>
                </svg>
                Verifying...
              </span>
              <span v-else>Verify Recovery Key</span>
            </button>

            <button
              @click="exitForgotPassword"
              class="w-full py-2.5 text-vault-text-secondary hover:text-vault-text text-sm transition-colors"
            >
              ← Back to Login
            </button>
          </div>

          <!-- Step 2: New Password -->
          <div v-else class="space-y-5">
            <div>
              <label class="block text-sm font-medium text-vault-text-secondary mb-2">New Password</label>
              <input
                type="password"
                v-model="newPassword"
                placeholder="Minimum 8 characters"
                class="w-full px-4 py-3 bg-vault-card border border-vault-border rounded-xl text-vault-text placeholder-vault-text-secondary/50 focus:outline-none focus:border-vault-accent focus:ring-1 focus:ring-vault-accent transition-all"
                autofocus
              />
            </div>

            <div>
              <label class="block text-sm font-medium text-vault-text-secondary mb-2">Confirm New Password</label>
              <input
                type="password"
                v-model="confirmNewPassword"
                placeholder="Confirm your new password"
                class="w-full px-4 py-3 bg-vault-card border border-vault-border rounded-xl text-vault-text placeholder-vault-text-secondary/50 focus:outline-none focus:border-vault-accent focus:ring-1 focus:ring-vault-accent transition-all"
              />
            </div>

            <div v-if="forgotError" class="bg-vault-danger/10 border border-vault-danger/20 text-vault-danger text-sm rounded-xl px-4 py-3">
              {{ forgotError }}
            </div>

            <button
              @click="handleResetPassword"
              :disabled="forgotLoading"
              class="w-full py-3 bg-gradient-to-r from-vault-accent to-purple-500 text-white font-semibold rounded-xl hover:from-vault-accent-hover hover:to-purple-400 transition-all duration-200 disabled:opacity-50 disabled:cursor-not-allowed shadow-lg shadow-vault-accent/20 active:scale-[0.98]"
            >
              <span v-if="forgotLoading" class="flex items-center justify-center gap-2">
                <svg class="animate-spin w-5 h-5" fill="none" viewBox="0 0 24 24">
                  <circle class="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" stroke-width="4"></circle>
                  <path class="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4z"></path>
                </svg>
                Resetting...
              </span>
              <span v-else>Reset Password</span>
            </button>

            <button
              @click="exitForgotPassword"
              class="w-full py-2.5 text-vault-text-secondary hover:text-vault-text text-sm transition-colors"
            >
              ← Back to Login
            </button>
          </div>
        </template>

        <!-- ─── Normal Login / Create Flow ─── -->
        <template v-else>
          <form @submit.prevent="handleSubmit" class="space-y-5">
          <!-- Password -->
          <div>
            <label class="block text-sm font-medium text-vault-text-secondary mb-2">
              {{ vaultExists ? 'Password' : 'Create Password' }}
            </label>
            <div class="relative">
              <input
                :type="showPassword ? 'text' : 'password'"
                v-model="password"
                :placeholder="vaultExists ? 'Enter your master password' : 'Minimum 8 characters'"
                class="w-full px-4 py-3 bg-vault-card border border-vault-border rounded-xl text-vault-text placeholder-vault-text-secondary/50 focus:outline-none focus:border-vault-accent focus:ring-1 focus:ring-vault-accent transition-all"
                autofocus
              />
              <button
                type="button"
                @click="showPassword = !showPassword"
                class="absolute right-3 top-1/2 -translate-y-1/2 text-vault-text-secondary hover:text-vault-text transition-colors"
              >
                <svg v-if="showPassword" class="w-5 h-5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                  <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M13.875 18.825A10.05 10.05 0 0112 19c-4.478 0-8.268-2.943-9.543-7a9.97 9.97 0 011.563-3.029m5.858.908a3 3 0 114.243 4.243M9.878 9.878l4.242 4.242M9.88 9.88l-3.29-3.29m7.532 7.532l3.29 3.29M3 3l3.59 3.59m0 0A9.953 9.953 0 0112 5c4.478 0 8.268 2.943 9.543 7a10.025 10.025 0 01-4.132 5.411m0 0L21 21" />
                </svg>
                <svg v-else class="w-5 h-5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                  <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M15 12a3 3 0 11-6 0 3 3 0 016 0z" />
                  <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M2.458 12C3.732 7.943 7.523 5 12 5c4.478 0 8.268 2.943 9.542 7-1.274 4.057-5.064 7-9.542 7-4.477 0-8.268-2.943-9.542-7z" />
                </svg>
              </button>
            </div>
          </div>

          <!-- Confirm Password (Create mode) -->
          <div v-if="!vaultExists">
            <label class="block text-sm font-medium text-vault-text-secondary mb-2">Confirm Password</label>
            <input
              :type="showPassword ? 'text' : 'password'"
              v-model="confirmPassword"
              placeholder="Confirm your password"
              class="w-full px-4 py-3 bg-vault-card border border-vault-border rounded-xl text-vault-text placeholder-vault-text-secondary/50 focus:outline-none focus:border-vault-accent focus:ring-1 focus:ring-vault-accent transition-all"
            />
          </div>

          <!-- TOTP Code -->
          <div v-if="showTotp">
            <label class="block text-sm font-medium text-vault-text-secondary mb-3">
              Authenticator Code
            </label>

            <!-- Lockout banner -->
            <div v-if="totpLocked" class="mb-3 bg-vault-danger/10 border border-vault-danger/20 rounded-xl px-4 py-3">
              <div class="flex items-start gap-2">
                <svg class="w-4 h-4 text-vault-danger flex-shrink-0 mt-0.5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                  <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M12 15v2m-6 4h12a2 2 0 002-2v-6a2 2 0 00-2-2H6a2 2 0 00-2 2v6a2 2 0 002 2zm10-10V7a4 4 0 00-8 0v4h8z" />
                </svg>
                <div>
                  <p class="text-xs font-semibold text-vault-danger">App bị khóa do nhập sai OTP quá nhiều lần</p>
                  <p class="text-xs text-vault-text-secondary mt-1">Thử lại sau: <span class="font-mono font-bold text-vault-danger">{{ totpCountdown }}</span></p>
                </div>
              </div>
            </div>

            <div :class="{ 'opacity-50 pointer-events-none': totpLocked }">
              <div class="flex gap-2 justify-center" @paste.prevent="onTotpPaste">
                <input
                  v-for="(_, i) in totpDigits"
                  :key="i"
                  :ref="el => { if (el) totpRefs[i] = el as HTMLInputElement }"
                  type="text"
                  inputmode="numeric"
                  maxlength="1"
                  :value="totpDigits[i]"
                  @input="onTotpInput(i, $event)"
                  @keydown="onTotpKeydown(i, $event)"
                  :disabled="totpLocked"
                  :class="['w-11 h-14 text-center text-xl font-bold rounded-xl border transition-all bg-vault-card text-vault-text',
                    totpDigits[i]
                      ? 'border-vault-accent ring-1 ring-vault-accent/50 shadow-sm shadow-vault-accent/20'
                      : 'border-vault-border focus:border-vault-accent focus:ring-1 focus:ring-vault-accent']"
                />
              </div>
              <p class="text-center text-[11px] text-vault-text-secondary/60 mt-2">Enter the 6-digit code from your authenticator app</p>
            </div>

            <!-- Remaining attempts warning -->
            <div v-if="!totpLocked && totpRemainingAttempts < 5" class="mt-3 bg-vault-warning/10 border border-vault-warning/20 rounded-xl px-4 py-2.5">
              <div class="flex items-center gap-2">
                <svg class="w-4 h-4 text-vault-warning flex-shrink-0" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                  <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M12 9v2m0 4h.01M10.29 3.86L1.82 18a2 2 0 001.71 3h16.94a2 2 0 001.71-3L13.71 3.86a2 2 0 00-3.42 0z"/>
                </svg>
                <p class="text-xs text-vault-warning">
                  Còn <strong>{{ totpRemainingAttempts }}</strong> lần thử. Nhập sai quá {{ 5 }} lần sẽ bị khóa app trong 24 giờ.
                </p>
              </div>
            </div>
          </div>

          <!-- Error message -->
          <div v-if="error" class="bg-vault-danger/10 border border-vault-danger/20 text-vault-danger text-sm rounded-xl px-4 py-3">
            {{ error }}
          </div>

          <!-- Submit Button -->
          <button
            type="submit"
            :disabled="loading || totpLocked"
            class="w-full py-3 bg-gradient-to-r from-vault-accent to-purple-500 text-white font-semibold rounded-xl hover:from-vault-accent-hover hover:to-purple-400 transition-all duration-200 disabled:opacity-50 disabled:cursor-not-allowed shadow-lg shadow-vault-accent/20 active:scale-[0.98]"
          >
            <span v-if="loading" class="flex items-center justify-center gap-2">
              <svg class="animate-spin w-5 h-5" fill="none" viewBox="0 0 24 24">
                <circle class="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" stroke-width="4"></circle>
                <path class="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4z"></path>
              </svg>
              Processing...
            </span>
            <span v-else>
              {{ vaultExists ? (showTotp ? 'Verify & Unlock' : 'Unlock Vault') : 'Create Vault' }}
            </span>
          </button>

          <!-- Forgot Password Link -->
          <div v-if="vaultExists" class="text-center">
            <button
              type="button"
              @click="enterForgotPassword"
              class="text-sm text-vault-accent hover:text-vault-accent-hover transition-colors"
            >
              Forgot password?
            </button>
          </div>

          <!-- Restore from Backup Link (no vault exists) -->
          <div v-if="!vaultExists" class="text-center">
            <button
              type="button"
              @click="enterRestoreMode"
              class="text-sm text-vault-accent hover:text-vault-accent-hover transition-colors"
            >
              Restore from Backup
            </button>
          </div>
        </form>
        </template>

        <!-- Security Info -->
        <div class="mt-6 pt-5 border-t border-vault-border">
          <div class="flex items-center gap-2 text-vault-text-secondary text-xs">
            <svg class="w-4 h-4 text-vault-success flex-shrink-0" fill="none" stroke="currentColor" viewBox="0 0 24 24">
              <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M9 12l2 2 4-4m5.618-4.016A11.955 11.955 0 0112 2.944a11.955 11.955 0 01-8.618 3.04A12.02 12.02 0 003 9c0 5.591 3.824 10.29 9 11.622 5.176-1.332 9-6.03 9-11.622 0-1.042-.133-2.052-.382-3.016z" />
            </svg>
            <span>AES-256 encryption · ECDSA signatures · PBKDF2 key derivation</span>
          </div>
        </div>
      </div>
    </div>
  </div>

  <!-- ── Tamper Detection Modal ── -->
  <Transition enter-from-class="opacity-0" enter-active-class="transition-opacity duration-200"
              leave-to-class="opacity-0" leave-active-class="transition-opacity duration-200">
    <div v-if="tamperModal" class="fixed inset-0 z-50 flex items-center justify-center p-4 bg-black/70 backdrop-blur-sm">
      <div class="w-full max-w-lg bg-vault-surface border border-vault-danger/40 rounded-2xl shadow-2xl overflow-hidden">

        <!-- Header -->
        <div class="flex items-start gap-4 px-6 py-5 bg-vault-danger/10 border-b border-vault-danger/20">
          <div class="w-10 h-10 rounded-xl bg-vault-danger/20 flex items-center justify-center flex-shrink-0 mt-0.5">
            <svg class="w-5 h-5 text-vault-danger" fill="none" stroke="currentColor" viewBox="0 0 24 24">
              <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2"
                d="M12 9v2m0 4h.01M10.29 3.86L1.82 18a2 2 0 001.71 3h16.94a2 2 0 001.71-3L13.71 3.86a2 2 0 00-3.42 0z"/>
            </svg>
          </div>
          <div>
            <h2 class="text-base font-bold text-vault-danger">Vault Integrity Violation</h2>
            <p class="text-xs text-vault-text-secondary mt-1 leading-relaxed">
              Chữ ký ECDSA của vault không khớp với nội dung. Dữ liệu có thể đã bị chỉnh sửa bên ngoài ứng dụng.
            </p>
          </div>
        </div>

        <!-- Detail -->
        <div class="px-6 py-4">
          <div class="bg-vault-card/60 border border-vault-border rounded-xl px-4 py-3 mb-4">
            <p class="text-[11px] font-mono text-vault-text-secondary leading-relaxed break-words">
              {{ tamperDetail || 'ECDSA signature verification failed for encrypted_data field.' }}
            </p>
          </div>

          <!-- Error inside modal -->
          <div v-if="tamperError" class="mb-4 bg-vault-danger/10 border border-vault-danger/20 text-vault-danger text-xs rounded-xl px-4 py-2.5">
            {{ tamperError }}
          </div>

          <!-- 3 options -->
          <div class="space-y-2.5">

            <!-- Option 1: Restore from Backup -->
            <button @click="handleRestoreFromBackup"
              :disabled="tamperLoading !== null"
              class="w-full flex items-center gap-4 px-4 py-3.5 rounded-xl border border-vault-accent/30 bg-vault-accent/5 hover:bg-vault-accent/10 transition-all disabled:opacity-50 disabled:cursor-not-allowed group">
              <div class="w-9 h-9 rounded-lg bg-vault-accent/15 group-hover:bg-vault-accent/25 flex items-center justify-center flex-shrink-0 transition-colors">
                <svg v-if="tamperLoading==='restore'" class="w-4 h-4 text-vault-accent animate-spin" fill="none" viewBox="0 0 24 24">
                  <circle class="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" stroke-width="4"/>
                  <path class="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4z"/>
                </svg>
                <svg v-else class="w-4 h-4 text-vault-accent" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                  <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2"
                    d="M4 16v1a3 3 0 003 3h10a3 3 0 003-3v-1M12 12V4m0 0L8 8m4-4l4 4"/>
                </svg>
              </div>
              <div class="text-left">
                <p class="text-sm font-semibold text-vault-text">Khôi phục từ Backup</p>
                <p class="text-[11px] text-vault-text-secondary mt-0.5">
                  Chọn file backup đã được ký số ECDSA hợp lệ để phục hồi dữ liệu an toàn
                </p>
              </div>
            </button>

            <!-- Recovery key input for tamper restore -->
            <div v-if="showTamperRecoveryInput" class="px-1">
              <input
                v-model="tamperRecoveryKey"
                type="text"
                placeholder="XXXX-XXXX-XXXX-XXXX-XXXX-XXXX"
                class="w-full px-4 py-2.5 bg-vault-card border border-vault-border rounded-xl text-vault-text text-sm font-mono tracking-wider placeholder-vault-text-secondary/30 focus:outline-none focus:border-vault-accent transition-colors uppercase"
                autofocus
              />
              <p class="text-[10px] text-vault-text-secondary mt-1">Nhập Recovery Key được dùng khi export backup</p>
            </div>

            <!-- Option 2: Open Read-Only -->
            <button @click="handleOpenReadOnly"
              :disabled="tamperLoading !== null"
              class="w-full flex items-center gap-4 px-4 py-3.5 rounded-xl border border-vault-warning/30 bg-vault-warning/5 hover:bg-vault-warning/10 transition-all disabled:opacity-50 disabled:cursor-not-allowed group">
              <div class="w-9 h-9 rounded-lg bg-vault-warning/15 group-hover:bg-vault-warning/25 flex items-center justify-center flex-shrink-0 transition-colors">
                <svg v-if="tamperLoading==='open'" class="w-4 h-4 text-vault-warning animate-spin" fill="none" viewBox="0 0 24 24">
                  <circle class="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" stroke-width="4"/>
                  <path class="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4z"/>
                </svg>
                <svg v-else class="w-4 h-4 text-vault-warning" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                  <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2"
                    d="M15 12a3 3 0 11-6 0 3 3 0 016 0z"/>
                  <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2"
                    d="M2.458 12C3.732 7.943 7.523 5 12 5c4.478 0 8.268 2.943 9.542 7-1.274 4.057-5.064 7-9.542 7-4.477 0-8.268-2.943-9.542-7z"/>
                </svg>
              </div>
              <div class="text-left">
                <div class="flex items-center gap-2">
                  <p class="text-sm font-semibold text-vault-text">Mở ở chế độ Read-Only</p>
                  <span class="text-[9px] font-bold uppercase tracking-wide bg-vault-warning/20 text-vault-warning px-1.5 py-0.5 rounded-full">Rủi ro</span>
                </div>
                <p class="text-[11px] text-vault-text-secondary mt-0.5">
                  Bỏ qua xác minh chữ ký, chỉ xem — không thể chỉnh sửa hay lưu dữ liệu mới
                </p>
              </div>
            </button>

            <!-- Option 3: Cancel -->
            <button @click="tamperModal = false"
              :disabled="tamperLoading !== null"
              class="w-full flex items-center gap-4 px-4 py-3.5 rounded-xl border border-vault-border hover:bg-vault-card/50 transition-all disabled:opacity-50 disabled:cursor-not-allowed group">
              <div class="w-9 h-9 rounded-lg bg-vault-card group-hover:bg-vault-border/40 flex items-center justify-center flex-shrink-0 transition-colors">
                <svg class="w-4 h-4 text-vault-text-secondary" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                  <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M6 18L18 6M6 6l12 12"/>
                </svg>
              </div>
              <div class="text-left">
                <p class="text-sm font-semibold text-vault-text">Huỷ bỏ</p>
                <p class="text-[11px] text-vault-text-secondary mt-0.5">Quay lại màn hình đăng nhập</p>
              </div>
            </button>

          </div>
        </div>
      </div>
    </div>
  </Transition>

  <!-- ── Recovery Key Modal (after vault creation) ── -->
  <Transition enter-from-class="opacity-0" enter-active-class="transition-opacity duration-200"
              leave-to-class="opacity-0" leave-active-class="transition-opacity duration-200">
    <div v-if="recoveryKeyModal" class="fixed inset-0 z-50 flex items-center justify-center p-4 bg-black/70 backdrop-blur-sm">
      <div class="w-full max-w-lg bg-vault-surface border border-vault-accent/30 rounded-2xl shadow-2xl overflow-hidden">

        <!-- Header -->
        <div class="flex items-start gap-4 px-6 py-5 bg-vault-accent/10 border-b border-vault-accent/20">
          <div class="w-10 h-10 rounded-xl bg-vault-accent/20 flex items-center justify-center flex-shrink-0 mt-0.5">
            <svg class="w-5 h-5 text-vault-accent" fill="none" stroke="currentColor" viewBox="0 0 24 24">
              <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M15 7a2 2 0 012 2m4 0a6 6 0 01-7.743 5.743L11 17H9v2H7v2H4a1 1 0 01-1-1v-2.586a1 1 0 01.293-.707l5.964-5.964A6 6 0 1121 9z" />
            </svg>
          </div>
          <div>
            <h2 class="text-base font-bold text-vault-accent">Recovery Key</h2>
            <p class="text-xs text-vault-text-secondary mt-1 leading-relaxed">
              Vault đã được tạo thành công! Hãy lưu Recovery Key bên dưới. Đây là cách duy nhất để khôi phục mật khẩu nếu bạn quên.
            </p>
          </div>
        </div>

        <!-- Body -->
        <div class="px-6 py-5 space-y-4">
          <!-- Recovery Key Display -->
          <div class="relative">
            <div class="bg-vault-card border border-vault-border rounded-xl px-4 py-4 text-center">
              <p class="font-mono text-lg font-bold text-vault-text tracking-[0.15em] select-all">{{ recoveryKey }}</p>
            </div>
            <button
              @click="copyRecoveryKey"
              class="absolute top-2 right-2 p-2 rounded-lg bg-vault-surface/80 hover:bg-vault-accent/20 text-vault-text-secondary hover:text-vault-accent transition-all"
              title="Copy"
            >
              <svg v-if="recoveryKeyCopied" class="w-4 h-4 text-vault-success" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M5 13l4 4L19 7" />
              </svg>
              <svg v-else class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M8 16H6a2 2 0 01-2-2V6a2 2 0 012-2h8a2 2 0 012 2v2m-6 12h8a2 2 0 002-2v-8a2 2 0 00-2-2h-8a2 2 0 00-2 2v8a2 2 0 002 2z" />
              </svg>
            </button>
          </div>

          <!-- Warning -->
          <div class="bg-vault-warning/10 border border-vault-warning/20 rounded-xl px-4 py-3">
            <div class="flex items-start gap-2">
              <svg class="w-4 h-4 text-vault-warning flex-shrink-0 mt-0.5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M12 9v2m0 4h.01M10.29 3.86L1.82 18a2 2 0 001.71 3h16.94a2 2 0 001.71-3L13.71 3.86a2 2 0 00-3.42 0z"/>
              </svg>
              <div class="text-xs text-vault-warning leading-relaxed">
                <p class="font-semibold mb-1">Lưu ý quan trọng:</p>
                <ul class="list-disc list-inside space-y-0.5 text-vault-text-secondary">
                  <li>Hãy lưu Recovery Key ở nơi an toàn</li>
                  <li>Recovery Key sẽ <strong class="text-vault-warning">không được hiển thị lại</strong></li>
                  <li>Nếu mất cả mật khẩu lẫn Recovery Key, dữ liệu sẽ không thể khôi phục</li>
                </ul>
              </div>
            </div>
          </div>

          <!-- Continue Button -->
          <button
            @click="handleRecoveryKeyDismiss"
            class="w-full py-3 bg-gradient-to-r from-vault-accent to-purple-500 text-white font-semibold rounded-xl hover:from-vault-accent-hover hover:to-purple-400 transition-all duration-200 shadow-lg shadow-vault-accent/20 active:scale-[0.98]"
          >
            Tôi đã lưu Recovery Key
          </button>
        </div>
      </div>
    </div>
  </Transition>

  <!-- New Recovery Key Modal (after password reset) -->
  <Transition enter-from-class="opacity-0" enter-active-class="transition-opacity duration-200"
              leave-to-class="opacity-0" leave-active-class="transition-opacity duration-200">
    <div v-if="newRecoveryKeyModal" class="fixed inset-0 z-50 flex items-center justify-center p-4 bg-black/70 backdrop-blur-sm">
      <div class="w-full max-w-lg bg-vault-surface border border-vault-success/30 rounded-2xl shadow-2xl overflow-hidden">

        <!-- Header -->
        <div class="flex items-start gap-4 px-6 py-5 bg-vault-success/10 border-b border-vault-success/20">
          <div class="w-10 h-10 rounded-xl bg-vault-success/20 flex items-center justify-center flex-shrink-0 mt-0.5">
            <svg class="w-5 h-5 text-vault-success" fill="none" stroke="currentColor" viewBox="0 0 24 24">
              <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M9 12l2 2 4-4m6 2a9 9 0 11-18 0 9 9 0 0118 0z" />
            </svg>
          </div>
          <div>
            <h2 class="text-base font-bold text-vault-success">Đặt lại mật khẩu thành công!</h2>
            <p class="text-xs text-vault-text-secondary mt-1 leading-relaxed">
              Mật khẩu đã được thay đổi. Một Recovery Key mới đã được tạo. Hãy lưu lại.
            </p>
          </div>
        </div>

        <!-- Body -->
        <div class="px-6 py-5 space-y-4">
          <!-- New Recovery Key Display -->
          <div class="relative">
            <div class="bg-vault-card border border-vault-border rounded-xl px-4 py-4 text-center">
              <p class="font-mono text-lg font-bold text-vault-text tracking-[0.15em] select-all">{{ newRecoveryKey }}</p>
            </div>
            <button
              @click="copyNewRecoveryKey"
              class="absolute top-2 right-2 p-2 rounded-lg bg-vault-surface/80 hover:bg-vault-success/20 text-vault-text-secondary hover:text-vault-success transition-all"
              title="Copy"
            >
              <svg v-if="newRecoveryKeyCopied" class="w-4 h-4 text-vault-success" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M5 13l4 4L19 7" />
              </svg>
              <svg v-else class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M8 16H6a2 2 0 01-2-2V6a2 2 0 012-2h8a2 2 0 012 2v2m-6 12h8a2 2 0 002-2v-8a2 2 0 00-2-2h-8a2 2 0 00-2 2v8a2 2 0 002 2z" />
              </svg>
            </button>
          </div>

          <!-- Warning -->
          <div class="bg-vault-warning/10 border border-vault-warning/20 rounded-xl px-4 py-3">
            <div class="flex items-start gap-2">
              <svg class="w-4 h-4 text-vault-warning flex-shrink-0 mt-0.5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M12 9v2m0 4h.01M10.29 3.86L1.82 18a2 2 0 001.71 3h16.94a2 2 0 001.71-3L13.71 3.86a2 2 0 00-3.42 0z"/>
              </svg>
              <div class="text-xs text-vault-warning leading-relaxed">
                <p class="font-semibold">Recovery Key cũ đã bị vô hiệu hoá. Hãy lưu key mới này ở nơi an toàn.</p>
              </div>
            </div>
          </div>

          <!-- Continue Button -->
          <button
            @click="handleNewRecoveryKeyDismiss"
            class="w-full py-3 bg-gradient-to-r from-vault-success to-emerald-400 text-white font-semibold rounded-xl hover:from-vault-success hover:to-emerald-300 transition-all duration-200 shadow-lg shadow-vault-success/20 active:scale-[0.98]"
          >
            Tôi đã lưu Recovery Key mới
          </button>
        </div>
      </div>
    </div>
  </Transition>

</template>
