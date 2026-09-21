/**
 * AI 提示词助手:调用 /api/v1/studio/ai-prompt 把一句创意扩写成高质量生成提示词。
 * 用户可自选一把自己的 API 密钥和该密钥可用的对话模型;不选时后端自动挑选。
 * 优化请求经网关计费链路,费用按所选密钥的分组定价计入账户。
 */
import { ref } from 'vue'
import { i18n } from '@/i18n'

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
  if (aiOptionsLoading.value) return
  if (!force && aiKeys.value.length) return
  aiOptionsLoading.value = true
  try {
    aiKeys.value = await listStudioKeys()
    const savedKeyId = Number(localStorage.getItem(LS_KEY_ID) || 0)
    const found = aiKeys.value.find(k => k.id === savedKeyId)
    aiKeyId.value = (found || aiKeys.value[0])?.id ?? null
    const selected = aiKeys.value.find(k => k.id === aiKeyId.value)
    if (selected) {
      const seq = ++modelReqSeq
      const models = await listChatModelsForKey(selected.key)
      if (seq === modelReqSeq) aiModels.value = models
    }
    const savedModel = localStorage.getItem(LS_MODEL) || ''
    if (savedModel && aiModels.value.includes(savedModel)) {
      aiModel.value = savedModel
    } else {
      aiModel.value = aiModels.value[0] || ''
    }
  } catch {
    aiKeys.value = []
  } finally {
    aiOptionsLoading.value = false
  }
}

// 模型列表请求序号:快速切换密钥时丢弃过期响应,防止旧密钥的慢响应
// 覆盖新密钥的状态(导致提交错配的 key/model 组合)
let modelReqSeq = 0

/** 切换密钥后重载该密钥可用的对话模型 */
export async function onAIKeyChange() {
  localStorage.setItem(LS_KEY_ID, String(aiKeyId.value || ''))
  const selected = aiKeys.value.find(k => k.id === aiKeyId.value)
  aiModels.value = []
  aiModel.value = ''
  if (!selected) return
  const seq = ++modelReqSeq
  try {
    const models = await listChatModelsForKey(selected.key)
    if (seq !== modelReqSeq) return // 已切换到其他密钥,丢弃
    aiModels.value = models
    const savedModel = localStorage.getItem(LS_MODEL) || ''
    aiModel.value = savedModel && models.includes(savedModel) ? savedModel : (models[0] || '')
    if (aiModel.value) localStorage.setItem(LS_MODEL, aiModel.value)
  } catch {
    if (seq === modelReqSeq) aiModels.value = []
  }
}

export function rememberAIModel() {
  if (aiModel.value) localStorage.setItem(LS_MODEL, aiModel.value)
}

export async function aiEnhancePrompt(
  kind: 'image' | 'video',
  prompt: string,
  opts?: { keyId?: number; model?: string }
): Promise<string> {
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
    if (err instanceof DOMException && err.name === 'AbortError') {
      throw new Error(t('canvas.aiPromptTimeout'))
    }
    throw err
  } finally {
    window.clearTimeout(timer)
  }
}
