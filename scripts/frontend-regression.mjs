#!/usr/bin/env node

const baseURL = process.env.WERK_REGRESSION_URL || 'http://127.0.0.1:3000';
const cdpURL = process.env.WERK_CDP_URL || 'http://127.0.0.1:9224';
let targetList;
try {
  targetList = await (await fetch(`${cdpURL}/json/list`)).json();
} catch (error) {
  console.error(`FAIL frontend.browser: Dev-Chrome ist unter ${cdpURL} nicht erreichbar`);
  console.error(`     ${error.message}`);
  process.exit(2);
}
const target = targetList.find((item) => item.type === 'page');
if (!target) throw new Error(`Kein Chrome-Tab am ${cdpURL} gefunden`);

const socket = new WebSocket(target.webSocketDebuggerUrl);
await new Promise((resolve, reject) => {
  socket.addEventListener('open', resolve, { once: true });
  socket.addEventListener('error', reject, { once: true });
});
let nextID = 0;
const pending = new Map();
socket.addEventListener('message', (event) => {
  const message = JSON.parse(event.data);
  if (!message.id) return;
  const waiter = pending.get(message.id);
  if (!waiter) return;
  pending.delete(message.id);
  message.error ? waiter.reject(new Error(message.error.message)) : waiter.resolve(message.result);
});
const call = (method, params = {}) => new Promise((resolve, reject) => {
  const id = ++nextID;
  pending.set(id, { resolve, reject });
  socket.send(JSON.stringify({ id, method, params }));
});
const evaluate = async (expression) => (await call('Runtime.evaluate', {
  expression, awaitPromise: true, returnByValue: true,
})).result?.value;

await call('Page.enable');
await call('Runtime.enable');
await call('Page.navigate', { url: `${baseURL}/admin` });
let state;
for (let attempt = 0; attempt < 100; attempt += 1) {
  state = await evaluate(`({ready: document.readyState, title: document.title, url: location.href})`);
  if (state?.ready === 'complete') break;
  await new Promise((resolve) => setTimeout(resolve, 100));
}
const result = await evaluate(`(() => ({
  title: document.title,
  hasAdminShell: Boolean(document.querySelector('.admin-workspace[data-authenticated-page="/admin"]')),
  hasGlobalNavigation: Boolean(document.querySelector('[data-global-navigation]')),
  hasUsersView: Boolean(document.querySelector('[data-admin-view="users"]')),
  hasUserDialog: Boolean(document.querySelector('[data-user-dialog]')),
  dialogCount: document.querySelectorAll('dialog.admin-dialog').length,
  hasAdminScript: Boolean(document.querySelector('script[src="/admin.js"]')),
}))()`);
const failures = Object.entries(result || {}).filter(([key, value]) =>
  (key === 'dialogCount' ? value < 8 : !value));
for (const [key, value] of failures) console.error(`FAIL frontend.${key}: ${String(value)}`);
if (failures.length > 0) process.exitCode = 1;
else console.log(`PASS frontend browser contract (${result.dialogCount} dialogs)`);
socket.close();
