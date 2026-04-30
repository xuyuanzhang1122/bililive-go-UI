#!/usr/bin/env node

import { createServer } from 'node:net'
import { createInterface } from 'node:readline/promises'
import { stdin as input, stdout as output } from 'node:process'
import { mkdir, writeFile, copyFile } from 'node:fs/promises'
import { existsSync } from 'node:fs'
import { homedir } from 'node:os'
import { resolve } from 'node:path'
import { spawnSync } from 'node:child_process'
import { randomBytes } from 'node:crypto'

const REPO = 'xuyuanzhang1122/bililive-go-UI'
const RAW_BASE = `https://raw.githubusercontent.com/${REPO}/main`
const DOCKER_IMAGE = 'xuniubi/bililive-go'
const CONTAINER_NAME = 'bililive-go'

const args = process.argv.slice(2)

const options = {
  dir: '',
  port: '',
  image: '',
  enableApiKey: undefined,
  apiKey: '',
  yes: false,
  help: false,
}

function usage() {
  console.log(`bililive-go 一次性安装器（npm/npx）

用法:
  npx bililive-go-ui install
  npx bililive-go-ui install --yes
  npx bililive-go-ui install --dir ~/bililive-go --port 8080 --enable-api-key

参数:
  install              安装 Docker 版 bililive-go（可省略）
  --dir PATH           安装目录，默认 ~/bililive-go
  --port N             Web UI 主机端口，默认 8080
  --image TAG          Docker 镜像 tag，默认 latest
  --version TAG        等同于 --image
  --enable-api-key     自动启用 API Key 并随机生成
  --api-key STR        指定 API Key（仅在 --enable-api-key 时生效）
  --yes, -y            非交互模式，全部使用默认值或命令行参数
  --help, -h           显示帮助
`)
}

function takeValue(index, name) {
  const value = args[index + 1]
  if (!value || value.startsWith('-')) {
    throw new Error(`${name} 缺少参数值`)
  }
  return value
}

for (let i = 0; i < args.length; i += 1) {
  const arg = args[i]
  switch (arg) {
    case 'install':
      break
    case '--dir':
      options.dir = takeValue(i, arg)
      i += 1
      break
    case '--port':
      options.port = takeValue(i, arg)
      i += 1
      break
    case '--image':
    case '--version':
      options.image = takeValue(i, arg)
      i += 1
      break
    case '--enable-api-key':
      options.enableApiKey = true
      break
    case '--api-key':
      options.apiKey = takeValue(i, arg)
      i += 1
      break
    case '--yes':
    case '-y':
      options.yes = true
      break
    case '--help':
    case '-h':
      options.help = true
      break
    default:
      throw new Error(`未知参数: ${arg}`)
  }
}

if (options.help) {
  usage()
  process.exit(0)
}

const color = {
  green: '\x1b[1;32m',
  yellow: '\x1b[1;33m',
  red: '\x1b[1;31m',
  blue: '\x1b[1;34m',
  reset: '\x1b[0m',
}

function log(message) {
  console.log(`${color.blue}->${color.reset} ${message}`)
}

function ok(message) {
  console.log(`${color.green}OK${color.reset} ${message}`)
}

function warn(message) {
  console.log(`${color.yellow}!!${color.reset} ${message}`)
}

function fail(message) {
  console.error(`${color.red}XX${color.reset} ${message}`)
}

function run(command, commandArgs, runOptions = {}) {
  const result = spawnSync(command, commandArgs, {
    stdio: runOptions.capture ? 'pipe' : 'inherit',
    encoding: 'utf8',
  })
  if (result.error) {
    throw result.error
  }
  if (result.status !== 0 && !runOptions.allowFailure) {
    throw new Error(`${command} ${commandArgs.join(' ')} 执行失败`)
  }
  return result
}

async function ask(rl, prompt, defaultValue) {
  if (options.yes) {
    console.log(`${prompt} [${defaultValue}]: ${defaultValue}（自动）`)
    return defaultValue
  }
  const answer = await rl.question(`${prompt} [${defaultValue}]: `)
  return answer.trim() || defaultValue
}

