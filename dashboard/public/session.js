const page = document.querySelector('[data-authenticated-page]');
const notice = document.querySelector('[data-page-notice]');
const logout = document.querySelector('[data-page-logout]');
const passwordForm = document.querySelector('[data-password-form]');

function csrfToken() {
  const prefix = 'werk_csrf=';
  const value = document.cookie.split(';').map((part) => part.trim()).find((part) => part.startsWith(prefix));
  return value ? decodeURIComponent(value.slice(prefix.length)) : '';
}

document.querySelectorAll('.profile-menu').forEach((menu) => {
  menu.addEventListener('toggle', () => {
    if (!menu.open) return;
    document.querySelectorAll('.profile-menu[open]').forEach((other) => {
      if (other !== menu) other.removeAttribute('open');
    });
  });
});

document.addEventListener('click', (event) => {
  document.querySelectorAll('.profile-menu[open]').forEach((menu) => {
    if (!menu.contains(event.target)) menu.removeAttribute('open');
  });
});

document.addEventListener('keydown', (event) => {
  if (event.key !== 'Escape') return;
  document.querySelectorAll('.profile-menu[open]').forEach((menu) => {
    menu.removeAttribute('open');
    menu.querySelector('summary')?.focus();
  });
});

function showPageNotice(message, kind = '') {
  if (!notice) return;
  notice.textContent = message;
  notice.dataset.kind = kind;
  notice.hidden = false;
}

function destinationForSession(session) {
  if (session?.must_change_password === true) return '/change-password';
  if (session?.mfa_enrollment_required === true) return '/mfa-setup';
  if (session?.account_class === 'admin' && session?.audience === 'werk-admin' && session?.home_path === '/admin') return session.home_path;
  if (session?.account_class === 'work' && session?.audience === 'werk-work' && session?.home_path === '/app') return session.home_path;
  return '/';
}

function populateProfile(session) {
  const profile = session?.profile || {};
  document.querySelectorAll('[data-profile-name]').forEach((element) => {
    element.textContent = profile.display_name || 'WERK Konto';
  });
  document.querySelectorAll('[data-profile-login]').forEach((element) => {
    element.textContent = profile.login_name || '—';
  });
  document.querySelectorAll('[data-profile-initials]').forEach((element) => {
    const initials = String(profile.display_name || 'W')
      .split(/\s+/).filter(Boolean).slice(0, 2).map((part) => part[0]).join('').toUpperCase();
    element.textContent = initials || 'W';
  });
  document.querySelectorAll('[data-profile-account-class]').forEach((element) => {
    element.textContent = session.account_class === 'admin' ? 'Administrationskonto' : 'Arbeitskonto';
  });
  document.querySelectorAll('[data-profile-tenant]').forEach((element) => {
    element.textContent = session.tenant_id || 'Nicht mandantengebunden';
  });
  document.querySelectorAll('[data-profile-expires]').forEach((element) => {
    element.textContent = session.expires_at
      ? new Intl.DateTimeFormat('de-DE', { dateStyle: 'medium', timeStyle: 'short' }).format(new Date(session.expires_at))
      : '—';
  });
  const home = destinationForSession(session);
  document.querySelectorAll('[data-profile-home], [data-profile-back]').forEach((element) => {
    element.href = home;
  });
}

