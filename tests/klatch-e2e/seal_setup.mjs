#!/usr/bin/env node
// Sandbox-only seal setup for klatch-e2e (Node >= 22, stdlib only).
//
// The participant helper unlocks an existing account seal but has no verb to
// create one, so the e2e harness seals its fresh sandbox accounts here, with
// the same representation the SDK's settings.setSeal writes: PBKDF2-SHA1
// (1024 rounds) of the seal password over a random salt keys AES-256-CBC
// encryption of the base64 PKCS#1 DER private key; the public key is base64
// SPKI DER.
//
//   node seal_setup.mjs --session FILE --input FILE
//
// Input is a private JSON file {"sealPassword": "..."}. An account that is
// already sealed is never overwritten. Output follows the helper envelope:
// stdout {"ok":true,"result":{...}} or stderr {"ok":false,"error":{"code"}}.
// Neither the token nor the seal password is ever printed.
import { readFile } from 'node:fs/promises';
import { createCipheriv, generateKeyPairSync, pbkdf2Sync, randomBytes } from 'node:crypto';

class Failure extends Error {}

function args(argv) {
  const out = {};
  while (argv.length) {
    const flag = argv.shift();
    if (!['--session', '--input'].includes(flag) || !argv.length || out[flag]) throw new Failure('USAGE');
    out[flag] = argv.shift();
  }
  if (!out['--session'] || !out['--input']) throw new Failure('USAGE');
  return out;
}

async function request(session, path, method = 'GET', body) {
  const url = new URL(session.server.replace(/\/+$/, '') + path);
  url.searchParams.set('agent', session.token);
  let response;
  try {
    response = await fetch(url, { method, redirect: 'error', signal: AbortSignal.timeout(30000),
      body: body === undefined ? undefined : JSON.stringify(body) });
  } catch { throw new Failure('NETWORK_ERROR'); }
  if (!response.ok) throw new Failure(response.status === 401 ? 'AUTH_REQUIRED' : 'HTTP_' + response.status);
  const text = await response.text();
  return text ? JSON.parse(text) : null;
}

async function main() {
  const opts = args(process.argv.slice(2));
  const session = JSON.parse(await readFile(opts['--session'], 'utf8'));
  const input = JSON.parse(await readFile(opts['--input'], 'utf8'));
  if (!session.server || !session.token) throw new Failure('INVALID_SESSION');
  if (typeof input.sealPassword !== 'string' || !input.sealPassword) throw new Failure('INVALID_INPUT');
  const status = await request(session, '/account/status');
  if (status?.seal?.publicKey) return { sealed: true, created: false, publicKey: status.seal.publicKey };
  const { publicKey, privateKey } = generateKeyPairSync('rsa', { modulusLength: 2048,
    publicKeyEncoding: { type: 'spki', format: 'der' }, privateKeyEncoding: { type: 'pkcs1', format: 'der' } });
  const salt = randomBytes(16), iv = randomBytes(16);
  const key = pbkdf2Sync(input.sealPassword, salt, 1024, 32, 'sha1');
  const cipher = createCipheriv('aes-256-cbc', key, iv);
  const privateKeyEncrypted = Buffer.concat([cipher.update(privateKey.toString('base64'), 'utf8'), cipher.final()]).toString('base64');
  const seal = { passwordSalt: salt.toString('hex'), privateKeyIv: iv.toString('hex'), privateKeyEncrypted,
    publicKey: publicKey.toString('base64') };
  await request(session, '/account/seal', 'PUT', seal);
  return { sealed: true, created: true, publicKey: seal.publicKey };
}

main().then(result => {
  process.stdout.write(JSON.stringify({ ok: true, result }) + '\n');
}, error => {
  const code = error instanceof Failure ? error.message : error instanceof SyntaxError ? 'INVALID_JSON' : 'OPERATION_FAILED';
  process.stderr.write(JSON.stringify({ ok: false, error: { code } }) + '\n');
  process.exitCode = code === 'USAGE' || code === 'INVALID_INPUT' || code === 'INVALID_JSON' ? 2 : 1;
});
