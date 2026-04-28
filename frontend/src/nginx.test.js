import { readFileSync } from 'node:fs'
import { dirname, resolve } from 'node:path'
import { fileURLToPath } from 'node:url'
import { describe, expect, it } from 'vitest'

const rootDir = resolve(dirname(fileURLToPath(import.meta.url)), '..')
const nginxConfig = readFileSync(resolve(rootDir, 'nginx.conf'), 'utf8')

function locationBlock(path) {
  const escapedPath = path.replace(/[.*+?^${}()|[\]\\]/g, '\\$&')
  const match = nginxConfig.match(new RegExp(`location\\s+${escapedPath}\\s*\\{([\\s\\S]*?)\\n\\s*\\}`))
  return match?.[1] ?? ''
}

describe('frontend nginx routing', () => {
  it('serves SPA routes like /login from index.html', () => {
    expect(locationBlock('/')).toContain('try_files $uri $uri/ /index.html')
  })

  it('keeps API traffic proxied to api-server', () => {
    expect(locationBlock('/api/')).toContain('proxy_pass http://aegis-api-server:8082')
  })

  it('serves frontend health locally instead of proxying to api-server', () => {
    const health = locationBlock('= /health')

    expect(health).toContain('return 200')
    expect(health).not.toContain('proxy_pass')
    expect(health).not.toContain('aegis-api-server')
  })
})
