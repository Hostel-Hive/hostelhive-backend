// Live verification with Firebase ID tokens held only in process environment.
// Never logs authenticated URLs, tokens, SSE payloads or provider error bodies.
const mode = process.argv[2];
const token = process.env.HH_TEST_TOKEN;
const root = process.env.HH_TEST_DATABASE_URL?.replace(/\/+$/, '');
const base = process.env.HH_TEST_API_URL || 'http://127.0.0.1:18080';
const abort = new AbortController();
let timer;
let expectedExpiry;

class CheckFailure extends Error {}
function check(ok, message) { if (!ok) throw new CheckFailure(message); }
function uri(path, authenticated = true) {
 const url = new URL(`${root}/${path}.json`);
 if (authenticated) url.searchParams.set('auth', token);
 return url;
}
async function request(path, options = {}, authenticated = true) {
 return fetch(uri(path, authenticated), {...options, signal: abort.signal});
}
async function denied(path, options = {}, authenticated = true) {
 const response = await request(path, options, authenticated);
 await response.body?.cancel();
 check([401, 403].includes(response.status), `FAIL: expected denied request (${response.status}).`);
}
async function api(path, method, credential = token) {
 const response = await fetch(`${base}${path}`, {method, headers: {Authorization: `Bearer ${credential}`}, signal: abort.signal});
 check(response.ok, `FAIL: backend request returned HTTP ${response.status}.`);
 return response.json();
}

async function main() {
 check(['verify', 'deny', 'revoke', 'expiry'].includes(mode), 'Use verify, deny, revoke or expiry.');
 check(token && root, 'Set HH_TEST_TOKEN and HH_TEST_DATABASE_URL.');
 const parsed = new URL(root);
 check(parsed.protocol === 'https:' && !parsed.username && !parsed.password && !parsed.search && !parsed.hash &&
  (parsed.hostname.endsWith('.firebaseio.com') || parsed.hostname.endsWith('.firebasedatabase.app')),
  'Use the actual Firebase database HTTPS root URL.');
 timer = setTimeout(() => abort.abort(), 45000);
 if (mode === 'deny') {
  await denied('hostelhive/attendance');
  console.log('PASS: direct Firebase read denied.');
  return;
 }
 const response = await request('hostelhive/attendance');
 check(response.ok, `FAIL: projection read returned HTTP ${response.status}; obtain a fresh staff session.`);
 const projection = await response.json();
 check(projection?.summary && projection.summary.total === projection.summary.in + projection.summary.out + projection.summary.unknown,
  'FAIL: projection missing or counts do not add up.');
 if (mode === 'verify') {
  // If rules are accidentally permissive, this rewrites the identical snapshot,
  // rather than deleting or replacing attendance data with a test payload.
  await denied('hostelhive/attendance', {method: 'PUT', headers: {'Content-Type': 'application/json'}, body: JSON.stringify(projection)});
  await denied('hostelhive');
  await denied('hostelhive/staffAccess');
  await denied('hostelhive/attendance', {}, false);
  console.log(JSON.stringify(projection, null, 2));
  console.log('PASS: staff read, denied client write, broader reads and anonymous read.');
  return;
 }
 const me = await api('/api/v1/me', 'GET');
 check(me.role === 'warden' && me.is_active, 'Use an active dedicated test Warden for listener checks.');
 if (mode === 'revoke') {
  check(process.env.HH_TEST_ADMIN_TOKEN, 'Set HH_TEST_ADMIN_TOKEN.');
  check(process.env.HH_TEST_WARDEN_USER_ID === me.user_id, 'Warden user_id must match the signed-in test Warden.');
  const admin = await api('/api/v1/me', 'GET', process.env.HH_TEST_ADMIN_TOKEN);
  check(admin.role === 'admin' && admin.is_active, 'Use an active Admin token for deactivation.');
 } else {
  const expires = Date.parse(process.env.HH_TEST_EXPIRES_AT);
  check(Number.isFinite(expires) && expires > Date.now(), 'Set HH_TEST_EXPIRES_AT from a fresh Warden session.');
  clearTimeout(timer);
  expectedExpiry=expires;
  timer = setTimeout(() => abort.abort(), Math.max(45000, expires - Date.now() + 45000));
 }
 const stream = await request('hostelhive/attendance', {headers: {Accept: 'text/event-stream'}});
 check(stream.ok && stream.body, `FAIL: listener returned HTTP ${stream.status}.`);
 const reader = stream.body.getReader();
 const decoder = new TextDecoder();
 let buffered = '', started = false, cancelled = false;
 try {
  while (!cancelled) {
   const {value, done} = await reader.read();
   check(!done, 'FAIL: stream closed without a cancellation event.');
   buffered += decoder.decode(value, {stream: true}).replace(/\r\n/g, '\n');
   let boundary;
   while ((boundary = buffered.indexOf('\n\n')) >= 0) {
    const frame = buffered.slice(0, boundary);
    buffered = buffered.slice(boundary + 2);
    const event = frame.split('\n').find(line => line.startsWith('event:'))?.slice(6).trim();
    if (!started && ['put', 'patch'].includes(event)) {
     started = true;
     console.log('Listener established.');
     if (mode === 'revoke') {
      await api(`/api/v1/users/${encodeURIComponent(me.user_id)}/deactivate`, 'POST', process.env.HH_TEST_ADMIN_TOKEN);
      console.log('Test Warden deactivated; waiting for listener cancellation.');
     } else console.log('Waiting for five-minute grant expiry. Do not renew this session.');
    }
    if (['cancel', 'auth_revoked'].includes(event)) {cancelled = true; break;}
   }
  }
 } finally {await reader.cancel();}
 check(started, 'FAIL: listener was denied before being established.');
 if(mode==='expiry') check(Date.now()>=expectedExpiry-1000,'FAIL: listener cancelled before grant expiry. Obtain a fresh Firebase token and session.');
 await denied('hostelhive/attendance');
 console.log(`PASS: ${mode} cancelled the established listener and denied a fresh read.`);
}

main().catch(error => {
 // Exceptions from fetch can contain authenticated URLs; do not expose them.
 console.error(error instanceof CheckFailure ? error.message : 'FAIL: live RTDB check did not pass. Check session expiry, setup, permissions and backend logs; no secrets were printed.');
 process.exitCode = 1;
}).finally(() => {clearTimeout(timer); abort.abort();});
