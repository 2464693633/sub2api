/** gifenc 无官方类型声明,按实际使用的 API 手写 */
declare module 'gifenc' {
  export interface GIFEncoderInstance {
    writeFrame(index: Uint8Array, width: number, height: number, opts?: {
      palette?: number[][]
      delay?: number
      transparent?: boolean
      dispose?: number
      first?: boolean
      repeat?: number
    }): void
    finish(): void
    bytes(): Uint8Array
    bytesView(): Uint8Array
    reset(): void
  }
  export function GIFEncoder(opts?: { auto?: boolean; initialCapacity?: number }): GIFEncoderInstance
  export function quantize(rgba: Uint8Array | Uint8ClampedArray, maxColors: number, opts?: {
    format?: 'rgb565' | 'rgb444' | 'rgba4444'
    oneBitAlpha?: boolean
    clearAlpha?: boolean
  }): number[][]
  export function applyPalette(rgba: Uint8Array | Uint8ClampedArray, palette: number[][], format?: 'rgb565' | 'rgb444' | 'rgba4444'): Uint8Array
}
