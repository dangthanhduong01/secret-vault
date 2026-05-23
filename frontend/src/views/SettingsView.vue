<script lang="ts" setup>
import { ref, onMounted } from 'vue'
import { useRouter } from 'vue-router'
import {
  GetVaultInfo, SetupTOTP, EnableTOTP, DisableTOTP,
  ChangePassword, ExportBackup, ImportBackup, IsUnlocked
} from '../../wailsjs/go/app/App'

const router = useRouter()

const vaultInfo = ref<any>(null)
const loading = ref(false)
const message = ref('')
const messageType = ref<'success' | 'error'>('success')

// TOTP Setup
const showTotpSetup = ref(false)
const totpSecret = ref('')
const totpQR = ref('')
const totpVerifyCode = ref('')

// Change Password
const showChangePassword = ref(false)
const oldPassword = ref('')
const newPassword = ref('')
const confirmNewPassword = ref('')

// Backup
const showBackupForm = ref<'export' | 'import' | null>(null)
const backupRecoveryKey = ref('')

onMounted(async () => {
  const unlocked = await IsUnlocked()
  if (!unlocked.success || !unlocked.data) {
    router.push('/')
    return
  }
  await loadInfo()
})

async function loadInfo() {
  const res = await GetVaultInfo()
  if (res.success) {
    vaultInfo.value = res.data
  }
}

function showMessage(msg: string, type: 'success' | 'error') {
  message.value = msg
  messageType.value = type
  setTimeout(() => { message.value = '' }, 4000)
}

// TOTP
async function startTotpSetup() {
  loading.value = true
  const res = await SetupTOTP()
  if (res.success) {
    totpSecret.value = (res.data as any).secret
    totpQR.value = (res.data as any).qr
    showTotpSetup.value = true
  } else {
    showMessage(res.error || 'Failed', 'error')
  }
  loading.value = false
}

async function confirmEnableTOTP() {
  if (totpVerifyCode.value.length !== 6) {
    showMessage('Please enter a 6-digit code', 'error')
    return
  }
  loading.value = true
  const res = await EnableTOTP(totpSecret.value, totpVerifyCode.value)
  if (res.success) {
    showMessage('Two-factor authentication enabled!', 'success')
    showTotpSetup.value = false
    totpVerifyCode.value = ''
    await loadInfo()
  } else {
    showMessage(res.error || 'Invalid code', 'error')
  }
  loading.value = false
}

async function handleDisableTOTP() {
  if (!confirm('Disable two-factor authentication?')) return
  loading.value = true
  const res = await DisableTOTP()
  if (res.success) {
    showMessage('Two-factor authentication disabled', 'success')
    await loadInfo()
  } else {
    showMessage(res.error || 'Failed', 'error')
  }
  loading.value = false
}

// Change Password
async function handleChangePassword() {
  if (newPassword.value.length < 8) {
    showMessage('New password must be at least 8 characters', 'error')
    return
  }
  if (newPassword.value !== confirmNewPassword.value) {
    showMessage('Passwords do not match', 'error')
    return
  }
  loading.value = true
  const res = await ChangePassword(oldPassword.value, newPassword.value)
  if (res.success) {
    showMessage('Password changed successfully!', 'success')
    showChangePassword.value = false
    oldPassword.value = ''
    newPassword.value = ''
    confirmNewPassword.value = ''
  } else {
    showMessage(res.error || 'Failed', 'error')
  }
  loading.value = false
}

// Backup
function startExport() {
  backupRecoveryKey.value = ''
  showBackupForm.value = 'export'
}

function startImport() {
  backupRecoveryKey.value = ''
  showBackupForm.value = 'import'
}

function cancelBackup() {
  showBackupForm.value = null
  backupRecoveryKey.value = ''
}

async function handleExport() {
  if (!backupRecoveryKey.value.trim()) {
    showMessage('Please enter your Recovery Key', 'error')
    return
  }
  loading.value = true
  const res = await ExportBackup(backupRecoveryKey.value.trim())
  if (res.success) {
    const d = res.data as any
    showMessage(`Backup exported: ${d.notes} notes, ${d.files} files`, 'success')
    cancelBackup()
  } else {
    showMessage(res.error || 'Export failed', 'error')
  }
  loading.value = false
}