function renderGlobalNavigation(session) {
  const navigation = document.querySelector('[data-global-navigation]');
  if (!navigation) return;
  const workItems = [
    { icon: 'Ü', label: 'Übersicht', href: '/app' },
    { icon: 'E', label: 'Inbox', disabled: true },
    { icon: 'A', label: 'Meine Aufgaben', disabled: true },
    { icon: 'D', label: 'Dokumente', disabled: true },
    { icon: 'P', label: 'Mein Profil', href: '/profile' },
  ];
  const currentPath = window.location.pathname;
  const itemsContainer = document.createElement('div');
  itemsContainer.className = 'global-navigation-items';

  const createNavigationItem = (item, nested = false) => {
    const element = document.createElement(item.disabled ? 'button' : 'a');
    element.className = nested ? 'global-navigation-subitem' : 'global-navigation-item';
    element.title = item.label;
    if (!nested) {
      const icon = document.createElement('span');
      icon.className = 'global-navigation-icon';
      icon.textContent = item.icon;
      icon.setAttribute('aria-hidden', 'true');
      element.append(icon);
    }
    const label = document.createElement('span');
    label.className = 'global-navigation-label';
    label.textContent = item.label;
    element.append(label);
    if (item.disabled) {
      element.type = 'button';
      element.disabled = true;
      const status = document.createElement('span');
      status.className = 'global-navigation-status';
      status.textContent = item.status || 'geplant';
      element.append(status);
    } else {
      element.href = item.href;
      if (item.view) element.dataset.adminViewLink = item.view;
      if (currentPath === item.href || (item.view && window.location.hash === `#${item.view}`)) {
        element.classList.add('is-active');
        element.setAttribute('aria-current', 'page');
      }
    }
    return element;
  };

  if (session.account_class === 'admin') {
    const iconPaths = {
      identity: '<svg viewBox="0 0 24 24"><path d="M16 20v-1.5a4.5 4.5 0 0 0-4.5-4.5h-3A4.5 4.5 0 0 0 4 18.5V20M10 10a3 3 0 1 0 0-6 3 3 0 0 0 0 6Zm7-1v6m-3-3h6"/></svg>',
      organization: '<svg viewBox="0 0 24 24"><path d="M4 20h16M6 20V9h12v11M9 9V5h6v4M9 13h2m2 0h2m-6 3h2m2 0h2"/></svg>',
      security: '<svg viewBox="0 0 24 24"><path d="M12 3 5 6v5c0 4.6 2.8 8.2 7 10 4.2-1.8 7-5.4 7-10V6l-7-3Zm-3 9 2 2 4-5"/></svg>',
      operations: '<svg viewBox="0 0 24 24"><path d="M5 4h14v6H5V4Zm0 10h14v6H5v-6ZM8 7h.01M8 17h.01m3-10h5m-5 10h5"/></svg>',
    };
    const sections = [
      { label: 'Verwaltung', entries: [
        { icon: 'identity', label: 'Identität & Zugriff', open: true, items: [
          { label: 'Benutzerkonten', href: '/admin#users', view: 'users' },
          { label: 'Rollen & Berechtigungen', href: '/admin#roles', view: 'roles' },
          { label: 'Anmeldung & Provider', href: '/admin#providers', view: 'providers' },
        ] },
        { icon: 'organization', label: 'Unternehmensstruktur', href: '/admin#organization', view: 'organization' },
      ] },
      { label: 'Governance', entries: [
        { icon: 'security', label: 'Sicherheit & Audit', items: [
          { label: 'Sicherheitsrichtlinien', disabled: true },
          { label: 'Aktive Sitzungen', disabled: true },
          { label: 'Audit-Protokoll', href: '/admin#audit', view: 'audit' },
        ] },
      ] },
      { label: 'System', entries: [
        { icon: 'operations', label: 'Plattformbetrieb', href: '/admin#operations', view: 'operations' },
      ] },
    ];
    sections.forEach((section) => {
      const heading = document.createElement('div');
      heading.className = 'global-navigation-heading';
      heading.textContent = section.label;
      itemsContainer.append(heading);
      section.entries.forEach((group) => {
        if (group.href) {
        const link = document.createElement('a');
        link.className = 'global-navigation-item admin-section-link';
        link.href = group.href;
        link.dataset.adminViewLink = group.view;
        link.title = group.label;
        const icon = document.createElement('span');
        icon.className = 'global-navigation-icon';
        icon.setAttribute('aria-hidden', 'true');
        icon.innerHTML = iconPaths[group.icon];
        const label = document.createElement('span');
        label.className = 'global-navigation-label';
        label.textContent = group.label;
        link.append(icon, label);
        if (window.location.hash === `#${group.view}`) {
          link.classList.add('is-active');
          link.setAttribute('aria-current', 'page');
        }
        itemsContainer.append(link);
        return;
        }
        const details = document.createElement('details');
        details.className = 'global-navigation-group';
        details.open = group.open || group.items.some((item) => item.view && window.location.hash === `#${item.view}`);
        const summary = document.createElement('summary');
        const icon = document.createElement('span');
        icon.className = 'global-navigation-icon';
        icon.innerHTML = iconPaths[group.icon];
        icon.setAttribute('aria-hidden', 'true');
        const label = document.createElement('span');
        label.className = 'global-navigation-label';
        label.textContent = group.label;
        const chevron = document.createElement('span');
        chevron.className = 'global-navigation-group-chevron';
        chevron.textContent = '›';
        chevron.setAttribute('aria-hidden', 'true');
        summary.append(icon, label, chevron);
        const children = document.createElement('div');
        children.className = 'global-navigation-subitems';
        children.append(...group.items.map((item) => createNavigationItem(item, true)));
        details.append(summary, children);
        itemsContainer.append(details);
      });
    });
  } else {
    itemsContainer.append(...workItems.map((item) => createNavigationItem(item)));
  }

  const mode = session.preferences?.navigation_mode === 'collapsed' ? 'collapsed' : 'bar';
  navigation.dataset.mode = mode;
  document.documentElement.dataset.navigationMode = mode;
  const collapseButton = document.createElement('button');
  collapseButton.type = 'button';
  collapseButton.className = 'global-navigation-collapse';
  collapseButton.dataset.navigationMode = mode === 'collapsed' ? 'bar' : 'collapsed';
  collapseButton.setAttribute('aria-label', mode === 'collapsed' ? 'Navigation ausklappen' : 'Navigation einklappen');
  collapseButton.title = collapseButton.getAttribute('aria-label');
  collapseButton.innerHTML = `<span aria-hidden="true">${mode === 'collapsed' ? '›' : '‹'}</span><span class="global-navigation-label">${mode === 'collapsed' ? 'Ausklappen' : 'Einklappen'}</span>`;
  collapseButton.addEventListener('click', () => updateNavigationMode(navigation, collapseButton.dataset.navigationMode));
  navigation.replaceChildren(itemsContainer, collapseButton);
}

