#!/usr/bin/env node

import { createServer } from 'node:net'
import { createInterface } from 'node:readline/promises'
import { stdin as input, stdout as output } from 'node:process'
import { chmod, copyFile, mkdir, mkdtemp, readdir, rm, writeFile } from 'node:fs/promises'
import { existsSync } from 'node:fs'
import { homedir, platform, tmpdir } from 'node:os'
import { join, resolve } from 'node:path'
import { spawnSync } from 'node:child_process'
import { randomBytes } from 'node:crypto'

const REPO = 'xuyuanzhang1122/bililive-go-UI'
const RAW_BASE = `https://raw.githubusercontent.com/${REPO}/main`
const DOCKER_IMAGE = 'xuniubi/bililive-go'
const CONTAINER_NAME = 'bililive-go'

const args = process.argv.slice(2)

const options = {
  mode: 'binary',
  dir: '',
  port: '',
  image: '',
  version: '',
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
  npx bililive-go-ui install --docker
  npx bililive-go-ui install --source
  npx bililive-go-ui install --dir ~/bililive-go --port 8080 --enable-api-key

参数:
  install              安装 bililive-go（可省略）
  --binary             安装 GitHub Release 二进制（默认，不需要 Docker）
  --source             从源码构建并安装（需要 Git、Go、Node.js、Make）
  --docker             使用 Docker 安装并启动容器
  --dir PATH           安装目录，默认 ~/bililive-go
  --port N             Web UI 端口，默认 8080（Docker 模式为主机端口）
  --image TAG          Docker 镜像 tag，默认 latest（仅 --docker）
  --version TAG        GitHub Release tag 或 Docker tag
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
    case '--binary':
      options.mode = 'binary'
      break
    case '--source':
      options.mode = 'source'
      break
    case '--docker':
      options.mode = 'docker'
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
      options.image = takeValue(i, arg)
      i += 1
      break
    case '--version':
      options.version = takeValue(i, arg)
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
    cwd: runOptions.cwd,
  })
  if (result.error) {
    throw result.error
  }
  if (result.status !== 0 && !runOptions.allowFailure) {
    throw new Error(`${command} ${commandArgs.join(' ')} 执行失败`)
  }
  return result
}

