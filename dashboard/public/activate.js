(() => {
  const fragment = window.location.hash.slice(1);
  const tokenMatch = /^token=([A-Za-z0-9_-]{43})$/.exec(fragment);
  const invitationToken = tokenMatch?.[1] || '';
  window.history.replaceState(null, '', `${window.location.pathname}${window.location.search}`);

  const form = document.querySelector('[data-activation-form]');
  const notice = document.querySelector('[data-activation-notice]');
  const success = document.querySelector('[data-activation-success]');

  function showNotice(message, kind = '') {
    notice.textContent = message;
    notice.dataset.kind = kind;
    notice.hidden = false;
  }

  if (!invitationToken) {
    showNotice('Dieser Aktivierungslink ist ungültig oder unvollständig.', 'error');
    return;
  }

  form.hidden = false;
  notice.hidden = true;
  form.addEventListener('submit', async (event) => {
    event.preventDefault();
    if (!form.reportValidity()) return;
    const values = new FormData(form);
    const password = String(values.get('new_password') || '');
    if (password !== String(values.get('confirm_password') || '')) {
      showNotice('Die neuen Passwörter stimmen nicht überein.', 'error');
      return;
    }
    const button = form.querySelector('button[type="submit"]');
    button.disabled = true;
    showNotice('Konto wird aktiviert …');
    try {
      const response = await fetch('/api/v1/auth/invitations/initial/accept', {
        method: 'POST',
        credentials: 'same-origin',
        headers: { accept: 'application/json', 'content-type': 'application/json' },
        body: JSON.stringify({ token: invitationToken, new_password: password }),
      });
      if (response.status !== 204) throw new Error('activation-rejected');
      form.reset();
      form.hidden = true;
      notice.hidden = true;
      success.hidden = false;
    } catch {
      showNotice('Die Einladung konnte nicht aktiviert werden. Der Link ist möglicherweise ungültig, abgelaufen oder bereits verwendet.', 'error');
      button.disabled = false;
    }
  });
})();
