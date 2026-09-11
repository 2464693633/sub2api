import type { GroupPlatform } from '@/types'

export const OPENAI_CC_SWITCH_CODEX_MODEL = 'gpt-5.5'
export const GROK_CC_SWITCH_MODEL = 'grok-4.5'

export type CcSwitchClientType = 'claude' | 'gemini'

/**
 * Codex 上下文窗口选项。CC Switch 3.19.1 的供应商深链会丢弃上下文字段，
 * 因此这两个值无法随 deeplink 导入，只能让用户手动粘贴到 config.toml 顶部。
 */
export type CodexContextWindowOption = '1m' | 'standard'

export interface CodexContextWindowSpec {
  /** model_context_window */
  contextWindow: number
  /** model_auto_compact_token_limit */
  autoCompactTokenLimit: number
}

export const CODEX_CONTEXT_WINDOW_SPECS: Record<CodexContextWindowOption, CodexContextWindowSpec> = {
  '1m': { contextWindow: 1050000, autoCompactTokenLimit: 800000 },
  standard: { contextWindow: 272000, autoCompactTokenLimit: 244800 }
}

export function buildCodexContextConfigSnippet(option: CodexContextWindowOption): string {
  const spec = CODEX_CONTEXT_WINDOW_SPECS[option]
  return [
    `model_context_window = ${spec.contextWindow}`,
    `model_auto_compact_token_limit = ${spec.autoCompactTokenLimit}`
  ].join('\n')
}

export interface CcSwitchImportConfig {
  app: string
  endpoint: string
  model?: string
}

export interface CcSwitchImportDeeplinkInput {
  baseUrl: string
  platform?: GroupPlatform | null
  clientType: CcSwitchClientType
  providerName: string
  apiKey: string
  usageScript: string
}

function withV1Endpoint(baseUrl: string): string {
  const normalizedBaseUrl = baseUrl.replace(/\/+$/, '')
  return normalizedBaseUrl.endsWith('/v1') ? normalizedBaseUrl : `${normalizedBaseUrl}/v1`
}

export function resolveCcSwitchImportConfig(
  platform: GroupPlatform | undefined | null,
  clientType: CcSwitchClientType,
  baseUrl: string
): CcSwitchImportConfig {
  switch (platform || 'anthropic') {
    case 'antigravity':
      return {
        app: clientType === 'gemini' ? 'gemini' : 'claude',
        endpoint: `${baseUrl}/antigravity`
      }
    case 'openai':
      return {
        app: 'codex',
        endpoint: baseUrl,
        model: OPENAI_CC_SWITCH_CODEX_MODEL
      }
    case 'gemini':
      return {
        app: 'gemini',
        endpoint: baseUrl
      }
    case 'grok':
      return {
        app: 'grokbuild',
        endpoint: withV1Endpoint(baseUrl),
        model: GROK_CC_SWITCH_MODEL
      }
    default:
      return {
        app: 'claude',
        endpoint: baseUrl
      }
  }
}

export function buildCcSwitchImportDeeplink(input: CcSwitchImportDeeplinkInput): string {
  const config = resolveCcSwitchImportConfig(input.platform, input.clientType, input.baseUrl)
  const entries: [string, string][] = [
    ['resource', 'provider'],
    ['app', config.app],
    ['name', input.providerName],
    ['homepage', input.baseUrl],
    ['endpoint', config.endpoint],
    ['apiKey', input.apiKey],
    ['configFormat', 'json'],
    ['usageEnabled', 'true'],
    ['usageScript', btoa(input.usageScript)],
    ['usageAutoInterval', '30']
  ]

  if (config.model) {
    entries.splice(2, 0, ['model', config.model])
  }

  return `ccswitch://v1/import?${new URLSearchParams(entries).toString()}`
}
