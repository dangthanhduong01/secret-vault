<script lang="ts" setup>
import { ref, onMounted, computed, watch, nextTick } from 'vue'
import { useRouter } from 'vue-router'
import { marked } from 'marked'
import {
  GetNotes, AddNote, UpdateNote, DeleteNote,
  LockVault, IsUnlocked, IsReadOnly, SearchNotes,
  GetFiles, AddFile, ExportFile, ExportFileForced, GetFileTamperDetails,
  DeleteFile, VerifyFile, ImportBackup
} from '../../wailsjs/go/app/App'

const router = useRouter()

// ──────────────── Types ─────────────────
interface Note {
  id: string; title: string; content: string
  tags: string[]; created_at: string; updated_at: string
}
interface FileMeta {
  id: string; original_name: string; mime_type: string
  size: number; size_human: string; content_hash: string
  created_at: string; updated_at: string; tampered: boolean
}

// ──────────────── State ─────────────────
const activeTab     = ref<'notes' | 'files'>('notes')
const notes         = ref<Note[]>([])
const files         = ref<FileMeta[]>([])
const selectedNote  = ref<Note | null>(null)
const searchQuery   = ref('')
const searchResults = ref<Note[] | null>(null)
const isSearching   = ref(false)
const editTitle     = ref('')
const editContent   = ref('')
const editTags      = ref('')
const isNewNote     = ref(false)
const showEditor    = ref(false)
const previewMode   = ref(false)
const saveDirty     = ref(false)
const sidebarCollapsed = ref(false)
const toast         = ref('')
const toastType     = ref<'success' | 'error' | 'warn'>('success')
const tamperWarning  = ref<FileMeta | null>(null)
const tamperDetails  = ref<Array<{field:string; reason:string; stored?:string; expected?:string}>>([])
const tamperTab      = ref<'detail'|'options'>('detail')
const tamperLoading  = ref<'export'|'restore'|null>(null)
const tamperRecoveryKey = ref('')
const showTamperRecoveryInput = ref(false)
const loading        = ref(false)
const isReadOnly     = ref(false)
const showFileInfo   = ref(false)

// ──────────────── Lifecycle ─────────────
onMounted(async () => {
  const r = await IsUnlocked()
  if (!r.success || !r.data) { router.push('/'); return }
  const ro = await IsReadOnly()
  if (ro.success) isReadOnly.value = ro.data as boolean
  await loadNotes()
  await loadFiles()
})

// ──────────────── Load ───────────────────
async function loadNotes() {
  const r = await GetNotes()
  if (r.success) notes.value = (r.data as Note[]) ?? []
}
async function loadFiles() {
  const r = await GetFiles()
  if (r.success) files.value = (r.data as FileMeta[]) ?? []
}

// ──────────────── Search ─────────────────
let searchTimer: ReturnType<typeof setTimeout> | null = null
watch(searchQuery, (q) => {
  if (searchTimer) clearTimeout(searchTimer)
  if (!q.trim()) { searchResults.value = null; return }
  isSearching.value = true
  searchTimer = setTimeout(async () => {
    const r = await SearchNotes(q.trim())
    if (r.success) searchResults.value = r.data as Note[]
    isSearching.value = false
  }, 300)
})

const displayedNotes = computed(() =>
  searchResults.value !== null ? searchResults.value : notes.value
)

// ──────────────── Note Editor ─────────────
function selectNote(note: Note) {
  if (saveDirty.value) autoSave()
  selectedNote.value = note
  editTitle.value = note.title
  editContent.value = note.content
  editTags.value = note.tags.join(', ')
  showEditor.value = true
  isNewNote.value = false
  saveDirty.value = false
}

function newNote() {
  if (saveDirty.value) autoSave()
  selectedNote.value = null
  editTitle.value = ''; editContent.value = ''; editTags.value = ''
  showEditor.value = true; isNewNote.value = true; saveDirty.value = false
}

function onContentChange() { saveDirty.value = true }

async function autoSave() {
  if (!saveDirty.value) return
  await saveNote()
}

async function saveNote() {
  const tags = editTags.value.split(',').map(t => t.trim()).filter(Boolean)
  const title = editTitle.value || 'Untitled'
  if (isNewNote.value) {
    const r = await AddNote(title, editContent.value, tags)
    if (r.success) {
      const saved = r.data as Note
      await loadNotes()
      // Update selectedNote ref without overwriting the title/content being edited
      selectedNote.value = saved
      isNewNote.value = false
      saveDirty.value = false
    } else { showToast(r.error || 'Save failed', 'error') }
  } else if (selectedNote.value) {
    const r = await UpdateNote(selectedNote.value.id, title, editContent.value, tags)
    if (r.success) {
      const saved = r.data as Note
      await loadNotes()
      // Preserve what the user is currently typing — only update the id ref
      selectedNote.value = { ...saved, title: editTitle.value, content: editContent.value }
      saveDirty.value = false
    } else { showToast(r.error || 'Save failed', 'error') }
  }
}

async function deleteCurrentNote() {
  if (!selectedNote.value) return
  if (!confirm(`Delete "${selectedNote.value.title}"?`)) return
  const r = await DeleteNote(selectedNote.value.id)
  if (r.success) {
    selectedNote.value = null; showEditor.value = false
    await loadNotes(); showToast('Note deleted', 'success')
  }
}

// ──────────────── Markdown ───────────────
const renderedMarkdown = computed(() => {
  try { return marked.parse(editContent.value || '') as string }
  catch { return '' }
})

// Markdown toolbar helpers
function insertMarkdown(before: string, after = '') {
  const ta = document.getElementById('md-textarea') as HTMLTextAreaElement
  if (!ta) return
  const start = ta.selectionStart, end = ta.selectionEnd
  const selected = editContent.value.substring(start, end)
  const replacement = before + selected + after
  editContent.value = editContent.value.substring(0, start) + replacement + editContent.value.substring(end)
  saveDirty.value = true
  nextTick(() => {
    ta.focus()
    ta.setSelectionRange(start + before.length, start + before.length + selected.length)
  })
}

// ──────────────── Files ──────────────────
async function handleAddFile() {
  loading.value = true
  const r = await AddFile()
  loading.value = false
  if (r.success) {
    await loadFiles()
    showToast('File encrypted & stored', 'success')
  } else if (r.error !== 'No file selected') {
    showToast(r.error || 'Failed', 'error')
  }
}

async function openTamperModal(file: FileMeta) {
  tamperWarning.value = file
  tamperTab.value = 'detail'
  tamperDetails.value = []
  tamperLoading.value = null
  tamperRecoveryKey.value = ''
  showTamperRecoveryInput.value = false
  // Load per-field analysis
  const r = await GetFileTamperDetails(file.id)
  if (r.success) {
    tamperDetails.value = (r.data as any).details ?? []
  }
}

