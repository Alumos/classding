import { api } from '@classding/api';

/** A failed request does not prevent the remaining devices from being updated. */
export async function saveDeviceConfigurations<T extends { id: string }>(devices: T[], url: string) {
  const errors: { device: T; message: string }[] = [];
  let saved = 0;
  for (const device of devices) {
    try {
      await api(`/admin/devices/${encodeURIComponent(device.id)}/config`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ server_url: url }),
      });
      saved++;
    } catch (error) {
      errors.push({
        device,
        message: error instanceof Error ? error.message : '无法连接服务，请检查网络或稍后重试。',
      });
    }
  }
  return { errors, saved, url };
}
