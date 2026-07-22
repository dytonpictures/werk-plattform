const statusText = document.querySelector('[data-platform-status]');
const statusDot = document.querySelector('[data-status-dot]');
const loginForm = document.querySelector('[data-login-form]');
const loginNotice = document.querySelector('[data-login-notice]');
const submitButton = loginForm?.querySelector('button[type="submit"]');
const logoutButton = document.querySelector('[data-logout]');

function csrfToken() {
  const prefix = 'werk_csrf=';
  const value = document.cookie.split(';').map((part) => part.trim()).find((part) => part.startsWith(prefix));
  return value ? decodeURIComponent(value.slice(prefix.length)) : '';
}

async function updatePlatformStatus() {
  try {
    const response = await fetch('/health/ready', {
      cache: 'no-store',
      headers: { accept: 'application/json' },
    });
    if (!response.ok) {
      throw new Error(`readiness returned ${response.status}`);
    }
    statusText.textContent = 'Plattformkern bereit';
    statusDot.classList.add('is-ready');
  } catch {
    statusText.textContent = 'Plattformkern nicht erreichbar';
    statusDot.classList.add('is-unavailable');
  }
}

function showNotice(message, kind = '') {
  loginNotice.textContent = message;
  loginNotice.dataset.kind = kind;
  loginNotice.hidden = false;
}

function destinationForSession(session) {
  if (session?.must_change_password === true) return '/change-password';
  if (session?.mfa_enrollment_required === true) return '/mfa-setup';
  if (session?.account_class === 'admin' && session?.audience === 'werk-admin') return '/admin';
  if (session?.account_class === 'work' && session?.audience === 'werk-work') return '/app';
  return null;
}

function allowedRedirect(value) {
  return ['/change-password', '/mfa', '/mfa-setup', '/admin', '/app'].includes(value) ? value : null;
}

async function checkSession() {
  try {
    const response = await fetch('/api/v1/auth/session', { credentials: 'same-origin', cache: 'no-store', headers: { accept: 'application/json' } });
    if (!response.ok) return;
    const session = await response.json();
    const destination = destinationForSession(session);
    if (destination) window.location.replace(destination);
  } catch {
    // The login form remains usable when the optional session endpoint is unavailable.
  }
}

loginForm.addEventListener('submit', async (event) => {
  event.preventDefault();
  if (!loginForm.reportValidity()) {
    return;
  }
  submitButton.disabled = true;
  showNotice('Anmeldung wird geprüft …');
  const form = new FormData(loginForm);
  try {
    const response = await fetch('/api/v1/auth/login', {
      method: 'POST', credentials: 'same-origin',
      headers: { accept: 'application/json', 'content-type': 'application/json' },
      body: JSON.stringify({ login_name: form.get('login_name'), password: form.get('password') }),
    });
    const payload = await response.json().catch(() => ({}));
    if (!response.ok) throw new Error(payload.detail || 'Anmeldung fehlgeschlagen.');
	const destination = allowedRedirect(payload.redirect);
	if (!destination) throw new Error('Die Anmeldung lieferte kein gültiges Ziel.');
    showNotice('Anmeldung erfolgreich. WERK wird geöffnet …', 'success');
    loginForm.reset();
    // The server chooses the destination; no account type is exposed in the URL.
    window.location.assign(destination);
  } catch (error) {
    showNotice(error.message || 'Anmeldung fehlgeschlagen.', 'error');
  } finally {
    submitButton.disabled = false;
  }
});

logoutButton.addEventListener('click', async () => {
  logoutButton.disabled = true;
  try { await fetch('/api/v1/auth/logout', { method: 'POST', credentials: 'same-origin', headers: { accept: 'application/json', 'X-CSRF-Token': csrfToken() } }); } finally {
    loginForm.hidden = false;
    logoutButton.hidden = true;
    showNotice('Sie wurden abgemeldet.', 'success');
    logoutButton.disabled = false;
  }
});

updatePlatformStatus();
checkSession();
