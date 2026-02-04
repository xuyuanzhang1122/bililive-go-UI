import { test, expect, APIRequestContext } from '@playwright/test';

/**
 * Bililive-Go StreamPreference 流偏好功能测试
 *
 * 测试流偏好配置的 API 和选择逻辑
 * 依赖 osrp-stream-tester 提供多种可用流
 */

// 流信息接口
interface StreamInfo {
  url: string;
  format: string;
  quality: string;
  codec: string;
  attributes?: Record<string, string>;
}

// 直播间信息接口（/api/lives 返回格式）
interface LiveInfo {
  id: string;  // live_id
  live_url: string;  // room URL
  platform_cn_name: string;
  host_name: string;
  room_name: string;
  status: boolean;
  listening: boolean;
  recording: boolean;
  initializing: boolean;
  audio_only: boolean;
  nick_name?: string;
  available_streams?: StreamInfo[];
}

test.describe('osrp-stream-tester 多流能力测试', () => {
  test('可以获取多种可用流', async ({ request }) => {
    // 直接请求 osrp-stream-tester 的可用流 API
    const response = await request.get('http://127.0.0.1:8888/api/streams/test/available');
    expect(response.ok()).toBeTruthy();

    const streams: StreamInfo[] = await response.json();
    expect(Array.isArray(streams)).toBe(true);
    expect(streams.length).toBeGreaterThan(1);

    // 验证流包含必要字段
    const firstStream = streams[0];
    expect(firstStream).toHaveProperty('url');
    expect(firstStream).toHaveProperty('format');
    expect(firstStream).toHaveProperty('quality');
    expect(firstStream).toHaveProperty('codec');

    // 验证包含 attributes 字段（用于流偏好选择）
    expect(firstStream).toHaveProperty('attributes');
    expect(firstStream.attributes).toHaveProperty('format');
    expect(firstStream.attributes).toHaveProperty('codec');

    console.log(`获取到 ${streams.length} 个可用流:`, streams.map((s: StreamInfo) => `${s.quality} ${s.format} ${s.codec}`));
  });

  test('多个不同分辨率和编码的流', async ({ request }) => {
    const response = await request.get('http://127.0.0.1:8888/api/streams/test/available');
    const streams: StreamInfo[] = await response.json();

    // 验证有不同分辨率
    const qualities = new Set(streams.map((s: StreamInfo) => s.quality));
    expect(qualities.size).toBeGreaterThan(1);
    console.log('可用分辨率:', [...qualities]);

    // 验证有不同编码
    const codecs = new Set(streams.map((s: StreamInfo) => s.codec));
    expect(codecs.size).toBeGreaterThan(1);
    console.log('可用编码:', [...codecs]);

    // 验证有不同格式
    const formats = new Set(streams.map((s: StreamInfo) => s.format));
    expect(formats.size).toBeGreaterThan(1);
    console.log('可用格式:', [...formats]);
  });
});

test.describe('流偏好 API 测试', () => {
  const testRoomUrl = 'http://127.0.0.1:8888/live/test';

  // 清理所有测试房间
  async function cleanupTestRooms(request: APIRequestContext) {
    const response = await request.get('/api/lives');
    const lives: LiveInfo[] = await response.json();
    for (const live of lives) {
      if (live.live_url?.includes('127.0.0.1:8888')) {
        await request.delete(`/api/lives/${live.id}`);
      }
    }
  }

  test.beforeEach(async ({ request }) => {
    await cleanupTestRooms(request);
  });

  test.afterEach(async ({ request }) => {
    await cleanupTestRooms(request);
  });

  test('添加测试房间', async ({ request }) => {
    // 添加房间
    const addResponse = await request.post('/api/lives', {
      data: [{ url: testRoomUrl, listen: true }]
    });
    expect(addResponse.ok()).toBeTruthy();

    // 验证添加成功
    const livesResponse = await request.get('/api/lives');
    const lives: LiveInfo[] = await livesResponse.json();
    const testRoom = lives.find((l: LiveInfo) => l.live_url?.includes('127.0.0.1:8888'));
    expect(testRoom).toBeDefined();
    expect(testRoom?.id).toBeDefined();

    console.log('测试房间 id:', testRoom?.id);
  });

  test('可以更新房间的流偏好', async ({ request }) => {
    // 先添加房间
    await request.post('/api/lives', {
      data: [{ url: testRoomUrl, listen: false }]
    });

    // 等待房间添加完成
    await new Promise(resolve => setTimeout(resolve, 500));

    // 获取 live_id
    const livesResponse = await request.get('/api/lives');
    const lives: LiveInfo[] = await livesResponse.json();
    const testRoom = lives.find((l: LiveInfo) => l.live_url?.includes('127.0.0.1:8888'));
    expect(testRoom).toBeDefined();
    const liveId = testRoom?.id;
    expect(liveId).toBeDefined();

    // 使用 switchStream API 更新流偏好
    const updateResponse = await request.post(
      `/api/lives/${liveId}/switchStream`,
      {
        data: {
          quality: '1080p',
          attributes: {
            format: 'flv',
            codec: 'hevc'
          }
        }
      }
    );
    expect(updateResponse.ok()).toBeTruthy();

    const updateResult = await updateResponse.json();
    expect(updateResult.success).toBe(true);
    console.log('流偏好更新结果:', updateResult);
  });
});

test.describe('全局流偏好配置测试', () => {
  test('配置 API 包含流偏好字段', async ({ request }) => {
    const response = await request.get('/api/config');
    expect(response.ok()).toBeTruthy();

    const config = await response.json();
    // 全局配置应该有 stream_preference 字段
    expect(config).toHaveProperty('stream_preference');
  });
});

test.describe('流偏好 UI 测试', () => {
  test('首页可以正常加载', async ({ page }) => {
    await page.goto('/');
    await page.waitForLoadState('domcontentloaded');

    // 验证页面标题
    await expect(page.getByRole('heading', { name: 'Bililive-go' })).toBeVisible();
  });
});
