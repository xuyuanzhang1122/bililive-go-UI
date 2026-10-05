const { test } = require('node:test');
const assert = require('node:assert/strict');
const fs = require('node:fs');
const { execFileSync, spawnSync } = require('node:child_process');
const path = require('node:path');
const root = path.resolve(__dirname, '..');

test('sun 发布预演不写工作区，保留精确特殊标签', () => {
  const before = execFileSync('git', ['diff'], { cwd: root, encoding: 'utf8' });
  const result = spawnSync('bash', ['scripts/release.sh', '--dry-run', 'sun'], { cwd: root, encoding: 'utf8' });
  assert.equal(result.status, 0, result.stdout + result.stderr);
  assert.match(result.stdout, /sun/);
  assert.doesNotMatch(result.stdout, /vsun/);
  assert.equal(execFileSync('git', ['diff'], { cwd: root, encoding: 'utf8' }), before);
});

test('新装模板默认录后准备，保留原文件', () => {
  for (const file of ['config.yml', 'config.docker.yml']) {
    const yaml = fs.readFileSync(path.join(root, file), 'utf8');
    assert.match(yaml, /on_record_finished:\n(?:[^\n]*\n)*?\s+prepare_playback: true/, file);
    assert.match(yaml, /convert_to_mp4: false/, file);
    assert.match(yaml, /delete_flv_after_convert: false/, file);
  }
});

test('指定 sun 不命中旧镜像，下载路径仍为 sun', () => {
  const script = fs.readFileSync(path.join(root, 'scripts/install.sh'), 'utf8').split('# ---- 解析参数 ----')[0];
  const run = script + `\nTAG=sun\nOS=linux\nGOARCH=amd64\nCATALOG_FILE=\nMIRROR_URL=\nresolve_binary_download\nprintf '%s\\n' "$DOWNLOAD_URL"\n`;
  const result = spawnSync('bash', ['-c', run], { encoding: 'utf8' });
  assert.equal(result.status, 0, result.stderr);
  assert.equal(result.stdout.trim(), 'https://github.com/xuyuanzhang1122/bililive-go-UI/releases/download/sun/bililive-linux-amd64.tar.gz');
});
