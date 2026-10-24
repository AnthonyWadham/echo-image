import { fileURLToPath } from 'node:url'
import { defineConfig } from 'vite'

// The SDK is the local go-ecs checkout, like go.mod's replace, until a release has the MessagePack
// fix. Keep in step with tsconfig.json.
const sdkDir = fileURLToPath(new URL('../../go-ecs/pkg/sdk-ts', import.meta.url))

export default defineConfig(({ mode }) => ({
  resolve: {
    alias: { '@argus-labs/sdk': `${sdkDir}/src/index.ts` },
  },
  // The SDK reads process.env.NODE_ENV, which only Node defines.
  define: {
    'process.env.NODE_ENV': JSON.stringify(mode === 'production' ? 'production' : 'development'),
  },
  server: {
    fs: { allow: ['.', sdkDir] },
    // world start's edge has no CORS, so the browser reaches it through this proxy.
    proxy: {
      '/world': {
        target: 'http://127.0.0.1:8080',
        rewrite: (path) => path.replace(/^\/world/, ''),
      },
    },
  },
}))