function commandExists(command) {
  const checker = platform() === 'win32' ? 'where' : 'sh'
  const checkerArgs = platform() === 'win32' ? [command] : ['-c', `command -v ${command}`]
  return run(checker, checkerArgs, { capture: true, allowFailure: true }).status === 0
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

async function downloadFile(url, target) {
  const response = await fetch(url)
  if (!response.ok) {
    throw new Error(`下载失败: ${url} (${response.status})`)
  }
  const body = Buffer.from(await response.arrayBuffer())
  await writeFile(target, body)
}

async function fetchLatestRelease() {
  const response = await fetch(`https://api.github.com/repos/${REPO}/releases/latest`, {
    headers: { 'User-Agent': 'bililive-go-ui-installer' },
  })
  if (response.status === 404) {
    return null
  }
  if (!response.ok) {
    throw new Error(`查询 GitHub Release 失败 (${response.status})`)
  }
  return response.json()
}

function getTarget() {
  const os = platform()
  const cpu = process.arch
  const platformName = {
    win32: 'windows',
    linux: 'linux',
    darwin: 'darwin',
  }[os]
  const archName = {
    x64: 'amd64',
    arm64: 'arm64',
    arm: 'arm',
  }[cpu]

  if (!platformName || !archName) {
    throw new Error(`暂不支持当前系统: ${os}/${cpu}`)
  }
  return { platformName, archName }
}

function getAssetName() {
  const { platformName, archName } = getTarget()
  const suffix = platformName === 'windows' ? 'zip' : 'tar.gz'
  return `bililive-${platformName}-${archName}.${suffix}`
}

async function findBinary(dir) {
  const entries = await readdir(dir, { withFileTypes: true })
  for (const entry of entries) {
    const fullPath = join(dir, entry.name)
    if (entry.isDirectory()) {
      const nested = await findBinary(fullPath)
      if (nested) return nested
      continue
    }
    const name = entry.name.toLowerCase()
    if (
      name.startsWith('bililive-') &&
      !name.endsWith('.zip') &&
      !name.endsWith('.tar.gz') &&
      !name.endsWith('.7z') &&
      !name.endsWith('.yml') &&
      !name.endsWith('.yaml')
    ) {
      return fullPath
    }
  }
  return ''
}

async function extractArchive(archive, targetDir) {
  await mkdir(targetDir, { recursive: true })
  if (archive.endsWith('.zip')) {
    if (platform() === 'win32') {
      run('powershell', ['-NoProfile', '-Command', 'Expand-Archive', '-LiteralPath', archive, '-DestinationPath', targetDir, '-Force'])
      return
    }
    if (!commandExists('unzip')) {
      throw new Error('解压 zip 需要 unzip，请安装 unzip 后重试')
    }
    run('unzip', ['-q', archive, '-d', targetDir])
    return
  }
  run('tar', ['-xzf', archive, '-C', targetDir])
}

function yamlPath(path) {
  return path.replaceAll('\\', '/')
}

function configurePortableConfig(config, installDir, port) {
  const videosPath = yamlPath(resolve(installDir, 'Videos'))
  const dataPath = yamlPath(resolve(installDir, 'Data'))
  return config
    .replace(/^(\s*bind:\s*).*/m, `$1:${port}`)
    .replace(/^out_put_path:\s*.*/m, `out_put_path: ${videosPath}`)
    .replace(/^app_data_path:\s*.*/m, `app_data_path: ${dataPath}`)
}

async function writeConfig(configFile, installDir, port, enableApiKey, apiKey) {
  if (existsSync(configFile)) {
    warn(`配置文件已存在: ${configFile}`)
    return
  }
  log(`下载配置模板 -> ${configFile}`)
  let config = await downloadText(`${RAW_BASE}/config.yml`)
  config = configurePortableConfig(config, installDir, port)
  if (enableApiKey) config = enableApiKeyInConfig(config, apiKey)
  await writeFile(configFile, config)
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

async function dockerInstall() {
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
    const tag = options.image || options.version || await ask(rl, 'Docker 镜像 tag', 'latest')

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

async function binaryInstall() {
  log('bililive-go 一次性安装（二进制模式，无需 Docker）')

  const rl = createInterface({ input, output })
  try {
    const installDir = expandHome(options.dir || await ask(rl, '安装目录', '~/bililive-go'))
    const portText = options.port || await ask(rl, 'Web UI 端口', '8080')
    const port = Number.parseInt(portText, 10)
    if (!Number.isInteger(port) || port < 1 || port > 65535) {
      throw new Error(`端口非法: ${portText}`)
    }

    let enableApiKey = options.enableApiKey
    if (enableApiKey === undefined) {
      enableApiKey = await askYesNo(rl, '启用 API Key 鉴权？（公网部署建议开启）', false)
    }
    const apiKey = enableApiKey ? options.apiKey || generateApiKey() : ''

    await mkdir(resolve(installDir, 'Videos'), { recursive: true })
    await mkdir(resolve(installDir, 'Data'), { recursive: true })

    const release = options.version
      ? {
          tag_name: options.version,
          assets: [{
            name: getAssetName(),
            browser_download_url: `https://github.com/${REPO}/releases/download/${options.version}/${getAssetName()}`,
          }],
        }
      : await fetchLatestRelease()

    if (!release) {
      warn('当前仓库还没有 GitHub Release，无法下载预编译二进制')
      await sourceInstall({ installDir, port, enableApiKey, apiKey, skipPrompt: true })
      return
    }

    const assetName = getAssetName()
    const asset = release.assets.find((item) => item.name === assetName)
    if (!asset) {
      throw new Error(`Release ${release.tag_name} 中没有当前系统资产: ${assetName}`)
    }

    const workDir = await mkdtemp(join(tmpdir(), 'bililive-go-install-'))
    try {
      const archive = join(workDir, assetName)
      log(`下载 ${release.tag_name}: ${asset.browser_download_url}`)
      await downloadFile(asset.browser_download_url, archive)

      const extractDir = join(workDir, 'extract')
      log('解压安装包')
      await extractArchive(archive, extractDir)

      const binary = await findBinary(extractDir)
      if (!binary) {
        throw new Error(`安装包内未找到 bililive 二进制: ${assetName}`)
      }

      const targetBinary = resolve(installDir, platform() === 'win32' ? 'bililive-go.exe' : 'bililive-go')
      await copyFile(binary, targetBinary)
      if (platform() !== 'win32') {
        await chmod(targetBinary, 0o755)
      }

      const configFile = resolve(installDir, 'config.yml')
      await writeConfig(configFile, installDir, port, enableApiKey, apiKey)
      await writeRunScript(installDir, targetBinary, configFile, port)

      printBinaryDone(installDir, targetBinary, configFile, port, enableApiKey, apiKey)
    } finally {
      await rm(workDir, { recursive: true, force: true })
    }
  } finally {
    rl.close()
  }
}

async function sourceInstall(prepared = {}) {
  log('bililive-go 一次性安装（源码构建模式，无需 Docker）')

  if (!commandExists('git')) throw new Error('源码构建需要 Git，请先安装 Git')
  if (!commandExists('go')) throw new Error('源码构建需要 Go，请先安装 Go 1.25+')
  if (!commandExists('make')) throw new Error('源码构建需要 GNU Make，请先安装 Make')

  const rl = prepared.skipPrompt ? null : createInterface({ input, output })
  try {
    const installDir = prepared.installDir || expandHome(options.dir || await ask(rl, '安装目录', '~/bililive-go'))
    const portText = prepared.port ? String(prepared.port) : options.port || await ask(rl, 'Web UI 端口', '8080')
    const port = Number.parseInt(portText, 10)
    if (!Number.isInteger(port) || port < 1 || port > 65535) {
      throw new Error(`端口非法: ${portText}`)
    }

    let enableApiKey = prepared.enableApiKey ?? options.enableApiKey
    if (enableApiKey === undefined) {
      enableApiKey = await askYesNo(rl, '启用 API Key 鉴权？（公网部署建议开启）', false)
    }
    const apiKey = prepared.apiKey || (enableApiKey ? options.apiKey || generateApiKey() : '')

    await mkdir(resolve(installDir, 'Videos'), { recursive: true })
    await mkdir(resolve(installDir, 'Data'), { recursive: true })

    const workDir = await mkdtemp(join(tmpdir(), 'bililive-go-src-'))
    try {
      log('克隆源码')
      run('git', ['clone', '--depth', '1', `https://github.com/${REPO}.git`, workDir])

      log('构建前端和后端')
      run('make', ['build-web', 'dev'], { cwd: workDir })

      const binDir = join(workDir, 'bin')
      const binary = await findBinary(binDir)
      if (!binary) {
        throw new Error('源码构建完成后未找到二进制产物')
      }

      const targetBinary = resolve(installDir, platform() === 'win32' ? 'bililive-go.exe' : 'bililive-go')
      await copyFile(binary, targetBinary)
      if (platform() !== 'win32') {
        await chmod(targetBinary, 0o755)
      }

      const configFile = resolve(installDir, 'config.yml')
      await writeConfig(configFile, installDir, port, enableApiKey, apiKey)
      await writeRunScript(installDir, targetBinary, configFile, port)

      printBinaryDone(installDir, targetBinary, configFile, port, enableApiKey, apiKey)
    } finally {
      await rm(workDir, { recursive: true, force: true })
    }
  } finally {
    if (rl) rl.close()
  }
}

async function writeRunScript(installDir, binary, configFile, port) {
  if (platform() === 'win32') {
    const script = resolve(installDir, 'start-bililive-go.ps1')
    await writeFile(script, `Set-Location -LiteralPath "${installDir.replaceAll('"', '`"')}"
& "${binary.replaceAll('"', '`"')}" -c "${configFile.replaceAll('"', '`"')}"
`)
    return
  }
  const script = resolve(installDir, 'start-bililive-go.sh')
  await writeFile(script, `#!/usr/bin/env sh
cd "${installDir.replaceAll('"', '\\"')}"
exec "${binary.replaceAll('"', '\\"')}" -c "${configFile.replaceAll('"', '\\"')}"
`)
  await chmod(script, 0o755)
}

function printBinaryDone(installDir, binary, configFile, port, enableApiKey, apiKey) {
  const startCommand = platform() === 'win32'
    ? `powershell -ExecutionPolicy Bypass -File "${resolve(installDir, 'start-bililive-go.ps1')}"`
    : `"${resolve(installDir, 'start-bililive-go.sh')}"`
  console.log(`
=== 安装完成 ===

  启动命令 : ${startCommand}
  Web UI   : http://127.0.0.1:${port}
  程序文件 : ${binary}
  配置文件 : ${configFile}
  数据目录 : ${resolve(installDir, 'Data')}
  视频目录 : ${resolve(installDir, 'Videos')}
${enableApiKey ? `  API Key  : ${apiKey}
` : '  API Key  : 未启用（公网访问建议开启）\n'}`)
}

async function main() {
  if (options.mode === 'docker') {
    await dockerInstall()
    return
  }
  if (options.mode === 'source') {
    await sourceInstall()
    return
  }
  await binaryInstall()
}

main().catch((error) => {
  fail(error.message)
  process.exit(1)
})
