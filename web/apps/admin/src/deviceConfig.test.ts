import { beforeEach, describe, expect, it, vi } from 'vitest';

const request = vi.hoisted(() => vi.fn());
vi.mock('@classding/api', () => ({ api: request }));
import { saveDeviceConfigurations } from './deviceConfig';

describe('设备批量服务器配置', () => {
  beforeEach(() => request.mockReset());

  it('逐台保存，准确记录失败并继续处理后续设备', async () => {
    const devices = [{ id: 'one', name: '第一台' }, { id: 'two', name: '第二台' }, { id: 'three', name: '第三台' }];
    request.mockResolvedValueOnce({}).mockRejectedValueOnce(new Error('设备不存在')).mockResolvedValueOnce({});
    const result = await saveDeviceConfigurations(devices, 'https://new.example');
    expect(result.saved).toBe(2);
    expect(result.errors).toEqual([{ device: devices[1], message: '设备不存在' }]);
    expect(request.mock.calls.map(call => call[0])).toEqual(['/admin/devices/one/config', '/admin/devices/two/config', '/admin/devices/three/config']);
    for (const [, options] of request.mock.calls) expect(JSON.parse(options.body)).toEqual({ server_url: 'https://new.example' });
  });

  it('空地址表示取消计划，设备 ID 会安全编码', async () => {
    request.mockResolvedValue({});
    const result = await saveDeviceConfigurations([{ id: 'device/1' }], '');
    expect(result).toMatchObject({ saved: 1, errors: [], url: '' });
    expect(request.mock.calls[0][0]).toBe('/admin/devices/device%2F1/config');
    expect(JSON.parse(request.mock.calls[0][1].body)).toEqual({ server_url: '' });
  });
});