async function handleImport() {
  if (!backupRecoveryKey.value.trim()) {
    showMessage('Please enter the Recovery Key used when exporting', 'error')
    return
  }
  loading.value = true
  const res = await ImportBackup(backupRecoveryKey.value.trim())
  if (res.success) {
    const d = res.data as any
    const parts: string[] = []
    if (d.imported_notes > 0) parts.push(`${d.imported_notes} notes`)
    if (d.imported_files > 0) parts.push(`${d.imported_files} files`)
    if (parts.length === 0) {
      showMessage('No new items to import — everything is already in the vault', 'success')
    } else {
      showMessage(`Imported ${parts.join(', ')}`, 'success')
    }
    cancelBackup()
    await loadInfo()
  } else {
    showMessage(res.error || 'Import failed', 'error')
  }
  loading.value = false
}
</script>

<template>
  <div class="h-screen flex flex-col bg-vault-bg">
    <!-- Header -->
    <div class="flex items-center gap-4 px-6 py-4 border-b border-vault-border bg-vault-surface">
      <button @click="router.push('/vault')" class="p-2 rounded-lg hover:bg-vault-card text-vault-text-secondary hover:text-vault-text transition-colors">
        <svg class="w-5 h-5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
          <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M15 19l-7-7 7-7" />
        </svg>
      </button>
      <h1 class="text-xl font-bold text-vault-text">Settings</h1>
    </div>

    <!-- Message Toast -->
    <div v-if="message" :class="['fixed top-4 right-4 z-50 px-5 py-3 rounded-xl shadow-lg text-sm font-medium transition-all', messageType === 'success' ? 'bg-vault-success/90 text-white' : 'bg-vault-danger/90 text-white']">
      {{ message }}
    </div>

    <!-- Content -->
    <div class="flex-1 overflow-y-auto p-6">
      <div class="max-w-2xl mx-auto space-y-6">
        
        <!-- Vault Info Card -->
        <div v-if="vaultInfo" class="bg-vault-surface border border-vault-border rounded-2xl p-6">
          <h2 class="text-lg font-semibold text-vault-text mb-4 flex items-center gap-2">
            <svg class="w-5 h-5 text-vault-accent" fill="none" stroke="currentColor" viewBox="0 0 24 24">
              <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M13 16h-1v-4h-1m1-4h.01M21 12a9 9 0 11-18 0 9 9 0 0118 0z" />
            </svg>
            Vault Information
          </h2>
          <div class="grid grid-cols-3 gap-4">
            <div class="bg-vault-card rounded-xl p-4">
              <div class="text-2xl font-bold text-vault-accent">{{ vaultInfo.notes_count }}</div>
              <div class="text-xs text-vault-text-secondary mt-1">Encrypted Notes</div>
            </div>
            <div class="bg-vault-card rounded-xl p-4">
              <div class="text-2xl font-bold text-vault-accent">{{ vaultInfo.files_count }}</div>
              <div class="text-xs text-vault-text-secondary mt-1">Encrypted Files</div>
            </div>
            <div class="bg-vault-card rounded-xl p-4">
              <div class="text-2xl font-bold text-vault-text">{{ vaultInfo.file_size }}</div>
              <div class="text-xs text-vault-text-secondary mt-1">Vault Size</div>
            </div>
          </div>
          <div class="mt-4 text-xs text-vault-text-secondary bg-vault-card rounded-lg px-3 py-2 font-mono break-all">
            {{ vaultInfo.vault_path }}
          </div>
        </div>

        <!-- Two-Factor Authentication -->
        <div class="bg-vault-surface border border-vault-border rounded-2xl p-6">
          <h2 class="text-lg font-semibold text-vault-text mb-4 flex items-center gap-2">
            <svg class="w-5 h-5 text-vault-accent" fill="none" stroke="currentColor" viewBox="0 0 24 24">
              <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M9 12l2 2 4-4m5.618-4.016A11.955 11.955 0 0112 2.944a11.955 11.955 0 01-8.618 3.04A12.02 12.02 0 003 9c0 5.591 3.824 10.29 9 11.622 5.176-1.332 9-6.03 9-11.622 0-1.042-.133-2.052-.382-3.016z" />
            </svg>
            Two-Factor Authentication (TOTP)
          </h2>

          <div v-if="vaultInfo?.totp_enabled" class="flex items-center justify-between">
            <div class="flex items-center gap-3">
              <div class="w-10 h-10 rounded-full bg-vault-success/10 flex items-center justify-center">
                <svg class="w-5 h-5 text-vault-success" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                  <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M5 13l4 4L19 7" />
                </svg>
              </div>
              <div>
                <div class="text-sm font-medium text-vault-text">Enabled</div>
                <div class="text-xs text-vault-text-secondary">Using authenticator app</div>
              </div>
            </div>
            <button @click="handleDisableTOTP" :disabled="loading" class="px-4 py-2 text-sm text-vault-danger border border-vault-danger/30 rounded-lg hover:bg-vault-danger/10 transition-colors">
              Disable
            </button>
          </div>

          <div v-else-if="!showTotpSetup">
            <p class="text-sm text-vault-text-secondary mb-4">Add an extra layer of security to your vault with an authenticator app like Google Authenticator.</p>
            <button @click="startTotpSetup" :disabled="loading" class="px-4 py-2 bg-vault-accent text-white text-sm font-medium rounded-lg hover:bg-vault-accent-hover transition-colors">
              Setup Two-Factor
            </button>
          </div>

          <!-- TOTP Setup Flow -->
          <div v-if="showTotpSetup" class="mt-4 space-y-4">
            <div class="bg-vault-card rounded-xl p-4 text-center">
              <p class="text-sm text-vault-text-secondary mb-3">Scan this QR code with your authenticator app:</p>
              <img v-if="totpQR" :src="totpQR" alt="QR Code" class="mx-auto rounded-lg" />
              <div class="mt-3">
                <p class="text-xs text-vault-text-secondary mb-1">Or enter this secret manually:</p>
                <code class="text-xs text-vault-accent bg-vault-bg px-3 py-1 rounded select-all">{{ totpSecret }}</code>
              </div>
            </div>
            <div>
              <label class="block text-sm text-vault-text-secondary mb-2">Enter verification code:</label>
              <div class="flex gap-3">
                <input
                  v-model="totpVerifyCode"
                  type="text"
                  maxlength="6"
                  placeholder="000000"
                  class="flex-1 px-4 py-2.5 bg-vault-card border border-vault-border rounded-lg text-vault-text text-center text-xl tracking-[0.3em] placeholder-vault-text-secondary/30 focus:outline-none focus:border-vault-accent transition-colors"
                />
                <button @click="confirmEnableTOTP" :disabled="loading" class="px-5 py-2.5 bg-vault-success text-white text-sm font-medium rounded-lg hover:opacity-90 transition-colors">
                  Verify
                </button>
              </div>
            </div>
            <button @click="showTotpSetup = false" class="text-sm text-vault-text-secondary hover:text-vault-text transition-colors">Cancel</button>
          </div>
        </div>

        <!-- Change Password -->
        <div class="bg-vault-surface border border-vault-border rounded-2xl p-6">
          <h2 class="text-lg font-semibold text-vault-text mb-4 flex items-center gap-2">
            <svg class="w-5 h-5 text-vault-accent" fill="none" stroke="currentColor" viewBox="0 0 24 24">
              <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M15 7a2 2 0 012 2m4 0a6 6 0 01-7.743 5.743L11 17H9v2H7v2H4a1 1 0 01-1-1v-2.586a1 1 0 01.293-.707l5.964-5.964A6 6 0 1121 9z" />
            </svg>
            Change Password
          </h2>

          <div v-if="!showChangePassword">
            <p class="text-sm text-vault-text-secondary mb-4">Change your master password. All notes will be re-encrypted.</p>
            <button @click="showChangePassword = true" class="px-4 py-2 bg-vault-card text-vault-text text-sm font-medium rounded-lg border border-vault-border hover:border-vault-accent transition-colors">
              Change Password
            </button>
          </div>

          <form v-else @submit.prevent="handleChangePassword" class="space-y-3">
            <input
              v-model="oldPassword"
              type="password"
              placeholder="Current password"
              class="w-full px-4 py-2.5 bg-vault-card border border-vault-border rounded-lg text-vault-text text-sm placeholder-vault-text-secondary/50 focus:outline-none focus:border-vault-accent transition-colors"
            />
            <input
              v-model="newPassword"
              type="password"
              placeholder="New password (min 8 characters)"
              class="w-full px-4 py-2.5 bg-vault-card border border-vault-border rounded-lg text-vault-text text-sm placeholder-vault-text-secondary/50 focus:outline-none focus:border-vault-accent transition-colors"
            />
            <input
              v-model="confirmNewPassword"
              type="password"
              placeholder="Confirm new password"
              class="w-full px-4 py-2.5 bg-vault-card border border-vault-border rounded-lg text-vault-text text-sm placeholder-vault-text-secondary/50 focus:outline-none focus:border-vault-accent transition-colors"
            />
            <div class="flex gap-3">
              <button type="submit" :disabled="loading" class="px-4 py-2 bg-vault-accent text-white text-sm font-medium rounded-lg hover:bg-vault-accent-hover transition-colors">
                Update Password
              </button>
              <button type="button" @click="showChangePassword = false" class="px-4 py-2 text-sm text-vault-text-secondary hover:text-vault-text transition-colors">
                Cancel
              </button>
            </div>
          </form>
        </div>

        <!-- Backup & Restore -->
        <div class="bg-vault-surface border border-vault-border rounded-2xl p-6">
          <h2 class="text-lg font-semibold text-vault-text mb-4 flex items-center gap-2">
            <svg class="w-5 h-5 text-vault-accent" fill="none" stroke="currentColor" viewBox="0 0 24 24">
              <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M7 16a4 4 0 01-.88-7.903A5 5 0 1115.9 6L16 6a5 5 0 011 9.9M9 19l3 3m0 0l3-3m-3 3V10" />
            </svg>
            Backup & Restore
          </h2>
          <p class="text-sm text-vault-text-secondary mb-4">Export a full encrypted backup of your vault — notes, files, and settings. Backups are encrypted with your Recovery Key and can be imported into any vault.</p>

          <!-- Default state: two buttons -->
          <div v-if="!showBackupForm" class="flex gap-3">
            <button @click="startExport" :disabled="loading" class="px-4 py-2 bg-vault-accent text-white text-sm font-medium rounded-lg hover:bg-vault-accent-hover transition-colors flex items-center gap-2">
              <svg class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M4 16v1a3 3 0 003 3h10a3 3 0 003-3v-1m-4-8l-4-4m0 0L8 8m4-4v12" />
              </svg>
              Export Backup
            </button>
            <button @click="startImport" :disabled="loading" class="px-4 py-2 bg-vault-card text-vault-text text-sm font-medium rounded-lg border border-vault-border hover:border-vault-accent transition-colors flex items-center gap-2">
              <svg class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M4 16v1a3 3 0 003 3h10a3 3 0 003-3v-1m-4-4l-4 4m0 0l-4-4m4 4V4" />
              </svg>
              Import Backup
            </button>
          </div>

          <!-- Recovery Key form for Export -->
          <div v-if="showBackupForm === 'export'" class="space-y-3">
            <div class="bg-vault-card rounded-xl p-4">
              <div class="flex items-center gap-2 mb-2">
                <svg class="w-4 h-4 text-vault-accent" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                  <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M15 7a2 2 0 012 2m4 0a6 6 0 01-7.743 5.743L11 17H9v2H7v2H4a1 1 0 01-1-1v-2.586a1 1 0 01.293-.707l5.964-5.964A6 6 0 1121 9z" />
                </svg>
                <span class="text-sm font-medium text-vault-text">Enter Recovery Key to export</span>
              </div>
              <p class="text-xs text-vault-text-secondary mb-3">The backup will be encrypted with a key derived from your Recovery Key. You'll need this same key to import.</p>
              <input
                v-model="backupRecoveryKey"
                type="text"
                placeholder="XXXX-XXXX-XXXX-XXXX-XXXX-XXXX"
                class="w-full px-4 py-2.5 bg-vault-bg border border-vault-border rounded-lg text-vault-text text-sm font-mono tracking-wider placeholder-vault-text-secondary/30 focus:outline-none focus:border-vault-accent transition-colors uppercase"
              />
            </div>
            <div class="flex gap-3">
              <button @click="handleExport" :disabled="loading || !backupRecoveryKey.trim()" class="px-4 py-2 bg-vault-accent text-white text-sm font-medium rounded-lg hover:bg-vault-accent-hover transition-colors disabled:opacity-50 flex items-center gap-2">
                <svg v-if="loading" class="w-4 h-4 animate-spin" fill="none" viewBox="0 0 24 24"><circle class="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" stroke-width="4"/><path class="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4z"/></svg>
                <svg v-else class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M4 16v1a3 3 0 003 3h10a3 3 0 003-3v-1m-4-8l-4-4m0 0L8 8m4-4v12" /></svg>
                Export
              </button>
              <button @click="cancelBackup" class="px-4 py-2 text-sm text-vault-text-secondary hover:text-vault-text transition-colors">Cancel</button>
            </div>
          </div>

          <!-- Recovery Key form for Import -->
          <div v-if="showBackupForm === 'import'" class="space-y-3">
            <div class="bg-vault-card rounded-xl p-4">
              <div class="flex items-center gap-2 mb-2">
                <svg class="w-4 h-4 text-vault-accent" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                  <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M15 7a2 2 0 012 2m4 0a6 6 0 01-7.743 5.743L11 17H9v2H7v2H4a1 1 0 01-1-1v-2.586a1 1 0 01.293-.707l5.964-5.964A6 6 0 1121 9z" />
                </svg>
                <span class="text-sm font-medium text-vault-text">Enter Recovery Key to import</span>
              </div>
              <p class="text-xs text-vault-text-secondary mb-3">Enter the Recovery Key that was used when the backup was exported. Duplicates are automatically skipped.</p>
              <input
                v-model="backupRecoveryKey"
                type="text"
                placeholder="XXXX-XXXX-XXXX-XXXX-XXXX-XXXX"
                class="w-full px-4 py-2.5 bg-vault-bg border border-vault-border rounded-lg text-vault-text text-sm font-mono tracking-wider placeholder-vault-text-secondary/30 focus:outline-none focus:border-vault-accent transition-colors uppercase"
              />
            </div>
            <div class="flex gap-3">
              <button @click="handleImport" :disabled="loading || !backupRecoveryKey.trim()" class="px-4 py-2 bg-vault-accent text-white text-sm font-medium rounded-lg hover:bg-vault-accent-hover transition-colors disabled:opacity-50 flex items-center gap-2">
                <svg v-if="loading" class="w-4 h-4 animate-spin" fill="none" viewBox="0 0 24 24"><circle class="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" stroke-width="4"/><path class="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4z"/></svg>
                <svg v-else class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M4 16v1a3 3 0 003 3h10a3 3 0 003-3v-1m-4-4l-4 4m0 0l-4-4m4 4V4" /></svg>
                Import
              </button>
              <button @click="cancelBackup" class="px-4 py-2 text-sm text-vault-text-secondary hover:text-vault-text transition-colors">Cancel</button>
            </div>
          </div>
        </div>

        <!-- Security Info -->
        <div class="bg-vault-surface border border-vault-border rounded-2xl p-6">
          <h2 class="text-lg font-semibold text-vault-text mb-4 flex items-center gap-2">
            <svg class="w-5 h-5 text-vault-accent" fill="none" stroke="currentColor" viewBox="0 0 24 24">
              <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M12 15v2m-6 4h12a2 2 0 002-2v-6a2 2 0 00-2-2H6a2 2 0 00-2 2v6a2 2 0 002 2zm10-10V7a4 4 0 00-8 0v4h8z" />
            </svg>
            Security Details
          </h2>
          <div class="space-y-3">
            <div class="flex items-center gap-3 text-sm">
              <div class="w-2 h-2 rounded-full bg-vault-success"></div>
              <span class="text-vault-text-secondary">Encryption:</span>
              <span class="text-vault-text font-medium">AES-256-GCM</span>
            </div>
            <div class="flex items-center gap-3 text-sm">
              <div class="w-2 h-2 rounded-full bg-vault-success"></div>
              <span class="text-vault-text-secondary">Key Derivation:</span>
              <span class="text-vault-text font-medium">PBKDF2 (600,000 iterations)</span>
            </div>
            <div class="flex items-center gap-3 text-sm">
              <div class="w-2 h-2 rounded-full bg-vault-success"></div>
              <span class="text-vault-text-secondary">Digital Signature:</span>
              <span class="text-vault-text font-medium">ECDSA P-256</span>
            </div>
            <div class="flex items-center gap-3 text-sm">
              <div class="w-2 h-2 rounded-full bg-vault-success"></div>
              <span class="text-vault-text-secondary">2FA:</span>
              <span class="text-vault-text font-medium">TOTP (HMAC-SHA1, RFC 6238)</span>
            </div>
            <div class="flex items-center gap-3 text-sm">
              <div class="w-2 h-2 rounded-full bg-vault-success"></div>
              <span class="text-vault-text-secondary">Steganography:</span>
              <span class="text-vault-text font-medium">LSB (Least Significant Bit)</span>
            </div>
          </div>
        </div>

      </div>
    </div>
  </div>
</template>
