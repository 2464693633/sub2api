/**
 * AI 提示词助手:调用 /api/v1/studio/ai-prompt 把一句创意扩写成高质量生成提示词。
 * 用户可自选一把自己的 API 密钥和该密钥可用的对话模型。
 * 优化请求经网关计费链路,费用按所选密钥的分组定价计入账户。
 */
import { ref } from 'vue'
import { i18n } from '@/i18n'
import { sessionOwnerKey as sessionOwner, StudioSessionChangedError } from './studioSession'

const t = i18n.global.t

const CHAT_MODEL_EXCLUDE = /(image|video|imagine|dall|flux|seedream|banana|diffusion|seedance|veo|sora|kling)/i

// 工作台自动创建的系统密钥,不出现在 AI 优化选择器里
const SYSTEM_KEY_NAMES = new Set(['生图工作台', '视频工作台', 'AI提示词'])

// ===== 共享选择状态(生图/视频工作台共用,localStorage 持久化) =====
export interface StudioKeyOption { id: number; name: string; key: string }

export const aiKeys = ref<StudioKeyOption[]>([])
export const aiKeyId = ref<number | null>(null)
export const aiModels = ref<string[]>([])
export const aiModel = ref('')
export const aiOptionsLoading = ref(false)

const LS_KEY_ID = 'ai_prompt_key_id'
const LS_MODEL = 'ai_prompt_model'
let optionsOwner = ''
let optionsReqSeq = 0
let sessionEpoch = 0

function ownerStorageKey(base: string, owner = sessionOwner()): string {
  return `${base}:${owner}`
}

function authHeader(): Record<string, string> {
  const token = localStorage.getItem('auth_token') || ''
  return { Authorization: `Bearer ${token}` }
}

/** 拉取用户自己的可用密钥(启用且绑定了分组) */
export async function listStudioKeys(): Promise<StudioKeyOption[]> {
  const res = await fetch(`${window.location.origin}/api/v1/keys?page=1&page_size=100`, { headers: authHeader() })
  const body = await res.json().catch(() => null) as {
    data?: { items?: Array<{ id: number; name: string; key: string; status: string; group_id?: number | null; group_ids?: number[] }> }
  } | null
  const items = body?.data?.items || []
  return items
    .filter(k => k.status === 'active' && (k.group_id || (k.group_ids && k.group_ids.length)) && k.key)
    .filter(k => !SYSTEM_KEY_NAMES.has(k.name || ''))
    .map(k => ({ id: k.id, name: k.name || `Key #${k.id}`, key: k.key }))
}

/** 用指定密钥拉取网关模型列表,过滤出对话模型 */
export async function listChatModelsForKey(keyValue: string): Promise<string[]> {
  const res = await fetch(`${window.location.origin}/v1/models`, {
    headers: { ...authHeader(), Authorization: `Bearer ${keyValue}` }
  })
  const body = await res.json().catch(() => null) as { data?: Array<{ id?: string }> } | null
  return (body?.data || [])
    .map(m => (m?.id || '').trim())
    .filter(id => id && !CHAT_MODEL_EXCLUDE.test(id))
}

/** 初始化密钥/模型选项(幂等,两个工作台共用) */
export async function initAIOptions(force = false) {
  const owner = sessionOwner()
  if (owner !== optionsOwner) {
    resetAIPromptSession(false)
    optionsOwner = owner
  }
  if (aiOptionsLoading.value) return
  if (!force && aiKeys.value.length) return
  aiOptionsLoading.value = true
  const optionsRequest = ++optionsReqSeq
  try {
    const keys = await listStudioKeys()
    if (optionsRequest !== optionsReqSeq || owner !== sessionOwner()) return
    aiKeys.value = keys
    const savedKeyId = Number(localStorage.getItem(ownerStorageKey(LS_KEY_ID, owner)) || 0)
    const found = aiKeys.value.find(k => k.id === savedKeyId)
    aiKeyId.value = (found || aiKeys.value[0])?.id ?? null
    const selected = aiKeys.value.find(k => k.id === aiKeyId.value)
    if (selected) {
      const seq = ++modelReqSeq
      const models = await listChatModelsForKey(selected.key)
      if (seq !== modelReqSeq || owner !== sessionOwner()) return
      aiModels.value = models
    }
    const savedModel = localStorage.getItem(ownerStorageKey(LS_MODEL, owner)) || ''
    if (savedModel && aiModels.value.includes(savedModel)) {
      aiModel.value = savedModel
    } else {
      aiModel.value = aiModels.value[0] || ''
    }
  } catch {
    if (optionsRequest === optionsReqSeq) aiKeys.value = []
  } finally {
    if (optionsRequest === optionsReqSeq) aiOptionsLoading.value = false
  }
}

