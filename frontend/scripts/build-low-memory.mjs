import { resolve } from 'node:path'
import { fileURLToPath } from 'node:url'
import { build, loadConfigFromFile } from 'vite'
import vue from '@vitejs/plugin-vue'
import * as compiler from 'vue/compiler-sfc'

// Bundle only: run type checking and tests separately. The normal build keeps
// those checks; this entry point supports servers with a limited Node heap.
const frontend = fileURLToPath(new URL('..', import.meta.url))
const outputDirectory = process.argv[2] && resolve(process.argv[2])
process.chdir(frontend)

const descriptors = new Set()
const scripts = new Set()
const buildCompiler = {
  ...compiler,
  parse(...args) {
    const result = compiler.parse(...args)
    descriptors.add(result.descriptor)
    return result
  },
  compileScript(...args) {
    const result = compiler.compileScript(...args)
    scripts.add(result)
    return result
  },
}

const { config } = await loadConfigFromFile(
  { command: 'build', mode: 'production' },
  resolve(frontend, 'vite.config.ts'),
)

await build({
  ...config,
  configFile: false,
  plugins: [
    ...config.plugins.filter(plugin => !['vite:vue', 'vite-plugin-checker'].includes(plugin?.name)),
    vue({ compiler: buildCompiler }),
    {
      name: 'release-vue-build-ast',
      buildEnd() {
        // All SFC compilation is finished at this point. These ASTs are used
        // for HMR, which a one-shot production build never runs. Release them
        // before Rollup allocates memory for tree shaking and chunk rendering.
        for (const descriptor of descriptors) {
          if (descriptor.template) descriptor.template.ast = undefined
        }
        for (const script of scripts) {
          script.scriptAst = undefined
          script.scriptSetupAst = undefined
        }
        descriptors.clear()
        scripts.clear()
        compiler.parseCache.clear()
        global.gc?.()
      },
    },
  ],
  build: {
    ...config.build,
    ...(outputDirectory ? { outDir: outputDirectory } : {}),
    reportCompressedSize: false,
    rollupOptions: { ...config.build.rollupOptions, maxParallelFileOps: 1 },
  },
})