async function updateNavigationMode(navigation, mode) {
  const previousMode = navigation.dataset.mode || 'bar';
  if (mode === previousMode) return;
  const controls = [...navigation.querySelectorAll('[data-navigation-mode]')];
  controls.forEach((control) => { control.disabled = true; });
  navigation.dataset.mode = mode;
  document.documentElement.dataset.navigationMode = mode;
  try {
    const response = await fetch('/api/v1/auth/preferences', {
      method: 'PATCH',
      credentials: 'same-origin',
      headers: { accept: 'application/json', 'content-type': 'application/json', 'X-CSRF-Token': csrfToken() },
      body: JSON.stringify({ navigation_mode: mode }),
    });
    const payload = await response.json().catch(() => ({}));
    if (!response.ok) throw new Error(payload.detail || 'Die Darstellung konnte nicht gespeichert werden.');
    showPageNotice(`Navigation ${mode === 'collapsed' ? 'eingeklappt' : 'ausgeklappt'} und gespeichert.`, 'success');
    const session = await loadSession();
    if (!session) return;
  } catch (error) {
    navigation.dataset.mode = previousMode;
    document.documentElement.dataset.navigationMode = previousMode;
    showPageNotice(error.message || 'Die Darstellung konnte nicht gespeichert werden.', 'error');
  } finally {
    controls.forEach((control) => { control.disabled = false; });
  }
}

async function loadSession() {
  const response = await fetch('/api/v1/auth/session', {
    credentials: 'same-origin',
    cache: 'no-store',
    headers: { accept: 'application/json' },
  });
  if (!response.ok) {
    window.location.replace('/');
    return null;
  }
  const session = await response.json();
  const expected = page?.dataset.authenticatedPage;
  const destination = destinationForSession(session);
  if (expected && expected !== '/profile' && destination !== expected) {
    window.location.replace(destination);
    return null;
  }
  if (expected === '/profile' && session.must_change_password) {
    window.location.replace('/change-password');
    return null;
  }
  document.querySelectorAll('[data-account-class]').forEach((element) => {
    element.textContent = session.account_class === 'admin' ? 'Administration' : 'Arbeitsbereich';
  });
  document.querySelectorAll('[data-tenant-label]').forEach((element) => {
    element.textContent = session.tenant_id ? `Mandant · ${session.tenant_id.slice(0, 8)}` : 'Kein Mandant';
    element.title = session.tenant_id || '';
  });
  populateProfile(session);
  renderGlobalNavigation(session);
  window.dispatchEvent(new CustomEvent('werk:session-ready', { detail: session }));
  return session;
}

document.querySelectorAll('[data-workspace-section]').forEach((control) => {
  control.addEventListener('click', () => {
    showPageNotice(`${control.textContent.trim()} wird in einer der nächsten Core-Ausbaustufen aktiviert.`);
  });
});

logout?.addEventListener('click', async () => {
  logout.disabled = true;
  try {
    await fetch('/api/v1/auth/logout', {
      method: 'POST',
      credentials: 'same-origin',
      headers: { accept: 'application/json', 'X-CSRF-Token': csrfToken() },
    });
  } finally {
    window.location.replace('/');
  }
});

passwordForm?.addEventListener('submit', async (event) => {
  event.preventDefault();
  if (!passwordForm.reportValidity()) return;
  const form = new FormData(passwordForm);
  const newPassword = String(form.get('new_password') || '');
  if (newPassword !== String(form.get('confirm_password') || '')) {
    showPageNotice('Die neuen Passwörter stimmen nicht überein.', 'error');
    return;
  }
  const button = passwordForm.querySelector('button[type="submit"]');
  button.disabled = true;
  showPageNotice('Passwort wird geändert …');
  try {
    const response = await fetch('/api/v1/auth/password', {
      method: 'POST',
      credentials: 'same-origin',
      headers: { accept: 'application/json', 'content-type': 'application/json', 'X-CSRF-Token': csrfToken() },
      body: JSON.stringify({
        current_password: form.get('current_password'),
        new_password: newPassword,
      }),
    });
    const payload = await response.json().catch(() => ({}));
    if (!response.ok) throw new Error(payload.detail || 'Das Passwort konnte nicht geändert werden.');
    showPageNotice('Passwort geändert. Der passende Bereich wird geöffnet …', 'success');
    const session = await loadSession();
    if (session) window.location.replace(destinationForSession(session));
  } catch (error) {
    showPageNotice(error.message || 'Das Passwort konnte nicht geändert werden.', 'error');
    button.disabled = false;
  }
});

loadSession().catch(() => window.location.replace('/'));