async function handleExportFile(id: string) {
  loading.value = true
  const r = await ExportFile(id)
  loading.value = false
  if (r.success) {
    showToast('✓ File decrypted & exported', 'success')
  } else if (r.error === 'TAMPERED') {
    await openTamperModal(r.data as FileMeta)
  } else if (r.error !== 'No location selected') {
    showToast(r.error || 'Export failed', 'error')
  }
}

async function handleVerifyFile(id: string) {
  loading.value = true
  const r = await VerifyFile(id)
  loading.value = false
  if (r.success) {
    showToast('✓ Signature valid — file is intact', 'success')
  } else if (r.error === 'TAMPERED') {
    await openTamperModal(r.data as FileMeta)
  } else {
    showToast(r.error || 'Verification failed', 'error')
  }
}

async function handleTamperExportForced() {
  if (!tamperWarning.value) return
  tamperLoading.value = 'export'
  const r = await ExportFileForced(tamperWarning.value.id)
  tamperLoading.value = null
  if (r.success) {
    tamperWarning.value = null
    const d = r.data as any
    if (d.corrupted) {
      showToast('⚠ AES decryption failed — raw encrypted data saved as .corrupted', 'error')
    } else {
      showToast('⚠ File exported without signature verification', 'warn')
    }
  } else if (r.error !== 'No location selected') {
    showToast(r.error || 'Export failed', 'error')
  }
}

async function handleTamperRestore() {
  if (!tamperRecoveryKey.value.trim()) {
    showToast('Vui lòng nhập Recovery Key', 'error')
    return
  }
  tamperLoading.value = 'restore'
  const r = await ImportBackup(tamperRecoveryKey.value.trim())
  tamperLoading.value = null
  if (r.success) {
    tamperWarning.value = null
    tamperRecoveryKey.value = ''
    showTamperRecoveryInput.value = false
    await loadNotes()
    await loadFiles()
    showToast('✓ Vault restored from backup', 'success')
  } else if (r.error !== 'No file selected') {
    showToast(r.error || 'Restore failed', 'error')
  }
}

async function handleDeleteFile(id: string, name: string) {
  if (!confirm(`Delete "${name}" from vault?`)) return
  const r = await DeleteFile(id)
  if (r.success) { await loadFiles(); showToast('File deleted', 'success') }
  else showToast(r.error || 'Failed', 'error')
}

// Drag-drop
function onDragOver(e: DragEvent) { e.preventDefault() }
async function onDrop(e: DragEvent) {
  e.preventDefault()
  const file = e.dataTransfer?.files[0]
  if (!file) return
  // Wails doesn't support direct drop paths on Linux well; trigger picker
  await handleAddFile()
}

