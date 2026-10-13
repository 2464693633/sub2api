/** Keep Grok Imagine aliases in the same picker as the backend xai classifiers. */
export function isGrokImageModel(id: string): boolean {
  const normalized = id.trim().toLowerCase().replace(/^(xai|x-ai|grok)\//, '').trim()
  return normalized === 'grok-imagine' || normalized === 'grok-imagine-1' ||
    normalized === 'grok-imagine-edit' || normalized.startsWith('grok-imagine-image') ||
    (normalized.startsWith('imagine') && !normalized.startsWith('imagine-video'))
}
