import { readFileSync } from 'node:fs'
import { dirname, resolve } from 'node:path'
import { fileURLToPath } from 'node:url'
import { describe, expect, it } from 'vitest'

const rootDir = resolve(dirname(fileURLToPath(import.meta.url)), '..')
const nginxConfig = readFileSync(resolve(rootDir, 'nginx.conf'), 'utf8')
const dockerCompose = readFileSync(resolve(rootDir, '..', 'docker-compose.yml'), 'utf8')

function locationBlock(path) {
  const escapedPath = path.replace(/[.*+?^${}()|[\]\\]/g, '\\$&')
  const match = nginxConfig.match(new RegExp(`location\\s+${escapedPath}\\s*\\{([\\s\\S]*?)\\n\\s*\\}`))
  return match?.[1] ?? ''
}

function serviceBlock(serviceName) {
  const match = dockerCompose.match(new RegExp(`^  ${serviceName}:\\n([\\s\\S]*?)(?=^  [a-zA-Z0-9_-]+:\\n|^networks:)`, 'm'))
  return match?.[1] ?? ''
}

describe('frontend nginx routing', () => {
  it('serves SPA routes like /login from index.html', () => {
    expect(locationBlock('/')).toContain('try_files $uri $uri/ /index.html')
  })

  it('keeps API traffic proxied to api-server', () => {
    expect(nginxConfig).toContain('set $api_server http://aegis-api-server:8082')
    expect(locationBlock('/api/')).toContain('proxy_pass $api_server')
  })

  it('resolves the API upstream at request time through Docker DNS', () => {
    expect(nginxConfig).toContain('resolver 127.0.0.11')
    expect(nginxConfig).not.toContain('proxy_pass http://aegis-api-server:8082')
  })

  it('serves frontend health locally instead of proxying to api-server', () => {
    const health = locationBlock('= /health')

    expect(health).toContain('return 200')
    expect(health).not.toContain('proxy_pass')
    expect(health).not.toContain('aegis-api-server')
  })

  it('starts frontend independently from api-server health', () => {
    const frontend = serviceBlock('frontend')

    expect(frontend).not.toContain('depends_on:')
    expect(frontend).not.toContain('api-server:')
    expect(frontend).toContain('http://127.0.0.1/health')
  })
})
