const pageType = document.body.dataset.mfaPage;
const notice = document.querySelector('[data-mfa-notice]');
const methodSelection = document.querySelector('[data-mfa-method-selection]');
const passkeyEnrollmentForm = document.querySelector('[data-passkey-enrollment-form]');
const totpEnrollmentForm = document.querySelector('[data-mfa-enrollment-form]');
const confirmation = document.querySelector('[data-mfa-confirmation]');
const recovery = document.querySelector('[data-mfa-recovery]');
let setupAccountClass = '';
let setupDestination = '/';

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

function showMethodSelection() {
  passkeyEnrollmentForm?.reset();
  totpEnrollmentForm?.reset();
  if (passkeyEnrollmentForm) passkeyEnrollmentForm.hidden = true;
  if (totpEnrollmentForm) totpEnrollmentForm.hidden = true;
  if (confirmation) confirmation.hidden = true;
  if (recovery) recovery.hidden = true;
  if (methodSelection) methodSelection.hidden = false;
  if (notice) notice.hidden = true;
}

function selectMethod(method) {
  if (!methodSelection || !['passkey', 'totp'].includes(method)) return;
  if (method === 'totp' && setupAccountClass !== 'admin') return;
  methodSelection.hidden = true;
  const form = method === 'passkey' ? passkeyEnrollmentForm : totpEnrollmentForm;
  if (!form) return;
  form.hidden = false;
  form.querySelector('[name="current_password"]')?.focus();
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

document.querySelectorAll('[data-select-mfa-method]').forEach((button) => {
  button.addEventListener('click', () => selectMethod(button.dataset.selectMfaMethod));
});
document.querySelectorAll('[data-mfa-method-back]').forEach((button) => {
  button.addEventListener('click', showMethodSelection);
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

passkeyEnrollmentForm?.addEventListener('submit', async (event) => {
  event.preventDefault();
  const formElement = event.currentTarget;
  if (!formElement.reportValidity()) return;
  const form = new FormData(formElement);
  const displayName = String(form.get('display_name') || '').trim();
  const button = formElement.querySelector('button[type="submit"]');
  button.disabled = true;
  showNotice('Passkey-Einrichtung wird gestartet …');
  try {
    const options = await postJSON('/api/v1/auth/passkeys/registration/options', {
      current_password: form.get('current_password'), display_name: displayName,
    });
    const credential = await window.WebAuthnClient.create(options);
    await postJSON('/api/v1/auth/passkeys/registration/verification', {
      display_name: displayName, credential,
    });
    showNotice('Passkey eingerichtet. Der passende Bereich wird geöffnet …', 'success');
    window.location.replace(setupDestination);
  } catch (error) {
    const message = error?.name === 'NotAllowedError'
      ? 'Die Passkey-Einrichtung wurde abgebrochen oder ist abgelaufen.'
      : error.message;
    showNotice(message || 'Die Passkey-Einrichtung ist fehlgeschlagen.', 'error');
    button.disabled = false;
  }
});

totpEnrollmentForm?.addEventListener('submit', async (event) => {
  event.preventDefault();
  if (setupAccountClass !== 'admin' || !totpEnrollmentForm.reportValidity()) return;
  const form = new FormData(totpEnrollmentForm);
  const button = totpEnrollmentForm.querySelector('button[type="submit"]');
  button.disabled = true;
  try {
    const payload = await postJSON('/api/v1/auth/mfa/totp/enrollment', {
      current_password: form.get('current_password'), display_name: form.get('display_name'),
    });
    document.querySelector('[data-mfa-secret]').textContent = payload.secret;
    document.querySelector('[data-mfa-uri]').textContent = payload.otpauth_uri;
    document.querySelector('[data-mfa-qr]').src = payload.qr_code_data_url;
    document.querySelector('[data-mfa-confirmation-form] [name="factor_id"]').value = payload.factor_id;
    totpEnrollmentForm.reset();
    totpEnrollmentForm.hidden = true;
    confirmation.hidden = false;
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
    confirmation.hidden = true;
    recovery.hidden = false;
    showNotice('MFA ist aktiv. Speichern Sie jetzt die Recovery-Codes.', 'success');
  } catch (error) {
    showNotice(error.message, 'error');
    button.disabled = false;
  }
});

document.querySelector('[data-mfa-finish]')?.addEventListener('click', () => window.location.replace(setupDestination));

function prepareSetup(session) {
  if (session.must_change_password) {
    window.location.replace('/change-password');
    return;
  }
  const isAdmin = session.account_class === 'admin' && session.audience === 'admin' && session.home_path === '/admin';
  const isWork = session.account_class === 'work' && session.audience === 'work' && session.home_path === '/app';
  if (!isAdmin && !isWork) {
    window.location.replace('/');
    return;
  }
  setupAccountClass = isAdmin ? 'admin' : 'work';
  setupDestination = isAdmin ? '/admin' : '/app';
  document.querySelector('[data-mfa-loading]').hidden = true;
  document.querySelector('[data-mfa-kicker]').textContent = isAdmin ? 'Administrationsschutz' : 'Empfohlener Kontoschutz';
  document.querySelector('[data-mfa-title]').textContent = isAdmin ? 'Administration absichern' : 'Passkey einrichten';
  document.querySelector('[data-mfa-description]').textContent = isAdmin
    ? 'Die Administration bleibt gesperrt, bis Sie selbst eine Methode auswählen und erfolgreich bestätigen. Ein Passkey ist die empfohlene Option.'
    : 'Ein Passkey ist für Ihr Arbeitskonto empfohlen und freiwillig. WERK startet weder TOTP noch eine Einrichtung ohne Ihre Auswahl.';
  document.querySelector('[data-passkey-method-copy]').textContent = isAdmin
    ? 'Der Passkey bestätigt die für den Admin-Bereich erforderliche Sicherheitsstufe.'
    : 'Der Passkey ist empfohlen und bleibt Ihre freiwillige Entscheidung.';
  document.querySelector('[data-admin-mfa-method]').hidden = !isAdmin;
  document.querySelector('[data-mfa-account-label]').textContent = isAdmin ? 'Administrationskonto' : 'Arbeitskonto · freiwillig';
  document.querySelector('[data-mfa-cancel]').href = isAdmin ? '/admin' : '/profile';
  showMethodSelection();
}

if (pageType === 'setup') {
  fetch('/api/v1/auth/session', { credentials: 'same-origin', cache: 'no-store', headers: { accept: 'application/json' } })
    .then(async (response) => {
      if (!response.ok) throw new Error('session-unavailable');
      prepareSetup(await response.json());
    })
    .catch(() => window.location.replace('/'));
}
