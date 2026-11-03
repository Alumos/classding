import { spawn } from 'node:child_process';
import { existsSync, mkdtempSync, rmSync } from 'node:fs';
import { tmpdir } from 'node:os';
import path from 'node:path';
import { fileURLToPath } from 'node:url';
const root = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '..');
const e2e = process.argv.includes('--e2e');
const local = path.join(root, 'tools', 'go', 'bin', process.platform === 'win32' ? 'go.exe' : 'go');
const data = e2e ? mkdtempSync(path.join(tmpdir(), 'classding-e2e-')) : path.join(root, 'server', 'data');
const child = spawn(existsSync(local) ? local : 'go', ['run', './cmd/classding'], {
  cwd: path.join(root, 'server'),
  stdio: 'inherit',
  env: {
    ...process.env,
    DEV_SEED: '1',
    LISTEN_ADDR: e2e ? '127.0.0.1:18080' : '127.0.0.1:8080',
    DATA_DIR: data,
    GOCACHE: process.env.GOCACHE || path.join(tmpdir(), 'classding-go-cache'),
  },
});
for (const signal of ['SIGINT', 'SIGTERM']) process.on(signal, () => child.kill(signal));
child.on('error', (error) => {
  console.error(`无法启动服务，请安装 Go 1.27+：${error.message}`);
  process.exitCode = 1;
});
child.on('exit', (code) => {
  if (e2e) rmSync(data, { recursive: true, force: true });
  process.exit(code ?? 0);
});