async function askYesNo(rl, prompt, defaultValue = false) {
  if (options.yes) {
    console.log(`${prompt} [${defaultValue ? 'Y/n' : 'y/N'}]: ${defaultValue ? 'y' : 'n'}（自动）`)
    return defaultValue
  }
  const answer = (await rl.question(`${prompt} [${defaultValue ? 'Y/n' : 'y/N'}]: `)).trim().toLowerCase()
  if (!answer) return defaultValue
  return answer === 'y' || answer === 'yes'
}

async function canListen(port) {
  return new Promise((resolvePort) => {
    const server = createServer()
    server.once('error', () => resolvePort(false))
    server.once('listening', () => {
      server.close(() => resolvePort(true))
    })
    server.listen(port, '0.0.0.0')
  })
}

function expandHome(path) {
  if (path === '~') return homedir()
  if (path.startsWith('~/') || path.startsWith('~\\')) {
    return resolve(homedir(), path.slice(2))
  }
  return resolve(path)
}

function generateApiKey() {
  return randomBytes(32).toString('hex')
}

async function downloadText(url) {
  const response = await fetch(url)
  if (!response.ok) {
    throw new Error(`下载失败: ${url} (${response.status})`)
  }
  return response.text()
}

function enableApiKeyInConfig(config, apiKey) {
  if (!config.includes('security:')) {
    return `${config.trimEnd()}

security:
  enable_api_key: true
  api_key: "${apiKey}"
  signed_url_ttl_seconds: 3600
`
  }
  let inSecurity = false
  let hasEnableApiKey = false
  let hasApiKey = false
  const lines = config.split(/\r?\n/)
  const outputLines = []

  for (const line of lines) {
    if (/^security:\s*$/.test(line)) {
      inSecurity = true
      outputLines.push(line)
      continue
    }
    if (inSecurity && /^[A-Za-z_][A-Za-z0-9_]*:/.test(line)) {
      if (!hasEnableApiKey) outputLines.push('  enable_api_key: true')
      if (!hasApiKey) outputLines.push(`  api_key: "${apiKey}"`)
      inSecurity = false
    }
    if (inSecurity && /^\s*enable_api_key:/.test(line)) {
      outputLines.push('  enable_api_key: true')
      hasEnableApiKey = true
      continue
    }
    if (inSecurity && /^\s*api_key:/.test(line)) {
      outputLines.push(`  api_key: "${apiKey}"`)
      hasApiKey = true
      continue
    }
    outputLines.push(line)
  }

  if (inSecurity) {
    if (!hasEnableApiKey) outputLines.push('  enable_api_key: true')
    if (!hasApiKey) outputLines.push(`  api_key: "${apiKey}"`)
  }

  return outputLines.join('\n')
}

async function waitReady(port) {
  const url = `http://127.0.0.1:${port}/api/auth-status`
  for (let i = 0; i < 30; i += 1) {
    try {
      const response = await fetch(url, { signal: AbortSignal.timeout(2000) })
      if (response.ok) return true
    } catch {
      // 服务启动中，继续等待。
    }
    await new Promise((resolveWait) => setTimeout(resolveWait, 1000))
  }
  return false
}