// ──────────────── Helpers ────────────────
function showToast(msg: string, type: 'success' | 'error' | 'warn') {
  toast.value = msg; toastType.value = type
  setTimeout(() => { toast.value = '' }, 3500)
}
async function lockAndExit() { await LockVault(); router.push('/') }
function formatDate(d: string) {
  return new Date(d).toLocaleDateString('en-US', { month:'short', day:'numeric', year:'numeric' })
}
function fileIcon(mime: string) {
  if (mime.startsWith('image/')) return '🖼️'
  if (mime === 'application/pdf') return '📄'
  if (mime.includes('word')) return '📝'
  if (mime.includes('spreadsheet') || mime.includes('excel')) return '📊'
  if (mime === 'application/zip') return '🗜️'
  return '📁'
}
function getPreview(c: string) { return c.replace(/[#*`>\-_]/g, '').substring(0, 70) + '…' }
</script>

<template>
  <div class="h-screen flex bg-vault-bg overflow-hidden">

    <!-- ────────── Toast ────────── -->
    <Transition enter-from-class="opacity-0 translate-y-2" enter-active-class="transition-all duration-200" leave-to-class="opacity-0 translate-y-2" leave-active-class="transition-all duration-200">
      <div v-if="toast" :class="['fixed top-4 right-4 z-50 px-5 py-3 rounded-xl shadow-xl text-sm font-medium max-w-xs',
        toastType==='success' ? 'bg-vault-success/90 text-white' :
        toastType==='warn'    ? 'bg-vault-warning/90 text-black' :
                                'bg-vault-danger/90 text-white']">
        {{ toast }}
      </div>
    </Transition>

    <!-- ────────── File Tamper Modal ────────── -->
    <Transition enter-from-class="opacity-0" enter-active-class="transition-opacity duration-200"
                leave-to-class="opacity-0" leave-active-class="transition-opacity duration-200">
      <div v-if="tamperWarning" class="fixed inset-0 z-50 flex items-center justify-center bg-black/65 backdrop-blur-sm p-4">
        <div class="bg-vault-surface border border-vault-danger/40 rounded-2xl shadow-2xl w-full max-w-lg overflow-hidden">

          <!-- Header -->
          <div class="flex items-start gap-4 px-6 py-5 bg-vault-danger/10 border-b border-vault-danger/20">
            <div class="w-10 h-10 rounded-xl bg-vault-danger/20 flex items-center justify-center flex-shrink-0 mt-0.5">
              <svg class="w-5 h-5 text-vault-danger" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2"
                  d="M12 9v2m0 4h.01M10.29 3.86L1.82 18a2 2 0 001.71 3h16.94a2 2 0 001.71-3L13.71 3.86a2 2 0 00-3.42 0z"/>
              </svg>
            </div>
            <div class="flex-1 min-w-0">
              <h3 class="text-sm font-bold text-vault-danger">File Integrity Violation</h3>
              <p class="text-xs text-vault-text-secondary mt-0.5 truncate">
                {{ tamperWarning.original_name }}
              </p>
            </div>
            <button @click="tamperWarning=null" :disabled="tamperLoading!==null"
              class="p-1.5 text-vault-text-secondary hover:text-vault-text rounded-lg hover:bg-vault-card transition-colors disabled:opacity-40">
              <svg class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M6 18L18 6M6 6l12 12"/>
              </svg>
            </button>
          </div>

          <!-- Tab bar -->
          <div class="flex border-b border-vault-border/50">
            <button @click="tamperTab='detail'"
              :class="['px-5 py-2.5 text-xs font-medium border-b-2 transition-colors',
                tamperTab==='detail' ? 'text-vault-danger border-vault-danger' : 'text-vault-text-secondary border-transparent hover:text-vault-text']">
              🔍 Chi tiết lỗi
            </button>
            <button @click="tamperTab='options'"
              :class="['px-5 py-2.5 text-xs font-medium border-b-2 transition-colors',
                tamperTab==='options' ? 'text-vault-accent border-vault-accent' : 'text-vault-text-secondary border-transparent hover:text-vault-text']">
              🛠 Tuỳ chọn khắc phục
            </button>
          </div>

          <!-- Detail tab -->
          <div v-if="tamperTab==='detail'" class="px-6 py-4 space-y-3 max-h-80 overflow-y-auto">
            <p class="text-xs text-vault-text-secondary">
              Chữ ký ECDSA P-256 không khớp với metadata. Các trường sau đây có thể đã bị thay đổi:
            </p>
            <div v-if="tamperDetails.length===0" class="flex items-center gap-2 text-xs text-vault-text-secondary">
              <svg class="w-3.5 h-3.5 animate-spin text-vault-accent" fill="none" viewBox="0 0 24 24">
                <circle class="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" stroke-width="4"/>
                <path class="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4z"/>
              </svg>
              Đang phân tích…
            </div>
            <div v-for="d in tamperDetails" :key="d.field"
              class="bg-vault-card/60 border border-vault-border rounded-xl p-3 space-y-1">
              <div class="flex items-center gap-2">
                <span class="text-[10px] font-mono font-bold text-vault-danger bg-vault-danger/10 px-2 py-0.5 rounded">
                  {{ d.field }}
                </span>
              </div>
              <p class="text-xs text-vault-text-secondary">{{ d.reason }}</p>
              <div v-if="d.stored" class="text-[10px] font-mono text-vault-text-secondary/70 break-all">
                <span class="text-vault-text-secondary/40">stored: </span>{{ d.stored }}
              </div>
              <div v-if="d.expected" class="text-[10px] font-mono text-vault-text-secondary/70 break-all">
                <span class="text-vault-text-secondary/40">actual: </span>
                <span class="text-vault-danger/80">{{ d.expected }}</span>
              </div>
            </div>
            <div class="text-[10px] text-vault-text-secondary/50 font-mono break-all border-t border-vault-border/30 pt-2">
              SHA-256: {{ tamperWarning.content_hash }}
            </div>
          </div>

          <!-- Options tab -->
          <div v-if="tamperTab==='options'" class="px-6 py-4 space-y-2.5">

            <!-- Option 1: Restore from backup -->
            <div class="w-full rounded-xl border border-vault-accent/30 bg-vault-accent/5 overflow-hidden">
              <button @click="showTamperRecoveryInput=!showTamperRecoveryInput" :disabled="tamperLoading!==null"
                class="w-full flex items-center gap-4 px-4 py-3.5 hover:bg-vault-accent/10 transition-all disabled:opacity-50 disabled:cursor-not-allowed group">
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
                <div class="text-left flex-1">
                  <p class="text-sm font-semibold text-vault-text">Khôi phục từ Backup</p>
                  <p class="text-[11px] text-vault-text-secondary mt-0.5">
                    Nhập Recovery Key và chọn file backup để phục hồi toàn bộ dữ liệu an toàn
                  </p>
                </div>
                <svg :class="['w-4 h-4 text-vault-text-secondary transition-transform', showTamperRecoveryInput ? 'rotate-180' : '']" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                  <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M19 9l-7 7-7-7"/>
                </svg>
              </button>
              <div v-if="showTamperRecoveryInput" class="px-4 pb-4 space-y-3 border-t border-vault-accent/20">
                <label class="block text-xs font-medium text-vault-text-secondary mt-3">Recovery Key</label>
                <input v-model="tamperRecoveryKey" type="text" placeholder="XXXX-XXXX-XXXX-XXXX-XXXX-XXXX"
                  class="w-full px-3 py-2 bg-vault-card border border-vault-border rounded-lg text-sm text-vault-text font-mono placeholder-vault-text-secondary/40 focus:outline-none focus:border-vault-accent transition-colors tracking-wider"/>
                <button @click="handleTamperRestore" :disabled="tamperLoading!==null || !tamperRecoveryKey.trim()"
                  class="w-full py-2 bg-vault-accent text-white text-sm font-medium rounded-lg hover:bg-vault-accent-hover transition-colors disabled:opacity-50 disabled:cursor-not-allowed">
                  {{ tamperLoading==='restore' ? 'Đang khôi phục…' : 'Chọn file backup & Khôi phục' }}
                </button>
              </div>
            </div>

            <!-- Option 2: Export forced -->
            <button @click="handleTamperExportForced" :disabled="tamperLoading!==null"
              class="w-full flex items-center gap-4 px-4 py-3.5 rounded-xl border border-vault-warning/30 bg-vault-warning/5 hover:bg-vault-warning/10 transition-all disabled:opacity-50 disabled:cursor-not-allowed group">
              <div class="w-9 h-9 rounded-lg bg-vault-warning/15 group-hover:bg-vault-warning/25 flex items-center justify-center flex-shrink-0 transition-colors">
                <svg v-if="tamperLoading==='export'" class="w-4 h-4 text-vault-warning animate-spin" fill="none" viewBox="0 0 24 24">
                  <circle class="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" stroke-width="4"/>
                  <path class="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4z"/>
                </svg>
                <svg v-else class="w-4 h-4 text-vault-warning" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                  <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2"
                    d="M4 16v1a3 3 0 003 3h10a3 3 0 003-3v-1m-4-4l-4 4m0 0l-4-4m4 4V4"/>
                </svg>
              </div>
              <div class="text-left">
                <div class="flex items-center gap-2">
                  <p class="text-sm font-semibold text-vault-text">Xuất file dù có rủi ro</p>
                  <span class="text-[9px] font-bold uppercase tracking-wide bg-vault-warning/20 text-vault-warning px-1.5 py-0.5 rounded-full">Rủi ro</span>
                </div>
                <p class="text-[11px] text-vault-text-secondary mt-0.5">
                  Bỏ qua xác minh chữ ký, giải mã và xuất file — nội dung có thể đã bị thay đổi
                </p>
              </div>
            </button>

            <!-- Option 3: Close -->
            <button @click="tamperWarning=null" :disabled="tamperLoading!==null"
              class="w-full flex items-center gap-4 px-4 py-3.5 rounded-xl border border-vault-border hover:bg-vault-card/50 transition-all disabled:opacity-50 group">
              <div class="w-9 h-9 rounded-lg bg-vault-card group-hover:bg-vault-border/40 flex items-center justify-center flex-shrink-0 transition-colors">
                <svg class="w-4 h-4 text-vault-text-secondary" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                  <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M6 18L18 6M6 6l12 12"/>
                </svg>
              </div>
              <div class="text-left">
                <p class="text-sm font-semibold text-vault-text">Đóng</p>
                <p class="text-[11px] text-vault-text-secondary mt-0.5">Không làm gì, quay lại danh sách file</p>
              </div>
            </button>
          </div>

        </div>
      </div>
    </Transition>

    <!-- ────────── Sidebar ────────── -->
    <div :class="['flex flex-col border-r border-vault-border bg-vault-surface transition-all duration-300 flex-shrink-0', sidebarCollapsed ? 'w-16' : 'w-72']">
      <!-- Header -->
      <div class="h-14 px-3 flex items-center gap-2 border-b border-vault-border">
        <button @click="sidebarCollapsed=!sidebarCollapsed" class="p-2 rounded-lg hover:bg-vault-card text-vault-text-secondary hover:text-vault-text transition-colors">
          <svg class="w-5 h-5" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M4 6h16M4 12h16M4 18h16"/></svg>
        </button>
        <span v-if="!sidebarCollapsed" class="font-bold text-vault-text text-lg tracking-tight flex-1">Secret Vault</span>
      </div>

      <!-- Tabs -->
      <div v-if="!sidebarCollapsed" class="flex gap-1 p-2 border-b border-vault-border">
        <button @click="activeTab='notes'" :class="['flex-1 py-1.5 text-xs font-medium rounded-lg transition-colors', activeTab==='notes' ? 'bg-vault-accent text-white' : 'text-vault-text-secondary hover:text-vault-text hover:bg-vault-card']">
          📝 Notes ({{ notes.length }})
        </button>
        <button @click="activeTab='files'" :class="['flex-1 py-1.5 text-xs font-medium rounded-lg transition-colors', activeTab==='files' ? 'bg-vault-accent text-white' : 'text-vault-text-secondary hover:text-vault-text hover:bg-vault-card']">
          🔒 Files ({{ files.length }})
        </button>
      </div>

      <!-- Search (notes tab) -->
      <div v-if="!sidebarCollapsed && activeTab==='notes'" class="p-2">
        <div class="flex items-center gap-1.5 px-2.5 py-1.5 bg-vault-card border border-vault-border rounded-lg focus-within:border-vault-accent transition-colors">
          <svg class="w-3.5 h-3.5 text-vault-text-secondary flex-shrink-0" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M21 21l-6-6m2-5a7 7 0 11-14 0 7 7 0 0114 0z"/></svg>
          <input v-model="searchQuery" type="text" placeholder="Search by title..."
            class="flex-1 min-w-0 bg-transparent text-xs text-vault-text placeholder-vault-text-secondary/50 focus:outline-none"/>
          <svg v-if="isSearching" class="w-3.5 h-3.5 text-vault-accent animate-spin flex-shrink-0" fill="none" viewBox="0 0 24 24"><circle class="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" stroke-width="4"/><path class="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4z"/></svg>
          <button v-if="searchQuery && !isSearching" @click="searchQuery=''; searchResults=null" class="flex-shrink-0 text-vault-text-secondary hover:text-vault-text">
            <svg class="w-3.5 h-3.5" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M6 18L18 6M6 6l12 12"/></svg>
          </button>
        </div>
        <p v-if="searchResults !== null" class="text-[10px] text-vault-text-secondary mt-1 px-1">{{ searchResults.length }} result{{ searchResults.length!==1?'s':'' }} for "{{ searchQuery }}"</p>
      </div>

      <!-- Notes List -->
      <div v-if="!sidebarCollapsed && activeTab==='notes'" class="flex-1 overflow-y-auto">
        <button v-for="note in displayedNotes" :key="note.id" @click="selectNote(note)"
          :class="['w-full text-left px-3 py-3 border-b border-vault-border/40 hover:bg-vault-card/60 transition-colors group',
            selectedNote?.id===note.id ? 'bg-vault-card border-l-2 border-l-vault-accent' : '']">
          <div class="font-medium text-vault-text text-xs truncate">{{ note.title || 'Untitled' }}</div>
          <div class="text-[11px] text-vault-text-secondary mt-0.5 truncate opacity-70">{{ getPreview(note.content) }}</div>
          <div class="flex items-center gap-1.5 mt-1.5 flex-wrap">
            <span class="text-[9px] text-vault-text-secondary/60">{{ formatDate(note.updated_at) }}</span>
            <span v-for="tag in note.tags.slice(0,2)" :key="tag" class="text-[9px] px-1.5 py-0.5 bg-vault-accent/10 text-vault-accent rounded-full">{{ tag }}</span>
          </div>
        </button>
        <div v-if="displayedNotes.length===0" class="p-4 text-center text-vault-text-secondary text-xs">
          {{ searchQuery ? 'No notes found' : 'No notes yet. Create one!' }}
        </div>
      </div>

      <!-- Files List -->
      <div v-if="!sidebarCollapsed && activeTab==='files'" class="flex-1 overflow-y-auto" @dragover="onDragOver" @drop="onDrop">
        <div v-if="files.length===0" class="p-4 text-center">
          <p class="text-vault-text-secondary text-xs mb-2">Drop files here or click Add</p>
        </div>
        <div v-for="file in files" :key="file.id"
          :class="['flex items-center gap-2 px-3 py-2.5 border-b border-vault-border/40 hover:bg-vault-card/60 transition-colors group', file.tampered ? 'border-l-2 border-l-vault-danger' : '']">
          <span class="text-lg flex-shrink-0">{{ fileIcon(file.mime_type) }}</span>
          <div class="flex-1 min-w-0">
            <div class="text-xs font-medium text-vault-text truncate">{{ file.original_name }}</div>
            <div class="text-[10px] text-vault-text-secondary">{{ file.size_human }}</div>
          </div>
          <div class="flex gap-1 opacity-0 group-hover:opacity-100 transition-opacity">
            <button @click="handleVerifyFile(file.id)" title="Verify integrity" class="p-1 rounded text-vault-success hover:bg-vault-success/10 transition-colors">
              <svg class="w-3.5 h-3.5" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M9 12l2 2 4-4m6 2a9 9 0 11-18 0 9 9 0 0118 0z"/></svg>
            </button>
            <button @click="handleExportFile(file.id)" title="Decrypt & export" class="p-1 rounded text-vault-accent hover:bg-vault-accent/10 transition-colors">
              <svg class="w-3.5 h-3.5" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M4 16v1a3 3 0 003 3h10a3 3 0 003-3v-1m-4-4l-4 4m0 0l-4-4m4 4V4"/></svg>
            </button>
            <button @click="handleDeleteFile(file.id, file.original_name)" title="Delete" class="p-1 rounded text-vault-danger hover:bg-vault-danger/10 transition-colors">
              <svg class="w-3.5 h-3.5" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M19 7l-.867 12.142A2 2 0 0116.138 21H7.862a2 2 0 01-1.995-1.858L5 7m5 4v6m4-6v6m1-10V4a1 1 0 00-1-1h-4a1 1 0 00-1 1v3M4 7h16"/></svg>
            </button>
          </div>
        </div>
      </div>

      <!-- Bottom Actions -->
      <div class="p-2 border-t border-vault-border space-y-1.5">
        <!-- Add button contextual -->
        <button v-if="activeTab==='notes'" @click="newNote"
          :class="['w-full flex items-center gap-2 py-2 rounded-lg bg-vault-accent text-white text-sm font-medium hover:bg-vault-accent-hover transition-colors', sidebarCollapsed ? 'justify-center px-2' : 'px-3']">
          <svg class="w-4 h-4 flex-shrink-0" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M12 4v16m8-8H4"/></svg>
          <span v-if="!sidebarCollapsed" class="text-xs">New Note</span>
        </button>
        <button v-else @click="handleAddFile" :disabled="loading"
          :class="['w-full flex items-center gap-2 py-2 rounded-lg bg-vault-accent text-white text-sm font-medium hover:bg-vault-accent-hover transition-colors disabled:opacity-50', sidebarCollapsed ? 'justify-center px-2' : 'px-3']">
          <svg class="w-4 h-4 flex-shrink-0" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M12 4v16m8-8H4"/></svg>
          <span v-if="!sidebarCollapsed" class="text-xs">Add File</span>
        </button>

        <!-- Nav row -->
        <div class="flex gap-1">
          <button @click="router.push('/steganography')" :class="['flex-1 flex items-center gap-1 py-1.5 rounded-lg text-vault-text-secondary hover:text-vault-text hover:bg-vault-card transition-colors', sidebarCollapsed ? 'justify-center' : 'px-2']" title="Steganography">
            <svg class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M4 16l4.586-4.586a2 2 0 012.828 0L16 16m-2-2l1.586-1.586a2 2 0 012.828 0L20 14m-6-6h.01M6 20h12a2 2 0 002-2V6a2 2 0 00-2-2H6a2 2 0 00-2 2v12a2 2 0 002 2z"/></svg>
            <span v-if="!sidebarCollapsed" class="text-[10px]">Stego</span>
          </button>
          <button @click="router.push('/settings')" :class="['flex-1 flex items-center gap-1 py-1.5 rounded-lg text-vault-text-secondary hover:text-vault-text hover:bg-vault-card transition-colors', sidebarCollapsed ? 'justify-center' : 'px-2']" title="Settings">
            <svg class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M10.325 4.317c.426-1.756 2.924-1.756 3.35 0a1.724 1.724 0 002.573 1.066c1.543-.94 3.31.826 2.37 2.37a1.724 1.724 0 001.066 2.573c1.756.426 1.756 2.924 0 3.35a1.724 1.724 0 00-1.066 2.573c.94 1.543-.826 3.31-2.37 2.37a1.724 1.724 0 00-2.573 1.066c-.426 1.756-2.924 1.756-3.35 0a1.724 1.724 0 00-2.573-1.066c-1.543.94-3.31-.826-2.37-2.37a1.724 1.724 0 00-1.066-2.573c-1.756-.426-1.756-2.924 0-3.35a1.724 1.724 0 001.066-2.573c-.94-1.543.826-3.31 2.37-2.37.996.608 2.296.07 2.572-1.065z"/><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M15 12a3 3 0 11-6 0 3 3 0 016 0z"/></svg>
            <span v-if="!sidebarCollapsed" class="text-[10px]">Settings</span>
          </button>
          <button @click="lockAndExit" :class="['flex-1 flex items-center gap-1 py-1.5 rounded-lg text-vault-danger hover:bg-vault-danger/10 transition-colors', sidebarCollapsed ? 'justify-center' : 'px-2']" title="Lock">
            <svg class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M12 15v2m-6 4h12a2 2 0 002-2v-6a2 2 0 00-2-2H6a2 2 0 00-2 2v6a2 2 0 002 2zm10-10V7a4 4 0 00-8 0v4h8z"/></svg>
            <span v-if="!sidebarCollapsed" class="text-[10px]">Lock</span>
          </button>
        </div>
      </div>
    </div>

    <!-- ────────── Main Content ────────── -->
    <div class="flex-1 flex flex-col min-w-0">

      <!-- Read-Only Banner -->
      <div v-if="isReadOnly"
        class="flex items-center gap-3 px-5 py-2.5 bg-vault-warning/10 border-b border-vault-warning/30 flex-shrink-0">
        <svg class="w-4 h-4 text-vault-warning flex-shrink-0" fill="none" stroke="currentColor" viewBox="0 0 24 24">
          <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2"
            d="M12 9v2m0 4h.01M10.29 3.86L1.82 18a2 2 0 001.71 3h16.94a2 2 0 001.71-3L13.71 3.86a2 2 0 00-3.42 0z"/>
        </svg>
        <p class="text-xs text-vault-warning flex-1">
          <span class="font-semibold">Chế độ Read-Only</span> — Vault đang mở sau khi phát hiện can thiệp chữ ký. Chỉ có thể xem và xuất dữ liệu, không thể chỉnh sửa hay lưu mới.
        </p>
        <button @click="router.push('/settings')"
          class="text-[11px] text-vault-warning underline underline-offset-2 hover:no-underline flex-shrink-0">
          Khôi phục backup
        </button>
      </div>

      <!-- Note Editor -->
      <template v-if="activeTab==='notes' && showEditor">
        <!-- Editor Header -->
        <div class="h-14 flex items-center gap-2 px-4 border-b border-vault-border bg-vault-surface/60 flex-shrink-0">
          <!-- Editable title — full width, no styling conflict -->
          <input v-model="editTitle" type="text" placeholder="Untitled note…" @input="onContentChange"
            class="flex-1 bg-transparent text-base font-semibold text-vault-text placeholder-vault-text-secondary/30 focus:outline-none min-w-0 caret-vault-accent"/>

          <!-- Right controls group -->
          <div class="flex items-center gap-1.5 flex-shrink-0">

            <!-- Edit / Preview toggle pill -->
            <div class="flex items-center bg-vault-card border border-vault-border rounded-lg gap-px p-0.5">
              <button @click="previewMode=false"
                :class="['flex items-center gap-1.5 px-3 py-1 text-xs font-medium transition-all rounded-md',
                  !previewMode ? 'bg-vault-accent text-white shadow-inner' : 'text-vault-text-secondary hover:text-vault-text hover:bg-white/5']">
                <svg class="w-3 h-3" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M11 5H6a2 2 0 00-2 2v11a2 2 0 002 2h11a2 2 0 002-2v-5m-1.414-9.414a2 2 0 112.828 2.828L11.828 15H9v-2.828l8.586-8.586z"/></svg>
                Edit
              </button>
              <button @click="previewMode=true"
                :class="['flex items-center gap-1.5 px-3 py-1 text-xs font-medium transition-all rounded-md',
                  previewMode ? 'bg-vault-accent text-white shadow-inner' : 'text-vault-text-secondary hover:text-vault-text hover:bg-white/5']">
                <svg class="w-3 h-3" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M15 12a3 3 0 11-6 0 3 3 0 016 0z"/><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M2.458 12C3.732 7.943 7.523 5 12 5c4.478 0 8.268 2.943 9.542 7-1.274 4.057-5.064 7-9.542 7-4.477 0-8.268-2.943-9.542-7z"/></svg>
                Preview
              </button>
            </div>

            <!-- Save button -->
            <button @click="saveNote"
              :class="['flex items-center gap-1.5 px-3.5 py-1.5 text-xs font-semibold rounded-lg transition-all',
                saveDirty
                  ? 'bg-vault-accent text-white hover:bg-vault-accent-hover shadow-lg shadow-vault-accent/20 ring-1 ring-vault-accent/50'
                  : 'bg-vault-card text-vault-text-secondary border border-vault-border cursor-default']">
              <svg v-if="saveDirty" class="w-3.5 h-3.5" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2.5" d="M5 13l4 4L19 7"/></svg>
              <svg v-else class="w-3.5 h-3.5 text-vault-success" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M9 12l2 2 4-4m6 2a9 9 0 11-18 0 9 9 0 0118 0z"/></svg>
              {{ saveDirty ? 'Save' : 'Saved' }}
            </button>

            <!-- Delete button (icon only) -->
            <button v-if="!isNewNote" @click="deleteCurrentNote"
              title="Delete note"
              class="p-1.5 rounded-lg text-vault-text-secondary hover:text-vault-danger hover:bg-vault-danger/10 border border-transparent hover:border-vault-danger/20 transition-all">
              <svg class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M19 7l-.867 12.142A2 2 0 0116.138 21H7.862a2 2 0 01-1.995-1.858L5 7m5 4v6m4-6v6m1-10V4a1 1 0 00-1-1h-4a1 1 0 00-1 1v3M4 7h16"/></svg>
            </button>
          </div>
        </div>

        <!-- Markdown Toolbar (edit mode) -->
        <div v-if="!previewMode" class="flex items-center gap-1 px-5 py-2 border-b border-vault-border/50 bg-vault-surface/30 flex-wrap flex-shrink-0">
          <button @click="insertMarkdown('**','**')" title="Bold" class="px-2 py-1 text-xs font-bold text-vault-text-secondary hover:text-vault-text hover:bg-vault-card rounded transition-colors">B</button>
          <button @click="insertMarkdown('*','*')"   title="Italic" class="px-2 py-1 text-xs italic text-vault-text-secondary hover:text-vault-text hover:bg-vault-card rounded transition-colors">I</button>
          <button @click="insertMarkdown('~~','~~')" title="Strikethrough" class="px-2 py-1 text-xs line-through text-vault-text-secondary hover:text-vault-text hover:bg-vault-card rounded transition-colors">S</button>
          <button @click="insertMarkdown('`','`')"   title="Code" class="px-2 py-1 text-xs font-mono text-vault-text-secondary hover:text-vault-text hover:bg-vault-card rounded transition-colors">&lt;/&gt;</button>
          <div class="w-px h-4 bg-vault-border mx-1"></div>
          <button @click="insertMarkdown('# ')"      title="H1" class="px-2 py-1 text-xs text-vault-text-secondary hover:text-vault-text hover:bg-vault-card rounded transition-colors">H1</button>
          <button @click="insertMarkdown('## ')"     title="H2" class="px-2 py-1 text-xs text-vault-text-secondary hover:text-vault-text hover:bg-vault-card rounded transition-colors">H2</button>
          <button @click="insertMarkdown('### ')"    title="H3" class="px-2 py-1 text-xs text-vault-text-secondary hover:text-vault-text hover:bg-vault-card rounded transition-colors">H3</button>
          <div class="w-px h-4 bg-vault-border mx-1"></div>
          <button @click="insertMarkdown('- ')"      title="List" class="px-2 py-1 text-xs text-vault-text-secondary hover:text-vault-text hover:bg-vault-card rounded transition-colors">• List</button>
          <button @click="insertMarkdown('1. ')"     title="Numbered List" class="px-2 py-1 text-xs text-vault-text-secondary hover:text-vault-text hover:bg-vault-card rounded transition-colors">1. List</button>
          <button @click="insertMarkdown('- [ ] ')"  title="Checkbox" class="px-2 py-1 text-xs text-vault-text-secondary hover:text-vault-text hover:bg-vault-card rounded transition-colors">☐ Task</button>
          <div class="w-px h-4 bg-vault-border mx-1"></div>
          <button @click="insertMarkdown('> ')"      title="Quote" class="px-2 py-1 text-xs text-vault-text-secondary hover:text-vault-text hover:bg-vault-card rounded transition-colors">❝ Quote</button>
          <button @click="insertMarkdown('---\n')"   title="Divider" class="px-2 py-1 text-xs text-vault-text-secondary hover:text-vault-text hover:bg-vault-card rounded transition-colors">─ HR</button>
          <button @click="insertMarkdown('| Col1 | Col2 |\n|------|------|\n| A    | B    |\n')" title="Table" class="px-2 py-1 text-xs text-vault-text-secondary hover:text-vault-text hover:bg-vault-card rounded transition-colors">⊞ Table</button>
          <!-- Tags -->
          <div class="ml-auto flex items-center gap-1.5">
            <svg class="w-3.5 h-3.5 text-vault-text-secondary" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M7 7h.01M7 3h5c.512 0 1.024.195 1.414.586l7 7a2 2 0 010 2.828l-7 7a2 2 0 01-2.828 0l-7-7A1.994 1.994 0 013 12V7a4 4 0 014-4z"/></svg>
            <input v-model="editTags" placeholder="tags, comma separated" @input="onContentChange"
              class="text-xs bg-transparent text-vault-text-secondary placeholder-vault-text-secondary/40 focus:outline-none w-40"/>
          </div>
        </div>

        <!-- Editor / Preview area -->
        <div class="flex-1 overflow-hidden flex flex-col">
          <!-- Edit mode: textarea -->
          <textarea v-if="!previewMode" id="md-textarea" v-model="editContent" @input="onContentChange"
            placeholder="# Note title&#10;&#10;Write your **Markdown** content here…&#10;&#10;- Use the toolbar above to format&#10;- Switch to Preview to see rendered output"
            class="flex-1 w-full px-6 py-4 bg-transparent text-vault-text placeholder-vault-text-secondary/30 focus:outline-none resize-none text-[14px] leading-relaxed font-mono"
            spellcheck="false">
          </textarea>

          <!-- Preview mode: rendered markdown -->
          <div v-else class="flex-1 overflow-y-auto px-8 py-6 prose prose-invert prose-sm max-w-none"
            v-html="renderedMarkdown"
            style="color:var(--color-vault-text);
                   --tw-prose-headings:var(--color-vault-text);
                   --tw-prose-bold:var(--color-vault-text);
                   --tw-prose-code:var(--color-vault-accent);
                   --tw-prose-links:var(--color-vault-accent);">
          </div>
        </div>

        <!-- Status bar -->
        <div class="h-7 px-5 border-t border-vault-border/50 bg-vault-surface/30 flex items-center gap-4 text-[10px] text-vault-text-secondary flex-shrink-0">
          <span v-if="selectedNote">Edited {{ formatDate(selectedNote.updated_at) }}</span>
          <span>{{ editContent.length }} chars · {{ editContent.split(/\s+/).filter(Boolean).length }} words</span>
          <div class="ml-auto flex items-center gap-1">
            <span class="w-1.5 h-1.5 rounded-full bg-vault-success"></span>
            <span>AES-256 Encrypted</span>
          </div>
        </div>
      </template>

      <!-- Notes empty state -->
      <div v-else-if="activeTab==='notes'" class="flex-1 flex items-center justify-center">
        <div class="text-center">
          <div class="w-20 h-20 mx-auto mb-5 rounded-2xl bg-vault-card flex items-center justify-center">
            <svg class="w-10 h-10 text-vault-text-secondary/30" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="1.5" d="M9 12h6m-6 4h6m2 5H7a2 2 0 01-2-2V5a2 2 0 012-2h5.586a1 1 0 01.707.293l5.414 5.414a1 1 0 01.293.707V19a2 2 0 01-2 2z"/></svg>
          </div>
          <h3 class="text-base font-semibold text-vault-text-secondary mb-1">No note selected</h3>
          <p class="text-xs text-vault-text-secondary/60 mb-5">Encrypted with AES-256-GCM, signed with ECDSA</p>
          <button @click="newNote" class="px-5 py-2.5 bg-vault-accent text-white text-sm font-medium rounded-xl hover:bg-vault-accent-hover transition-colors inline-flex items-center gap-2">
            <svg class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M12 4v16m8-8H4"/></svg>
            New Note
          </button>
        </div>
      </div>

      <!-- Files panel (drop zone) -->
      <div v-if="activeTab==='files'" class="flex-1 flex flex-col overflow-hidden">
        <div class="h-14 flex items-center px-6 border-b border-vault-border bg-vault-surface/50 flex-shrink-0 gap-3">
          <h2 class="font-bold text-vault-text">Encrypted Files</h2>
          <span class="text-xs text-vault-text-secondary bg-vault-card px-2 py-1 rounded-full">AES-256-GCM + ECDSA</span>
          <div class="ml-auto flex items-center gap-2">
            <!-- Info button -->
            <div class="relative" @mouseenter="showFileInfo=true" @mouseleave="showFileInfo=false">
              <button
                class="w-7 h-7 flex items-center justify-center rounded-lg border border-vault-border text-vault-text-secondary hover:text-vault-accent hover:border-vault-accent/40 hover:bg-vault-accent/5 transition-all"
                title="Supported file types">
                <svg class="w-3.5 h-3.5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                  <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M13 16h-1v-4h-1m1-4h.01M21 12a9 9 0 11-18 0 9 9 0 0118 0z"/>
                </svg>
              </button>
              <!-- Info popover -->
              <Transition enter-from-class="opacity-0 scale-95" enter-active-class="transition-all duration-150"
                          leave-to-class="opacity-0 scale-95" leave-active-class="transition-all duration-100">
                <div v-if="showFileInfo"
                  class="absolute right-0 top-full mt-2 z-50 w-72 bg-vault-surface border border-vault-border rounded-xl shadow-2xl p-4 space-y-3">
                  <div class="flex items-center justify-between">
                    <h4 class="text-xs font-bold text-vault-text">Supported File Types</h4>
                  </div>
                  <div class="grid grid-cols-2 gap-1.5 text-[11px]">
                    <div class="flex items-center gap-1.5 text-vault-text-secondary">
                      <span>🖼️</span><span>PNG, JPG, GIF</span>
                    </div>
                    <div class="flex items-center gap-1.5 text-vault-text-secondary">
                      <span>📄</span><span>PDF</span>
                    </div>
                    <div class="flex items-center gap-1.5 text-vault-text-secondary">
                      <span>📝</span><span>DOCX, DOC</span>
                    </div>
                    <div class="flex items-center gap-1.5 text-vault-text-secondary">
                      <span>📊</span><span>XLSX, XLS, CSV</span>
                    </div>
                    <div class="flex items-center gap-1.5 text-vault-text-secondary">
                      <span>📁</span><span>TXT, MD, JSON</span>
                    </div>
                    <div class="flex items-center gap-1.5 text-vault-text-secondary">
                      <span>🗜️</span><span>ZIP, RAR, 7Z</span>
                    </div>
                    <div class="flex items-center gap-1.5 text-vault-text-secondary">
                      <span>🔑</span><span>PEM, KEY, CER</span>
                    </div>
                    <div class="flex items-center gap-1.5 text-vault-text-secondary">
                      <span>📦</span><span>Any binary file</span>
                    </div>
                  </div>
                  <p class="text-[10px] text-vault-text-secondary/60 border-t border-vault-border/40 pt-2">
                    Mọi file đều được mã hoá AES-256-GCM và ký số ECDSA P-256. Không giới hạn định dạng.
                  </p>
                </div>
              </Transition>
            </div>
            <button @click="handleAddFile" :disabled="loading" class="px-4 py-2 bg-vault-accent text-white text-sm font-medium rounded-lg hover:bg-vault-accent-hover transition-colors flex items-center disabled:opacity-50">
              <svg class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M12 4v16m8-8H4"/></svg>
              Add File
            </button>
          </div>
        </div>

        <!-- Drop zone / Table -->
        <div class="flex-1 overflow-y-auto p-6" @dragover="onDragOver" @drop="onDrop">
          <div v-if="files.length===0" class="h-full flex items-center justify-center">
            <div class="text-center border-2 border-dashed border-vault-border rounded-2xl p-12 max-w-sm w-full">
              <div class="text-4xl mb-3">🔒</div>
              <h3 class="font-semibold text-vault-text-secondary mb-1">No encrypted files</h3>
              <p class="text-xs text-vault-text-secondary/60 mb-4">Click "Add File" or drag & drop files here</p>
              <p class="text-[10px] text-vault-text-secondary/40">Supports: PDF, images, documents, and more</p>
            </div>
          </div>

          <div v-else class="bg-vault-surface border border-vault-border rounded-2xl overflow-hidden">
            <table class="w-full text-sm">
              <thead>
                <tr class="border-b border-vault-border bg-vault-card/50">
                  <th class="text-left px-4 py-3 text-xs font-semibold text-vault-text-secondary">File</th>
                  <th class="text-left px-4 py-3 text-xs font-semibold text-vault-text-secondary">Type</th>
                  <th class="text-left px-4 py-3 text-xs font-semibold text-vault-text-secondary">Size</th>
                  <th class="text-left px-4 py-3 text-xs font-semibold text-vault-text-secondary">Added</th>
                  <th class="text-left px-4 py-3 text-xs font-semibold text-vault-text-secondary">Integrity</th>
                  <th class="px-4 py-3"></th>
                </tr>
              </thead>
              <tbody>
                <tr v-for="file in files" :key="file.id" :class="['border-b border-vault-border/50 hover:bg-vault-card/30 transition-colors', file.tampered ? 'bg-vault-danger/5' : '']">
                  <td class="px-4 py-3">
                    <div class="flex items-center gap-2">
                      <span class="text-xl">{{ fileIcon(file.mime_type) }}</span>
                      <div>
                        <div class="font-medium text-vault-text text-xs">{{ file.original_name }}</div>
                        <div class="text-[10px] text-vault-text-secondary font-mono truncate max-w-48" :title="file.content_hash">{{ file.content_hash.substring(0,16) }}…</div>
                      </div>
                    </div>
                  </td>
                  <td class="px-4 py-3 text-xs text-vault-text-secondary">{{ file.mime_type.split('/').pop() }}</td>
                  <td class="px-4 py-3 text-xs text-vault-text">{{ file.size_human }}</td>
                  <td class="px-4 py-3 text-xs text-vault-text-secondary">{{ formatDate(file.created_at) }}</td>
                  <td class="px-4 py-3">
                    <span v-if="file.tampered" class="inline-flex items-center gap-1 text-[10px] text-vault-danger bg-vault-danger/10 px-2 py-0.5 rounded-full">
                      ⚠ Tampered
                    </span>
                    <span v-else class="inline-flex items-center gap-1 text-[10px] text-vault-success bg-vault-success/10 px-2 py-0.5 rounded-full">
                      ✓ Signed
                    </span>
                  </td>
                  <td class="px-4 py-3">
                    <div class="flex items-center gap-1 justify-end">
                      <button @click="handleVerifyFile(file.id)" title="Verify ECDSA signature" class="p-1.5 rounded-lg text-vault-success hover:bg-vault-success/10 transition-colors">
                        <svg class="w-3.5 h-3.5" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M9 12l2 2 4-4m6 2a9 9 0 11-18 0 9 9 0 0118 0z"/></svg>
                      </button>
                      <button @click="handleExportFile(file.id)" title="Decrypt & export" class="p-1.5 rounded-lg text-vault-accent hover:bg-vault-accent/10 transition-colors">
                        <svg class="w-3.5 h-3.5" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M4 16v1a3 3 0 003 3h10a3 3 0 003-3v-1m-4-4l-4 4m0 0l-4-4m4 4V4"/></svg>
                      </button>
                      <button @click="handleDeleteFile(file.id, file.original_name)" title="Delete" class="p-1.5 rounded-lg text-vault-danger hover:bg-vault-danger/10 transition-colors">
                        <svg class="w-3.5 h-3.5" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M19 7l-.867 12.142A2 2 0 0116.138 21H7.862a2 2 0 01-1.995-1.858L5 7m5 4v6m4-6v6m1-10V4a1 1 0 00-1-1h-4a1 1 0 00-1 1v3M4 7h16"/></svg>
                      </button>
                    </div>
                  </td>
                </tr>
              </tbody>
            </table>
          </div>
        </div>
      </div>
    </div>
  </div>
