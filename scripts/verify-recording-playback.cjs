// 实际生产网页验收：鉴权由临时文件提供，输出不包含管理 Key 或播放令牌。
// 用法：NODE_PATH=<Playwright 依赖目录> node scripts/verify-recording-playback.cjs <服务器地址> <临时鉴权 JSON> <证据目录> <文件名片段...>
const fs = require('node:fs');
const path = require('node:path');
const assert = require('node:assert/strict');
const { chromium } = require('playwright');

(async () => {
  const [baseURL, authFile, evidenceDir, ...fragments] = process.argv.slice(2);
  assert(baseURL && authFile && evidenceDir && fragments.length, '缺少验收参数');
  const { key } = JSON.parse(fs.readFileSync(authFile, 'utf8'));
  fs.mkdirSync(evidenceDir, { recursive: true });
  const browser = await chromium.launch({ channel: 'chrome', headless: true });
  try {
    const context = await browser.newContext({ viewport: { width: 1440, height: 900 } });
    // 只隔离验收产生的观看进度写入；媒体、会话及历史读取均请求真实服务器。
    await context.route('**/api/history', route => route.request().method() === 'POST'
      ? route.fulfill({ status: 200, contentType: 'application/json', body: '{"err_no":0}' })
      : route.continue());
    await context.addInitScript(value => localStorage.setItem('bililive_api_key', value), key);
    const response = await context.request.get(`${baseURL}/api/v2/recordings?limit=100`, { headers: { Authorization: `Bearer ${key}` } });
    assert.equal(response.status(), 200);
    const rows = (await response.json()).data.items;
    const results = [];
    for (const fragment of fragments) {
      const matches = rows.filter(row => row.path.includes(fragment));
      assert.equal(matches.length, 1, `文件名片段须唯一：${fragment}`);
      const row = matches[0], page = await context.newPage();
      const errors = [];
      page.on('pageerror', error => errors.push(error.message));
      const query = new URLSearchParams({ room: row.room, play: row.path, name: row.path.split('/').pop() });
      await page.goto(`${baseURL}/#/videoLibrary?${query}`, { waitUntil: 'domcontentloaded' });
      await page.waitForFunction(() => {
        const video = document.querySelector('video');
        return video && video.readyState >= 2 && Number.isFinite(video.duration) && video.duration > 0;
      }, null, { timeout: 30 * 60 * 1000 });
      const duration = await page.locator('video').evaluate(video => video.duration);
      const samples = [];
      for (const position of [2, duration / 2, Math.max(2, duration - 4)]) {
        await page.locator('video').evaluate(async (video, position) => {
          video.pause(); video.muted = true; video.currentTime = position;
          await video.play();
        }, position);
        await page.waitForFunction(position => {
          const video = document.querySelector('video');
          return video && !video.error && video.currentTime >= position + 1 && video.currentTime < position + 10 && video.readyState >= 2;
        }, position, { timeout: 30000 });
        const sample = await page.locator('video').evaluate(video => ({
          current_time: video.currentTime, ready_state: video.readyState,
          width: video.videoWidth, height: video.videoHeight,
          decoded_frames: video.getVideoPlaybackQuality().totalVideoFrames,
          error: video.error && { code: video.error.code, message: video.error.message },
        }));
        assert(sample.decoded_frames > 0 && sample.width > 0 && sample.height > 0 && !sample.error, '须实际解码出画面');
        samples.push(sample);
      }
      const body = await page.locator('body').innerText();
      assert(!/Invalid NAL|single-track|\[flv @|没有匹配的已准备资产/.test(body), '播放器仍显示内部诊断');
      assert.equal(errors.length, 0, '网页存在未处理异常');
      await page.screenshot({ path: path.join(evidenceDir, `playback-${fragment}.png`) });
      results.push({ recording_id: row.recording_id, source_version: row.source_version, path: row.path, duration, samples, page_errors: errors });
      fs.writeFileSync(path.join(evidenceDir, 'browser-playback.json'), JSON.stringify(results, null, 2));
      console.log(JSON.stringify({ fragment, duration, samples, page_errors: errors }));
      await page.close();
    }
  } finally {
    await browser.close();
  }
})().catch(error => { console.error(error.message); process.exitCode = 1; });