// 模型列表请求序号:快速切换密钥时丢弃过期响应,防止旧密钥的慢响应
// 覆盖新密钥的状态(导致提交错配的 key/model 组合)
let modelReqSeq = 0

/** 切换密钥后重载该密钥可用的对话模型 */
export async function onAIKeyChange() {
  const owner = sessionOwner()
  if (optionsOwner !== owner) {
    resetAIPromptSession()
    return
  }
  localStorage.setItem(ownerStorageKey(LS_KEY_ID, owner), String(aiKeyId.value || ''))
  const selected = aiKeys.value.find(k => k.id === aiKeyId.value)
  aiModels.value = []
  aiModel.value = ''
  const seq = ++modelReqSeq
  if (!selected) return
  try {
    const models = await listChatModelsForKey(selected.key)
    if (seq !== modelReqSeq || owner !== sessionOwner()) return // 已切换到其他密钥/会话,丢弃
    aiModels.value = models
    const savedModel = localStorage.getItem(ownerStorageKey(LS_MODEL, owner)) || ''
    aiModel.value = savedModel && models.includes(savedModel) ? savedModel : (models[0] || '')
    if (aiModel.value) localStorage.setItem(ownerStorageKey(LS_MODEL, owner), aiModel.value)
  } catch {
    if (seq === modelReqSeq) aiModels.value = []
  }
}

export function rememberAIModel() {
  if (optionsOwner !== sessionOwner()) return
  if (aiModel.value) localStorage.setItem(ownerStorageKey(LS_MODEL), aiModel.value)
}

/** Drop all key material and pending option requests when the login session ends. */
export function resetAIPromptSession(clearOwner = true) {
  sessionEpoch++
  optionsReqSeq++
  modelReqSeq++
  aiKeys.value = []
  aiKeyId.value = null
  aiModels.value = []
  aiModel.value = ''
  aiOptionsLoading.value = false
  if (clearOwner) optionsOwner = ''
}

if (typeof window !== 'undefined') {
  window.addEventListener('studio-session-ended', () => resetAIPromptSession())
}

export async function aiEnhancePrompt(
  kind: 'image' | 'video',
  prompt: string,
  opts: { keyId?: number; model?: string }
): Promise<string> {
  // 仅支持用户自选密钥+模型(后端同样强制校验)
  if (!opts?.keyId) throw new Error(t('canvas.aiNeedKey'))
  const owner = sessionOwner()
  const requestEpoch = sessionEpoch
  const token = localStorage.getItem('auth_token') || ''
  // 上游偶发挂起:75 秒无响应主动超时,提示更换密钥/模型
  const controller = new AbortController()
  const timer = window.setTimeout(() => controller.abort(), 75000)
  try {
    const res = await fetch(`${window.location.origin}/api/v1/studio/ai-prompt`, {
      method: 'POST',
      headers: { Authorization: `Bearer ${token}`, 'Content-Type': 'application/json' },
      body: JSON.stringify({
        prompt,
        kind,
        key_id: opts?.keyId || undefined,
        model: opts?.model || undefined
      }),
      signal: controller.signal
    })
  const body = await res.json().catch(() => null) as {
    message?: string
    error?: { message?: string }
    choices?: Array<{ message?: { content?: string | Array<{ text?: string }> } }>
  } | null
  if (requestEpoch !== sessionEpoch || owner !== sessionOwner()) throw new StudioSessionChangedError()
  if (!res.ok) {
    throw new Error(body?.message || body?.error?.message || `HTTP ${res.status}`)
  }
  const raw = body?.choices?.[0]?.message?.content
  const text = typeof raw === 'string'
    ? raw
    : (Array.isArray(raw) ? raw.map(p => p?.text || '').join('') : '')
  const cleaned = text.trim()
  if (!cleaned) throw new Error(t('canvas.aiPromptFailed'))
  return cleaned
  } catch (err) {
    if (requestEpoch !== sessionEpoch || owner !== sessionOwner()) throw new StudioSessionChangedError()
    if (err instanceof DOMException && err.name === 'AbortError') {
      throw new Error(t('canvas.aiPromptTimeout'))
    }
    throw err
  } finally {
    window.clearTimeout(timer)
  }
}