</template>

<style scoped>
/* Markdown preview styles */
:deep(.prose) {
  h1, h2, h3, h4 { color: var(--color-vault-text); margin-top: 1.5em; margin-bottom: 0.5em; font-weight: 700; }
  h1 { font-size: 1.6em; border-bottom: 1px solid var(--color-vault-border); padding-bottom: 0.3em; }
  h2 { font-size: 1.3em; }
  h3 { font-size: 1.1em; }
  p  { color: var(--color-vault-text); margin-bottom: 0.75em; line-height: 1.7; }
  ul, ol { color: var(--color-vault-text); padding-left: 1.5em; margin-bottom: 0.75em; }
  li { margin-bottom: 0.25em; }
  li input[type=checkbox] { margin-right: 0.5em; accent-color: var(--color-vault-accent); }
  blockquote { border-left: 3px solid var(--color-vault-accent); padding-left: 1em; color: var(--color-vault-text-secondary); font-style: italic; margin: 1em 0; }
  code { background: var(--color-vault-card); color: var(--color-vault-accent); padding: 0.1em 0.4em; border-radius: 4px; font-size: 0.85em; }
  pre  { background: var(--color-vault-card); border: 1px solid var(--color-vault-border); border-radius: 8px; padding: 1em; overflow-x: auto; }
  pre code { background: none; padding: 0; color: var(--color-vault-text); }
  a { color: var(--color-vault-accent); text-decoration: underline; }
  table { width: 100%; border-collapse: collapse; margin-bottom: 1em; }
  th { background: var(--color-vault-card); color: var(--color-vault-text); padding: 0.5em 1em; text-align: left; border: 1px solid var(--color-vault-border); }
  td { color: var(--color-vault-text); padding: 0.5em 1em; border: 1px solid var(--color-vault-border); }
  tr:nth-child(even) td { background: var(--color-vault-card)/30; }
  hr { border-color: var(--color-vault-border); margin: 1.5em 0; }
  strong { color: var(--color-vault-text); font-weight: 700; }
}
</style>
