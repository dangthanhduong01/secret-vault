<script lang="ts" setup>
import { ref, onMounted, computed } from 'vue'
import { useRouter } from 'vue-router'
import { marked } from 'marked'
import {
  GetNotes, HideInImage, ExtractFromImage,
  IsUnlocked, SelectImage, PreviewHideInImage
} from '../../wailsjs/go/app/App'

const router = useRouter()

interface Note { id: string; title: string; content: string; tags: string[] }

// ── shared ──
const notes         = ref<Note[]>([])
const activeTab     = ref<'hide' | 'extract'>('hide')
const loading       = ref(false)
const toast         = ref('')
const toastType     = ref<'success'|'error'>('success')

// ── hide tab ──
const selectedNoteId   = ref('')
const coverImagePath   = ref('')    // absolute path on disk
const coverImageName   = ref('')
const coverPreviewB64  = ref('')    // original image base64 for display
const coverImageSize   = ref('')
const stegoPreviewB64  = ref('')    // result image base64 for comparison
const stegoDataSize    = ref('')
const stegoSavedPath   = ref('')
const previewLoading   = ref(false)
const showComparison   = ref(false)

const extractedTab     = ref<'raw'|'md'>('raw')
const extractedContent = ref('')
const extractedMd      = computed(() => {
  try { return marked.parse(extractedContent.value || '') as string }
  catch { return '' }
})

// ── lifecycle ──
onMounted(async () => {
  const r = await IsUnlocked()
  if (!r.success || !r.data) { router.push('/'); return }
  const nr = await GetNotes()
  if (nr.success) notes.value = (nr.data as Note[]) ?? []
})

function showToast(msg: string, type: 'success'|'error') {
  toast.value = msg; toastType.value = type
  setTimeout(() => { toast.value = '' }, 4000)
}

const selectedNote = computed(() => notes.value.find(n => n.id === selectedNoteId.value) ?? null)

// Estimate capacity: very rough — requires actual image dimensions from backend
const capacityInfo = computed(() => {
  if (!selectedNote.value || !coverImageSize.value) return null
  const contentBytes = new TextEncoder().encode(selectedNote.value.content).length
  // AES-256-GCM adds ~60 bytes overhead, then base64 ~4/3x
  const estimatedEncSize = Math.ceil((contentBytes + 60) * 1.4)
  return { contentBytes, estimatedEncSize }
})

// ── Hide flow ──
async function pickCoverImage() {
  const r = await SelectImage()
  if (!r.success) {
    if (r.error !== 'No image selected') showToast(r.error || 'Failed', 'error')
    return
  }
  const d = r.data as any
  coverImagePath.value  = d.path
  coverImageName.value  = d.name
  coverImageSize.value  = d.size
  coverPreviewB64.value = d.preview
  stegoPreviewB64.value = ''
  stegoSavedPath.value  = ''
  showComparison.value  = false
}

async function generatePreview() {
  if (!selectedNoteId.value || !coverImagePath.value) return
  previewLoading.value = true
  stegoPreviewB64.value = ''
  const r = await PreviewHideInImage(selectedNoteId.value, coverImagePath.value)
  previewLoading.value = false
  if (r.success) {
    const d = r.data as any
    stegoPreviewB64.value = d.preview
    stegoDataSize.value   = d.data_size
    showComparison.value  = true
  } else {
    showToast(r.error || 'Preview failed', 'error')
  }
}

async function handleHideAndSave() {
  if (!selectedNoteId.value) { showToast('Please select a note', 'error'); return }
  if (!coverImagePath.value)  { showToast('Please select a cover image', 'error'); return }
  loading.value = true
  const r = await HideInImage(selectedNoteId.value, coverImagePath.value)
  loading.value = false
  if (r.success) {
    const d = r.data as any
    stegoSavedPath.value  = d.path
    stegoPreviewB64.value = d.preview
    showComparison.value  = true
    showToast('✓ Data hidden & saved successfully', 'success')
  } else if (r.error !== 'No save location selected' && r.error !== 'No image selected') {
    showToast(r.error || 'Failed', 'error')
  }
}

// ── Extract flow ──
async function handleExtract() {
  loading.value = true
  extractedContent.value = ''
  const r = await ExtractFromImage()
  loading.value = false
  if (r.success) {
    extractedContent.value = (r.data as any).content
    showToast('✓ Data extracted & decrypted', 'success')
  } else if (r.error !== 'No image selected') {
    showToast(r.error || 'Failed to extract', 'error')
  }
}
</script>

