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

function renderOrganizationPath(path) {
  const container = document.querySelector('[data-workspace-organization-path]');
  if (!container) return;
  const items = Array.isArray(path) ? path.filter((item) => item?.name) : [];
  if (!items.length) {
    const item = document.createElement('li');
    item.textContent = 'Keine Organisationseinheit zugeordnet';
    item.className = 'is-empty';
    container.replaceChildren(item);
    return;
  }
  container.replaceChildren(...items.map((unit, index) => {
    const item = document.createElement('li');
    const name = document.createElement('span');
    name.textContent = unit.name;
    const type = document.createElement('span');
    type.className = 'visually-hidden';
    type.textContent = `, ${workspaceUnitTypeLabel(unit.unit_type)}`;
    item.append(name, type);
    if (index === items.length - 1) item.setAttribute('aria-current', 'location');
    return item;
  }));
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
  renderOrganizationPath(overview.organizational_path);
  const context = document.querySelector('[data-workspace-unit-context]');
  if (context) context.textContent = unit?.name ? ` · ${unit.name}` : '';
  const state = document.querySelector('[data-workspace-state]');
  if (state) {
    state.classList.remove('is-error');
    state.querySelector('.status-dot')?.classList.add('is-ready');
    const label = state.querySelector('[data-workspace-state-label]');
    if (label) label.textContent = 'Zugriff bestätigt';
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
  if (!response.ok) {
    const error = new Error(payload.detail || 'Der Arbeitskontext konnte nicht geladen werden.');
    error.status = response.status;
    throw error;
  }
  renderWorkspace(payload);
}

window.addEventListener('werk:session-ready', (event) => {
  if (event.detail?.account_class !== 'work') return;
  loadWorkspace().catch((error) => {
    workspaceRoot?.setAttribute('aria-busy', 'false');
    setWorkspaceText('[data-workspace-tenant-name]', 'Kontext nicht verfügbar');
    setWorkspaceText('[data-workspace-tenant-status]', 'Erneut laden oder neu anmelden');
    setWorkspaceText('[data-workspace-unit-name]', 'Nicht verfügbar');
    setWorkspaceText('[data-workspace-unit-type]', 'Keine bestätigte Zuordnung');
    setWorkspaceText('[data-workspace-membership]', 'Nicht verfügbar');
    setWorkspaceText('[data-workspace-permission]', '—');
    setWorkspaceText('[data-workspace-tenant-id]', '—');
    setWorkspaceText('[data-workspace-access]', 'Nicht bestätigt');
    renderOrganizationPath([]);
    const context = document.querySelector('[data-workspace-unit-context]');
    if (context) context.textContent = '';
    const state = document.querySelector('[data-workspace-state]');
    state?.classList.add('is-error');
    state?.querySelector('.status-dot')?.classList.remove('is-ready');
    const label = state?.querySelector('[data-workspace-state-label]');
    if (label) label.textContent = error.status === 403 ? 'Zugriff verweigert' : 'Kontext nicht verfügbar';
    showPageNotice(error.message, 'error');
  });
});
