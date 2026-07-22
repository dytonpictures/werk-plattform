const workspaceRoot = document.querySelector('[data-workspace-root]');

function workspaceMembershipLabel(value) {
  return ({
    'team.member': 'Teammitglied',
    'team.manager': 'Teamleitung',
  })[value] || value || 'Keine aktive Mitgliedschaft';
}

function workspaceUnitTypeLabel(value) {
  return ({
    company: 'Gesellschaft',
    location: 'Standort',
    division: 'Bereich',
    department: 'Abteilung',
    team: 'Team',
  })[value] || value || 'Nicht zugeordnet';
}

function setWorkspaceText(selector, value) {
  document.querySelectorAll(selector).forEach((element) => { element.textContent = value; });
}

function renderWorkspace(overview) {
  const tenant = overview.tenant || {};
  const unit = overview.organizational_unit;
  setWorkspaceText('[data-workspace-tenant-name]', tenant.name || 'Unbekannter Mandant');
  setWorkspaceText('[data-workspace-tenant-status]', tenant.status === 'active' ? 'Aktiver Mandant' : tenant.status || 'Status unbekannt');
  setWorkspaceText('[data-workspace-unit-name]', unit?.name || 'Keine Organisationseinheit');
  setWorkspaceText('[data-workspace-unit-type]', unit ? workspaceUnitTypeLabel(unit.unit_type) : 'Nicht zugeordnet');
  setWorkspaceText('[data-workspace-membership]', workspaceMembershipLabel(overview.membership_type));
  setWorkspaceText('[data-workspace-permission]', overview.permission || '—');
  setWorkspaceText('[data-workspace-tenant-id]', tenant.id || '—');
  setWorkspaceText('[data-workspace-access]', 'Freigegeben');
  const context = document.querySelector('[data-workspace-unit-context]');
  if (context) context.textContent = unit?.name ? ` · ${unit.name}` : '';
  const state = document.querySelector('[data-workspace-state]');
  if (state) {
    state.classList.remove('is-error');
    state.querySelector('.status-dot')?.classList.add('is-ready');
    state.lastChild.textContent = ' Zugriff bestätigt';
  }
  workspaceRoot?.setAttribute('aria-busy', 'false');
}

async function loadWorkspace() {
  const response = await fetch('/api/v1/workspace', {
    credentials: 'same-origin',
    cache: 'no-store',
    headers: { accept: 'application/json' },
  });
  const payload = await response.json().catch(() => ({}));
  if (!response.ok) throw new Error(payload.detail || 'Der Arbeitskontext konnte nicht geladen werden.');
  renderWorkspace(payload);
}

window.addEventListener('werk:session-ready', (event) => {
  if (event.detail?.account_class !== 'work') return;
  loadWorkspace().catch((error) => {
    workspaceRoot?.setAttribute('aria-busy', 'false');
    const state = document.querySelector('[data-workspace-state]');
    state?.classList.add('is-error');
    if (state?.lastChild) state.lastChild.textContent = ' Zugriff verweigert';
    showPageNotice(error.message, 'error');
  });
});
