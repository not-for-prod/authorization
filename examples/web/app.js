'use strict';
const $ = id => document.getElementById(id);
const base = '/v1/tenants/main/auth';
let transaction = null, pending = null, selections = [], busy = false, result = null;
const label = value => value.replace(/^INTENT_/, '').toLowerCase().replaceAll('_', ' ');
const selected = () => pending?.selection || selections[Number($('method').value)];
function render() {
 const selection = selected();
 const method = selection?.methodId;
 $('status').textContent = transaction?.status?.replace(/^TRANSACTION_STATUS_/, '').toLowerCase() || 'No transaction';
 $('tx-id').textContent = transaction?.id || '—';
 $('attempt-id').textContent = pending?.attemptId || '—';
 $('challenge-id').textContent = pending?.challengeId || '—';
 $('create').disabled = busy || !String($('intent').value).startsWith('INTENT_');
 $('method').disabled = busy || !!pending || !selections.length;
 $('start').disabled = busy || !transaction || !!pending || !selection;
 $('refresh').disabled = busy || !transaction;
 $('email-field').hidden = method !== 'email_code';
 $('email').disabled = !!pending || busy;
 $('proof-fields').hidden = !pending;
 $('password-fields').hidden = method !== 'password';
 $('code-field').hidden = method === 'password';
 $('verify').disabled = busy || !pending;
 $('callback').disabled = busy || !pending?.challengeId || method !== 'email_code';
 $('hint').textContent = method === 'password' ? 'Sign in as alice with test-password. Passwords are verified using bcrypt.' : 'Mock code: 123456. Change it to test rejection.';
 $('instruction').textContent = result ? 'Completed. Create another transaction to test a new flow.' : pending ? 'The attempt is ready. Submit proof to continue.' : transaction ? 'Choose a method from the remaining policy.' : 'Create a transaction to see available methods.';
 $('success').hidden = !result;
 if (result) { $('principal').textContent = 'User: ' + (result.principal?.userId || '—'); $('result').textContent = JSON.stringify(result, null, 2); }
}
function accept(data) {
 if (data.transaction) {
  transaction = data.transaction;
  pending = transaction.pendingAttempt || null;
  selections = pending ? [pending.selection] : (transaction.availableMethods || []);
  $('method').replaceChildren(...selections.map((s, index) => {
   const option = document.createElement('option'); option.value = index;
   option.textContent = `${s.requirementId} · ${s.methodId === 'password' ? 'login + password' : s.methodId}${s.providerId ? ' / ' + s.providerId : ''}`;
   return option;
  }));
 }
 if (data.result) result = data.result;
}
async function request(path, body) {
 const started = performance.now();
 const response = await fetch(base + path, {method: body === undefined ? 'GET' : 'POST', credentials: 'same-origin', headers: {'Content-Type': 'application/json'}, ...(body === undefined ? {} : {body: JSON.stringify(body)})});
 const text = await response.text();
 let data; try { data = JSON.parse(text); } catch { data = {message: text || response.statusText}; }
 const item = document.createElement('li');
 item.textContent = `${response.status} · ${body === undefined ? 'GET' : 'POST'} ${path} · ${Math.round(performance.now() - started)} ms`;
 if (!response.ok) item.className = 'bad';
 if ($('history').querySelector('.muted')) $('history').replaceChildren();
 $('history').prepend(item);
 while ($('history').children.length > 30) $('history').lastChild.remove();
 // Tokens are shown only in the explicitly opened result panel.
 const display = structuredClone(data);
 if (display.result?.session) display.result.session.token = '[see session result]';
 $('response').textContent = JSON.stringify(display, null, 2);
 if (!response.ok) throw new Error(data.message || `HTTP ${response.status}`);
 return data;
}
async function run(action) {
 if (busy) return;
 busy = true; $('message').textContent = ''; $('message').className = ''; render();
 try { await action(); } catch (error) { $('message').textContent = error.message; $('message').className = 'error'; }
 finally { busy = false; render(); }
}
$('create').onclick = () => run(async () => {
 const data = await request('/transactions', {intent: $('intent').value});
 result = null; $('result').textContent = ''; accept(data);
});
$('method').onchange = render;
$('start').onclick = () => run(async () => {
 const selection = selected();
 const input = selection.methodId === 'email_code' ? {email: $('email').value.trim()} : {};
 accept(await request(`/transactions/${encodeURIComponent(transaction.id)}:startMethod`, {selection, input}));
});
function proof() { return selected().methodId === 'password' ? {login: $('username').value, password: $('password').value} : {code: $('code').value}; }
$('verify').onclick = () => run(async () => {
 accept(await request(`/transactions/${encodeURIComponent(transaction.id)}:verifyMethod`, {attemptId: pending.attemptId, ...(pending.challengeId ? {challengeId: pending.challengeId} : {}), input: proof()}));
});
$('callback').onclick = () => run(async () => {
 accept(await request(`/transactions/${encodeURIComponent(transaction.id)}/challenges/${encodeURIComponent(pending.challengeId)}:completeCallback`, {attemptId: pending.attemptId, input: proof()}));
});
$('refresh').onclick = () => run(async () => accept(await request(`/transactions/${encodeURIComponent(transaction.id)}`)));
$('clear').onclick = () => { $('history').replaceChildren(); $('response').textContent = 'History cleared.'; };
run(async () => {
 const data = await request('/policy');
 $('policy').textContent = JSON.stringify(data.intents || [], null, 2);
 $('intent').replaceChildren(...(data.intents || []).map(policy => {
  const option = document.createElement('option'); option.value = policy.intent; option.textContent = label(policy.intent); return option;
 }));
 if ([...$('intent').options].some(option => option.value === 'INTENT_SIGN_IN')) $('intent').value = 'INTENT_SIGN_IN';
});