<template>
  <div class="h-screen flex flex-col bg-vault-bg overflow-hidden">

    <!-- Toast -->
    <Transition enter-from-class="opacity-0 -translate-y-1" enter-active-class="transition-all duration-200"
                leave-to-class="opacity-0 -translate-y-1" leave-active-class="transition-all duration-200">
      <div v-if="toast" :class="['fixed top-4 right-4 z-50 px-5 py-3 rounded-xl shadow-xl text-sm font-medium',
        toastType==='success' ? 'bg-vault-success/90 text-white' : 'bg-vault-danger/90 text-white']">
        {{ toast }}
      </div>
    </Transition>

    <!-- Header -->
    <div class="h-14 flex items-center gap-3 px-5 border-b border-vault-border bg-vault-surface flex-shrink-0">
      <button @click="router.push('/vault')"
        class="p-2 rounded-lg hover:bg-vault-card text-vault-text-secondary hover:text-vault-text transition-colors">
        <svg class="w-5 h-5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
          <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M15 19l-7-7 7-7"/>
        </svg>
      </button>
      <div class="w-8 h-8 rounded-lg bg-gradient-to-br from-vault-accent to-purple-600 flex items-center justify-center flex-shrink-0">
        <svg class="w-4 h-4 text-white" fill="none" stroke="currentColor" viewBox="0 0 24 24">
          <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M4 16l4.586-4.586a2 2 0 012.828 0L16 16m-2-2l1.586-1.586a2 2 0 012.828 0L20 14m-6-6h.01M6 20h12a2 2 0 002-2V6a2 2 0 00-2-2H6a2 2 0 00-2 2v12a2 2 0 002 2z"/>
        </svg>
      </div>
      <div>
        <h1 class="font-bold text-vault-text text-base leading-tight">Steganography</h1>
        <p class="text-[10px] text-vault-text-secondary leading-none">LSB • AES-256-GCM • Ghost Mode</p>
      </div>
    </div>

    <!-- Tab bar -->
    <div class="flex gap-1 p-2 border-b border-vault-border bg-vault-surface/60 flex-shrink-0">
      <button @click="activeTab='hide'"
        :class="['flex-1 flex items-center justify-center gap-2 py-2 rounded-lg text-sm font-medium transition-all',
          activeTab==='hide' ? 'bg-vault-accent text-white shadow-lg shadow-vault-accent/20' : 'text-vault-text-secondary hover:text-vault-text hover:bg-vault-card']">
        <svg class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
          <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M13.875 18.825A10.05 10.05 0 0112 19c-4.478 0-8.268-2.943-9.543-7a9.97 9.97 0 011.563-3.029m5.858.908a3 3 0 114.243 4.243M9.878 9.878l4.242 4.242M9.88 9.88l-3.29-3.29m7.532 7.532l3.29 3.29M3 3l3.59 3.59m0 0A9.953 9.953 0 0112 5c4.478 0 8.268 2.943 9.543 7a10.025 10.025 0 01-4.132 5.411m0 0L21 21"/>
        </svg>
        Hide in Image
      </button>
      <button @click="activeTab='extract'"
        :class="['flex-1 flex items-center justify-center gap-2 py-2 rounded-lg text-sm font-medium transition-all',
          activeTab==='extract' ? 'bg-vault-accent text-white shadow-lg shadow-vault-accent/20' : 'text-vault-text-secondary hover:text-vault-text hover:bg-vault-card']">
        <svg class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
          <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M15 12a3 3 0 11-6 0 3 3 0 016 0z"/>
          <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M2.458 12C3.732 7.943 7.523 5 12 5c4.478 0 8.268 2.943 9.542 7-1.274 4.057-5.064 7-9.542 7-4.477 0-8.268-2.943-9.542-7z"/>
        </svg>
        Extract from Image
      </button>
    </div>

    <!-- ═══════════ HIDE TAB ═══════════ -->
    <div v-if="activeTab==='hide'" class="flex-1 overflow-y-auto">
      <div class="max-w-5xl mx-auto p-5 space-y-5">

        <!-- Step 1: Select Note -->
        <div class="bg-vault-surface border border-vault-border rounded-2xl overflow-hidden">
          <div class="flex items-center gap-3 px-5 py-3 border-b border-vault-border/50 bg-vault-card/30">
            <span class="w-6 h-6 rounded-full bg-vault-accent text-white text-xs font-bold flex items-center justify-center flex-shrink-0">1</span>
            <span class="text-sm font-semibold text-vault-text">Select encrypted note to hide</span>
          </div>
          <div class="p-5">
            <div class="grid gap-2 max-h-48 overflow-y-auto">
              <button v-for="note in notes" :key="note.id" @click="selectedNoteId = note.id"
                :class="['w-full text-left px-4 py-3 rounded-xl border transition-all',
                  selectedNoteId===note.id
                    ? 'border-vault-accent bg-vault-accent/10 ring-1 ring-vault-accent/30'
                    : 'border-vault-border bg-vault-card/40 hover:border-vault-accent/40 hover:bg-vault-card/70']">
                <div class="flex items-center gap-3">
                  <svg :class="['w-4 h-4 flex-shrink-0', selectedNoteId===note.id ? 'text-vault-accent' : 'text-vault-text-secondary']" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                    <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M9 12h6m-6 4h6m2 5H7a2 2 0 01-2-2V5a2 2 0 012-2h5.586a1 1 0 01.707.293l5.414 5.414a1 1 0 01.293.707V19a2 2 0 01-2 2z"/>
                  </svg>
                  <div class="flex-1 min-w-0">
                    <div class="text-sm font-medium text-vault-text truncate">{{ note.title || 'Untitled' }}</div>
                    <div class="text-[11px] text-vault-text-secondary truncate">{{ note.content.substring(0, 80).replace(/[#*`>]/g,'') }}…</div>
                  </div>
                  <div class="text-[10px] text-vault-text-secondary/60 flex-shrink-0">{{ note.content.length }} chars</div>
                  <svg v-if="selectedNoteId===note.id" class="w-4 h-4 text-vault-accent flex-shrink-0" fill="currentColor" viewBox="0 0 20 20">
                    <path fill-rule="evenodd" d="M10 18a8 8 0 100-16 8 8 0 000 16zm3.707-9.293a1 1 0 00-1.414-1.414L9 10.586 7.707 9.293a1 1 0 00-1.414 1.414l2 2a1 1 0 001.414 0l4-4z" clip-rule="evenodd"/>
                  </svg>
                </div>
              </button>
              <div v-if="notes.length===0" class="text-center py-6 text-vault-text-secondary text-sm">
                No notes found. Create a note in the vault first.
              </div>
            </div>

            <!-- Capacity indicator -->
            <div v-if="selectedNote && coverImagePath && capacityInfo" class="mt-3 flex items-center gap-2 text-xs text-vault-text-secondary bg-vault-card/40 rounded-lg px-3 py-2">
              <svg class="w-3.5 h-3.5 text-vault-accent" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M13 16h-1v-4h-1m1-4h.01M21 12a9 9 0 11-18 0 9 9 0 0118 0z"/></svg>
              Payload: ~{{ (capacityInfo.estimatedEncSize / 1024).toFixed(1) }} KB after AES encryption + base64
            </div>
          </div>
        </div>

        <!-- Step 2: Select Cover Image -->
        <div class="bg-vault-surface border border-vault-border rounded-2xl overflow-hidden">
          <div class="flex items-center gap-3 px-5 py-3 border-b border-vault-border/50 bg-vault-card/30">
            <span class="w-6 h-6 rounded-full bg-vault-accent text-white text-xs font-bold flex items-center justify-center flex-shrink-0">2</span>
            <span class="text-sm font-semibold text-vault-text">Choose cover image (carrier PNG)</span>
          </div>
          <div class="p-5">
            <div v-if="!coverPreviewB64" @click="pickCoverImage"
              class="border-2 border-dashed border-vault-border rounded-xl p-10 flex flex-col items-center gap-3 cursor-pointer hover:border-vault-accent/60 hover:bg-vault-card/30 transition-all group">
              <div class="w-14 h-14 rounded-2xl bg-vault-card/60 group-hover:bg-vault-accent/10 flex items-center justify-center transition-colors">
                <svg class="w-7 h-7 text-vault-text-secondary group-hover:text-vault-accent transition-colors" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                  <path stroke-linecap="round" stroke-linejoin="round" stroke-width="1.5" d="M4 16l4.586-4.586a2 2 0 012.828 0L16 16m-2-2l1.586-1.586a2 2 0 012.828 0L20 14m-6-6h.01M6 20h12a2 2 0 002-2V6a2 2 0 00-2-2H6a2 2 0 00-2 2v12a2 2 0 002 2z"/>
                </svg>
              </div>
              <div class="text-center">
                <p class="text-sm font-medium text-vault-text-secondary group-hover:text-vault-text transition-colors">Click to select a PNG image</p>
                <p class="text-xs text-vault-text-secondary/60 mt-1">The larger the image, the more data it can carry</p>
              </div>
            </div>
            <div v-else class="space-y-3">
              <div class="flex items-center gap-3">
                <div class="flex-1 flex items-center gap-3 bg-vault-card/40 rounded-xl px-4 py-2.5">
                  <svg class="w-5 h-5 text-vault-success flex-shrink-0" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M4 16l4.586-4.586a2 2 0 012.828 0L16 16m-2-2l1.586-1.586a2 2 0 012.828 0L20 14m-6-6h.01M6 20h12a2 2 0 002-2V6a2 2 0 00-2-2H6a2 2 0 00-2 2v12a2 2 0 002 2z"/></svg>
                  <div class="flex-1 min-w-0">
                    <div class="text-sm font-medium text-vault-text truncate">{{ coverImageName }}</div>
                    <div class="text-xs text-vault-text-secondary">{{ coverImageSize }}</div>
                  </div>
                </div>
                <button @click="pickCoverImage" class="px-3 py-2 text-xs text-vault-text-secondary hover:text-vault-text bg-vault-card border border-vault-border rounded-lg transition-colors">
                  Change
                </button>
              </div>
              <img :src="coverPreviewB64" alt="Cover image" class="w-full max-h-48 object-contain rounded-xl border border-vault-border bg-vault-card/20"/>
            </div>
          </div>
        </div>

        <!-- Step 3: Preview + Save -->
        <div class="bg-vault-surface border border-vault-border rounded-2xl overflow-hidden">
          <div class="flex items-center gap-3 px-5 py-3 border-b border-vault-border/50 bg-vault-card/30">
            <span class="w-6 h-6 rounded-full bg-vault-accent text-white text-xs font-bold flex items-center justify-center flex-shrink-0">3</span>
            <span class="text-sm font-semibold text-vault-text">Preview & export steganographic image</span>
          </div>
          <div class="p-5 space-y-4">
            <!-- Action buttons row -->
            <div class="flex gap-3">
              <button @click="generatePreview"
                :disabled="!selectedNoteId || !coverImagePath || previewLoading"
                class="flex-1 flex items-center justify-center gap-2 py-2.5 text-sm font-medium rounded-xl border border-vault-accent/40 text-vault-accent hover:bg-vault-accent/10 transition-all disabled:opacity-40 disabled:cursor-not-allowed">
                <svg v-if="previewLoading" class="w-4 h-4 animate-spin" fill="none" viewBox="0 0 24 24"><circle class="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" stroke-width="4"/><path class="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4z"/></svg>
                <svg v-else class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M15 12a3 3 0 11-6 0 3 3 0 016 0z"/><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M2.458 12C3.732 7.943 7.523 5 12 5c4.478 0 8.268 2.943 9.542 7-1.274 4.057-5.064 7-9.542 7-4.477 0-8.268-2.943-9.542-7z"/></svg>
                {{ previewLoading ? 'Generating…' : 'Preview Result' }}
              </button>
              <button @click="handleHideAndSave"
                :disabled="!selectedNoteId || !coverImagePath || loading"
                class="flex-1 flex items-center justify-center gap-2 py-2.5 text-sm font-semibold rounded-xl bg-gradient-to-r from-vault-accent to-purple-600 text-white hover:from-vault-accent-hover hover:to-purple-500 transition-all shadow-lg shadow-vault-accent/20 disabled:opacity-40 disabled:cursor-not-allowed">
                <svg v-if="loading" class="w-4 h-4 animate-spin" fill="none" viewBox="0 0 24 24"><circle class="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" stroke-width="4"/><path class="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4z"/></svg>
                <svg v-else class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M4 16v1a3 3 0 003 3h10a3 3 0 003-3v-1m-4-4l-4 4m0 0l-4-4m4 4V4"/></svg>
                {{ loading ? 'Encoding…' : 'Save Stego Image' }}
              </button>
            </div>

            <!-- Side-by-side comparison -->
            <Transition enter-from-class="opacity-0 translate-y-2" enter-active-class="transition-all duration-300">
              <div v-if="showComparison && (coverPreviewB64 || stegoPreviewB64)" class="space-y-3">
                <div class="flex items-center gap-2">
                  <div class="flex-1 h-px bg-vault-border/40"></div>
                  <span class="text-[11px] text-vault-text-secondary px-2">Visual comparison — mắt thường không phân biệt được sự khác biệt</span>
                  <div class="flex-1 h-px bg-vault-border/40"></div>
                </div>
                <div class="grid grid-cols-2 gap-3">
                  <!-- Original -->
                  <div class="space-y-1.5">
                    <div class="flex items-center gap-1.5 text-xs font-medium text-vault-text-secondary">
                      <span class="w-2 h-2 rounded-full bg-vault-success"></span>
                      Original (cover)
                    </div>
                    <div class="relative rounded-xl overflow-hidden border border-vault-border bg-vault-card/20">
                      <img :src="coverPreviewB64" alt="Original" class="w-full object-contain max-h-56"/>
                      <div class="absolute bottom-0 left-0 right-0 px-3 py-1.5 bg-black/40 backdrop-blur-sm">
                        <p class="text-[10px] text-white/80 truncate">{{ coverImageName }} · {{ coverImageSize }}</p>
                      </div>
                    </div>
                  </div>
                  <!-- Stego result -->
                  <div class="space-y-1.5">
                    <div class="flex items-center gap-1.5 text-xs font-medium text-vault-text-secondary">
                      <span class="w-2 h-2 rounded-full bg-vault-accent"></span>
                      Steganographic result
                    </div>
                    <div class="relative rounded-xl overflow-hidden border border-vault-accent/30 bg-vault-card/20">
                      <img v-if="stegoPreviewB64" :src="stegoPreviewB64" alt="Stego result" class="w-full object-contain max-h-56"/>
                      <div v-else class="w-full h-56 flex items-center justify-center">
                        <svg class="w-8 h-8 text-vault-text-secondary/30 animate-spin" fill="none" viewBox="0 0 24 24"><circle class="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" stroke-width="4"/><path class="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4z"/></svg>
                      </div>
                      <div class="absolute bottom-0 left-0 right-0 px-3 py-1.5 bg-black/40 backdrop-blur-sm">
                        <p class="text-[10px] text-white/80">🔒 Contains hidden AES-256 payload · {{ stegoDataSize }}</p>
                      </div>
                    </div>
                  </div>
                </div>
                <!-- Saved path -->
                <div v-if="stegoSavedPath" class="flex items-center gap-2 bg-vault-success/10 border border-vault-success/20 rounded-xl px-4 py-2.5">
                  <svg class="w-4 h-4 text-vault-success flex-shrink-0" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M5 13l4 4L19 7"/></svg>
                  <div class="min-w-0">
                    <p class="text-xs font-medium text-vault-success">Saved successfully</p>
                    <p class="text-[10px] text-vault-text-secondary truncate">{{ stegoSavedPath }}</p>
                  </div>
                </div>
              </div>
            </Transition>

            <!-- Info badges -->
            <div class="flex flex-wrap gap-2">
              <span class="inline-flex items-center gap-1.5 text-[10px] text-vault-text-secondary bg-vault-card/50 px-2.5 py-1 rounded-full border border-vault-border/50">
                <span class="w-1.5 h-1.5 rounded-full bg-vault-accent"></span> LSB 1-bit per channel
              </span>
              <span class="inline-flex items-center gap-1.5 text-[10px] text-vault-text-secondary bg-vault-card/50 px-2.5 py-1 rounded-full border border-vault-border/50">
                <span class="w-1.5 h-1.5 rounded-full bg-vault-success"></span> AES-256-GCM encrypted
              </span>
              <span class="inline-flex items-center gap-1.5 text-[10px] text-vault-text-secondary bg-vault-card/50 px-2.5 py-1 rounded-full border border-vault-border/50">
                <span class="w-1.5 h-1.5 rounded-full bg-purple-400"></span> PNG lossless output
              </span>
            </div>
          </div>
        </div>

      </div>
    </div>

    <!-- ═══════════ EXTRACT TAB ═══════════ -->
    <div v-if="activeTab==='extract'" class="flex-1 overflow-y-auto">
      <div class="max-w-2xl mx-auto p-5 space-y-5">

        <!-- How it works -->
        <div class="bg-gradient-to-br from-vault-accent/8 to-purple-600/8 border border-vault-accent/20 rounded-2xl p-5">
          <div class="flex gap-3">
            <svg class="w-5 h-5 text-vault-accent flex-shrink-0 mt-0.5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
              <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M13 16h-1v-4h-1m1-4h.01M21 12a9 9 0 11-18 0 9 9 0 0118 0z"/>
            </svg>
            <div class="space-y-1 text-xs text-vault-text-secondary leading-relaxed">
              <p class="font-medium text-vault-text text-sm">How to extract</p>
              <p>1. Click the button below and select a steganographic PNG image</p>
              <p>2. The app reads the LSB bits from pixel channels to reconstruct the encrypted payload</p>
              <p>3. The vault password is used to AES-256-GCM decrypt the extracted data</p>
              <p>4. The plaintext Markdown is shown below with full rendering</p>
            </div>
          </div>
        </div>

        <!-- Extract button -->
        <button @click="handleExtract" :disabled="loading"
          class="w-full flex items-center justify-center gap-3 py-4 bg-gradient-to-r from-vault-accent to-purple-600 text-white font-semibold rounded-2xl hover:from-vault-accent-hover hover:to-purple-500 transition-all shadow-lg shadow-vault-accent/20 disabled:opacity-50 disabled:cursor-not-allowed">
          <svg v-if="loading" class="w-5 h-5 animate-spin" fill="none" viewBox="0 0 24 24"><circle class="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" stroke-width="4"/><path class="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4z"/></svg>
          <svg v-else class="w-5 h-5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
            <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M15 12a3 3 0 11-6 0 3 3 0 016 0z"/>
            <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M2.458 12C3.732 7.943 7.523 5 12 5c4.478 0 8.268 2.943 9.542 7-1.274 4.057-5.064 7-9.542 7-4.477 0-8.268-2.943-9.542-7z"/>
          </svg>
          {{ loading ? 'Extracting LSB data…' : 'Select Image & Extract' }}
        </button>

        <!-- Extracted result -->
        <Transition enter-from-class="opacity-0 translate-y-2" enter-active-class="transition-all duration-300">
          <div v-if="extractedContent" class="bg-vault-surface border border-vault-success/30 rounded-2xl overflow-hidden">
            <!-- Result header -->
            <div class="flex items-center gap-3 px-5 py-3 border-b border-vault-border/50 bg-vault-success/5">
              <svg class="w-4 h-4 text-vault-success" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M9 12l2 2 4-4m6 2a9 9 0 11-18 0 9 9 0 0118 0z"/>
              </svg>
              <span class="text-sm font-semibold text-vault-success">Decrypted content</span>
              <span class="ml-auto text-[10px] text-vault-text-secondary">{{ extractedContent.length }} chars</span>
            </div>

            <!-- Tab: raw / rendered -->
            <div class="flex border-b border-vault-border/50">
              <button @click="extractedTab='raw'"
                :class="['px-4 py-2 text-xs font-medium border-b-2 transition-colors',
                  extractedTab==='raw' ? 'text-vault-accent border-vault-accent' : 'text-vault-text-secondary border-transparent hover:text-vault-text']">
                Raw text
              </button>
              <button @click="extractedTab='md'"
                :class="['px-4 py-2 text-xs font-medium border-b-2 transition-colors',
                  extractedTab==='md' ? 'text-vault-accent border-vault-accent' : 'text-vault-text-secondary border-transparent hover:text-vault-text']">
                Markdown preview
              </button>
            </div>

            <div v-if="extractedTab==='raw'" class="p-5">
              <pre class="text-sm text-vault-text whitespace-pre-wrap break-words font-mono leading-relaxed max-h-80 overflow-y-auto">{{ extractedContent }}</pre>
            </div>
            <div v-else class="p-5 prose prose-sm max-w-none max-h-80 overflow-y-auto" v-html="extractedMd"
              style="color:var(--color-vault-text)">
            </div>
          </div>
        </Transition>

      </div>
    </div>

  </div>
</template>

<style scoped>
:deep(.prose) {
  h1,h2,h3 { color: var(--color-vault-text); font-weight: 700; margin-bottom: 0.4em; }
  p { color: var(--color-vault-text); line-height: 1.7; margin-bottom: 0.6em; }
  ul,ol { color: var(--color-vault-text); padding-left: 1.5em; }
  code { background: var(--color-vault-card); color: var(--color-vault-accent); padding: 0.1em 0.4em; border-radius: 4px; font-size: 0.85em; }
  pre { background: var(--color-vault-card); border-radius: 8px; padding: 0.8em; overflow-x: auto; }
  blockquote { border-left: 3px solid var(--color-vault-accent); padding-left: 1em; color: var(--color-vault-text-secondary); }
  strong { color: var(--color-vault-text); }
}
</style>
