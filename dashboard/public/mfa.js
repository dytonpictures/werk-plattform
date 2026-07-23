const pageType = document.body.dataset.mfaPage;
const notice = document.querySelector('[data-mfa-notice]');

function csrfToken() {
  const prefix = 'werk_csrf=';
  const value = document.cookie.split(';').map((part) => part.trim()).find((part) => part.startsWith(prefix));
  return value ? decodeURIComponent(value.slice(prefix.length)) : '';
}

function showNotice(message, kind = '') {
  if (!notice) return;
  notice.textContent = message;
  notice.dataset.kind = kind;
  notice.hidden = false;
}

async function postJSON(path, body) {
  const response = await fetch(path, {
    method: 'POST', credentials: 'same-origin',
    headers: { accept: 'application/json', 'content-type': 'application/json', 'X-CSRF-Token': csrfToken() },
    body: JSON.stringify(body),
  });
  const payload = await response.json().catch(() => ({}));
  if (!response.ok) throw new Error(payload.detail || 'Die Sicherheitsprüfung ist fehlgeschlagen.');
  return payload;
}

async function copyText(value) {
  if (navigator.clipboard?.writeText && window.isSecureContext) {
    await navigator.clipboard.writeText(value);
    return;
  }
  const input = document.createElement('textarea');
  input.value = value;
  input.setAttribute('readonly', '');
  input.style.position = 'fixed';
  input.style.opacity = '0';
  document.body.append(input);
  input.select();
  const copied = document.execCommand('copy');
  input.remove();
  if (!copied) throw new Error('Kopieren ist in diesem Browser nicht verfügbar.');
}

document.querySelectorAll('[data-copy-source]').forEach((button) => {
  button.addEventListener('click', async () => {
    const source = document.querySelector(button.dataset.copySource);
    const action = button.querySelector('[data-copy-action]');
    const value = source?.textContent?.trim();
    if (!value) return;
    try {
      await copyText(value);
      action.textContent = 'Kopiert';
      showNotice('In die Zwischenablage kopiert.', 'success');
      window.setTimeout(() => { action.textContent = 'Kopieren'; }, 1800);
    } catch (error) {
      showNotice(error.message, 'error');
    }
  });
});

document.querySelector('[data-mfa-challenge-form]')?.addEventListener('submit', async (event) => {
  event.preventDefault();
  const form = event.currentTarget;
  const button = form.querySelector('button[type="submit"]');
  button.disabled = true;
  try {
    const payload = await postJSON('/api/v1/auth/mfa/challenge', { code: new FormData(form).get('code') });
    window.location.replace(['/admin', '/change-password'].includes(payload.redirect) ? payload.redirect : '/');
  } catch (error) {
    showNotice(error.message, 'error');
    button.disabled = false;
  }
});

const enrollmentForm = document.querySelector('[data-mfa-enrollment-form]');
enrollmentForm?.addEventListener('submit', async (event) => {
  event.preventDefault();
  const form = new FormData(enrollmentForm);
  const button = enrollmentForm.querySelector('button[type="submit"]');
  button.disabled = true;
  try {
    const payload = await postJSON('/api/v1/auth/mfa/totp/enrollment', {
      current_password: form.get('current_password'), display_name: form.get('display_name'),
    });
    document.querySelector('[data-mfa-secret]').textContent = payload.secret;
    document.querySelector('[data-mfa-uri]').textContent = payload.otpauth_uri;
    document.querySelector('[data-mfa-qr]').src = payload.qr_code_data_url;
    document.querySelector('[data-mfa-confirmation-form] [name="factor_id"]').value = payload.factor_id;
    enrollmentForm.hidden = true;
    document.querySelector('[data-mfa-confirmation]').hidden = false;
    showNotice('Schlüssel erzeugt. Bestätigen Sie jetzt einen Code.', 'success');
  } catch (error) {
    showNotice(error.message, 'error');
    button.disabled = false;
  }
});

document.querySelector('[data-mfa-confirmation-form]')?.addEventListener('submit', async (event) => {
  event.preventDefault();
  const formElement = event.currentTarget;
  const form = new FormData(formElement);
  const button = formElement.querySelector('button[type="submit"]');
  button.disabled = true;
  try {
    const payload = await postJSON('/api/v1/auth/mfa/totp/confirmation', {
      factor_id: form.get('factor_id'), code: form.get('code'),
    });
    document.querySelector('[data-recovery-codes]').textContent = payload.recovery_codes.join('\n');
    document.querySelector('[data-mfa-confirmation]').hidden = true;
    document.querySelector('[data-mfa-recovery]').hidden = false;
    showNotice('MFA ist aktiv. Speichern Sie jetzt die Recovery-Codes.', 'success');
  } catch (error) {
    showNotice(error.message, 'error');
    button.disabled = false;
  }
});

document.querySelector('[data-mfa-finish]')?.addEventListener('click', () => window.location.replace('/admin'));

if (pageType === 'setup') {
  fetch('/api/v1/auth/session', { credentials: 'same-origin', cache: 'no-store', headers: { accept: 'application/json' } })
    .then(async (response) => {
      if (!response.ok) throw new Error();
      const session = await response.json();
      if (session.must_change_password) window.location.replace('/change-password');
      else if (!session.mfa_enrollment_required) window.location.replace(session.home_path || '/');
    })
    .catch(() => window.location.replace('/'));
}