async function main() {
  log('bililive-go 一次性安装（Docker 模式）')

  try {
    run('docker', ['--version'], { capture: true })
    run('docker', ['info'], { capture: true })
  } catch {
    fail('未检测到可用 Docker，或当前用户没有权限访问 Docker。请先安装并启动 Docker。')
    process.exit(1)
  }

  const rl = createInterface({ input, output })
  try {
    const installDir = expandHome(options.dir || await ask(rl, '安装目录（数据/视频/配置都放这里）', '~/bililive-go'))
    const portText = options.port || await ask(rl, 'Web UI 端口（主机侧）', '8080')
    const tag = options.image || await ask(rl, 'Docker 镜像 tag', 'latest')

    const port = Number.parseInt(portText, 10)
    if (!Number.isInteger(port) || port < 1 || port > 65535) {
      throw new Error(`端口非法: ${portText}`)
    }

    let enableApiKey = options.enableApiKey
    if (enableApiKey === undefined) {
      enableApiKey = await askYesNo(rl, '启用 API Key 鉴权？（公网部署建议开启）', false)
    }

    const apiKey = enableApiKey ? options.apiKey || generateApiKey() : ''

    if (!await canListen(port)) {
      warn(`端口 ${port} 可能已被占用`)
      if (!await askYesNo(rl, '继续使用此端口？', false)) {
        throw new Error('已取消')
      }
    }

    const existing = run('docker', ['ps', '-a', '--filter', `name=^/${CONTAINER_NAME}$`, '--format', '{{.Names}}'], {
      capture: true,
      allowFailure: true,
    }).stdout.trim()
    if (existing === CONTAINER_NAME) {
      warn(`已存在容器 ${CONTAINER_NAME}`)
      if (!await askYesNo(rl, `删除旧容器并重建？（数据保留在 ${installDir}）`, true)) {
        throw new Error('已取消，请手动处理旧容器后重跑')
      }
      log(`删除旧容器 ${CONTAINER_NAME}`)
      run('docker', ['rm', '-f', CONTAINER_NAME])
    }

    log(`创建安装目录: ${installDir}`)
    await mkdir(resolve(installDir, 'Videos'), { recursive: true })
    await mkdir(resolve(installDir, 'Data'), { recursive: true })

    const configFile = resolve(installDir, 'config.docker.yml')
    if (existsSync(configFile)) {
      warn(`配置文件已存在: ${configFile}`)
      if (await askYesNo(rl, '用最新模板覆盖？（旧文件会备份为 .bak）', false)) {
        await copyFile(configFile, `${configFile}.bak.${Date.now()}`)
        let config = await downloadText(`${RAW_BASE}/config.docker.yml`)
        if (enableApiKey) config = enableApiKeyInConfig(config, apiKey)
        await writeFile(configFile, config)
      }
    } else {
      log(`下载配置模板 -> ${configFile}`)
      let config = await downloadText(`${RAW_BASE}/config.docker.yml`)
      if (enableApiKey) config = enableApiKeyInConfig(config, apiKey)
      await writeFile(configFile, config)
    }

    log(`拉取镜像 ${DOCKER_IMAGE}:${tag}`)
    run('docker', ['pull', `${DOCKER_IMAGE}:${tag}`])

    log('启动容器')
    run('docker', [
      'run',
      '-d',
      '--name',
      CONTAINER_NAME,
      '--restart',
      'unless-stopped',
      '-p',
      `${port}:8080`,
      '-v',
      `${resolve(installDir, 'Videos')}:/srv/bililive`,
      '-v',
      `${resolve(installDir, 'Data')}:/var/lib/bililive`,
      '-v',
      `${configFile}:/etc/bililive-go/config.yml`,
      `${DOCKER_IMAGE}:${tag}`,
    ])

    log('等待服务就绪')
    const ready = await waitReady(port)
    console.log('')
    if (ready) {
      ok('bililive-go 已启动')
    } else {
      warn(`服务未在 30s 内响应，请用 docker logs ${CONTAINER_NAME} 排查`)
    }

    console.log(`
=== 安装完成 ===

  Web UI   : http://<服务器 IP>:${port}
  本机访问 : http://127.0.0.1:${port}
  数据目录 : ${resolve(installDir, 'Data')}
  视频目录 : ${resolve(installDir, 'Videos')}
  配置文件 : ${configFile}
${enableApiKey ? `  API Key  : ${apiKey}
` : '  API Key  : 未启用（公网访问建议开启）\n'}
  常用命令 :
    docker logs -f ${CONTAINER_NAME}
    docker restart ${CONTAINER_NAME}
    docker rm -f ${CONTAINER_NAME}
`)
  } finally {
    rl.close()
  }
}

main().catch((error) => {
  fail(error.message)
  process.exit(1)
})
