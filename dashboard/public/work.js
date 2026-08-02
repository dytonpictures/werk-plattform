const workspaceRoot = document.querySelector('[data-workspace-root]');
const workspaceBusinessObjectKind = 'core.workspace.workspace';
let workspaceBusinessObjectLoad = 0;

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

function setWorkspaceBusinessObjectText(selector, value) {
  const element = document.querySelector(selector);
  if (element) element.textContent = value;
}

function workspaceBusinessObjectClassificationLabel(value) {
  return ({
    public: 'Öffentlich',
    internal: 'Intern',
    confidential: 'Vertraulich',
    restricted: 'Streng vertraulich',
  })[value] || value;
}

function setWorkspaceBusinessObjectState(state, label) {
  const card = document.querySelector('[data-business-object-card]');
  const status = document.querySelector('[data-business-object-status]');
  const dot = status?.querySelector('.status-dot');
  card?.setAttribute('aria-busy', state === 'loading' ? 'true' : 'false');
  if (status) status.dataset.state = state;
  dot?.classList.toggle('is-ready', state === 'active');
  dot?.classList.toggle('is-unavailable', state === 'unavailable');
  setWorkspaceBusinessObjectText('[data-business-object-status-label]', label);
}

function renderWorkspaceBusinessObjectLoading() {
  setWorkspaceBusinessObjectState('loading', 'Projektion wird geprüft');
  setWorkspaceBusinessObjectText('[data-business-object-title]', 'Wird geladen');
  setWorkspaceBusinessObjectText('[data-business-object-owner]', '—');
  setWorkspaceBusinessObjectText('[data-business-object-version]', '—');
  setWorkspaceBusinessObjectText('[data-business-object-updated-at]', '—');
  const classificationRow = document.querySelector('[data-business-object-classification-row]');
  if (classificationRow) classificationRow.hidden = true;
}

function renderWorkspaceBusinessObjectUnavailable() {
  setWorkspaceBusinessObjectState('unavailable', 'Projektion nicht verfügbar');
  setWorkspaceBusinessObjectText('[data-business-object-title]', 'Nicht verfügbar');
  setWorkspaceBusinessObjectText('[data-business-object-owner]', 'Nicht verfügbar');
  setWorkspaceBusinessObjectText('[data-business-object-version]', 'Nicht verfügbar');
  setWorkspaceBusinessObjectText('[data-business-object-updated-at]', 'Nicht verfügbar');
  const classificationRow = document.querySelector('[data-business-object-classification-row]');
  if (classificationRow) classificationRow.hidden = true;
}

function validWorkspaceBusinessObject(view, tenantID) {
  const classification = view?.classification;
  const updatedAt = typeof view?.updated_at === 'string' ? new Date(view.updated_at) : null;
  return view?.ref?.tenant_id === tenantID
    && view.ref.kind === workspaceBusinessObjectKind
    && view.ref.id === tenantID
    && view.owner_module === 'core.workspace'
    && typeof view.title === 'string'
    && view.title.trim() !== ''
    && (classification === undefined || (typeof classification === 'string' && classification.trim() !== ''))
    && Number.isSafeInteger(view.version)
    && view.version > 0
    && updatedAt !== null
    && !Number.isNaN(updatedAt.getTime());
}

function renderWorkspaceBusinessObject(view) {
  const updatedAt = new Date(view.updated_at);
  setWorkspaceBusinessObjectText('[data-business-object-title]', view.title);
  setWorkspaceBusinessObjectText('[data-business-object-owner]', view.owner_module);
  setWorkspaceBusinessObjectText('[data-business-object-version]', String(view.version));
  setWorkspaceBusinessObjectText(
    '[data-business-object-updated-at]',
    new Intl.DateTimeFormat('de-DE', { dateStyle: 'medium', timeStyle: 'short' }).format(updatedAt),
  );
  const classificationRow = document.querySelector('[data-business-object-classification-row]');
  if (classificationRow) {
    classificationRow.hidden = view.classification === undefined;
    if (!classificationRow.hidden) {
      setWorkspaceBusinessObjectText(
        '[data-business-object-classification]',
        workspaceBusinessObjectClassificationLabel(view.classification),
      );
    }
  }
  setWorkspaceBusinessObjectState('active', 'Projektion aktiv');
}

async function loadWorkspaceBusinessObject(tenantID) {
  const load = ++workspaceBusinessObjectLoad;
  renderWorkspaceBusinessObjectLoading();
  if (typeof tenantID !== 'string' || tenantID.trim() === '') {
    renderWorkspaceBusinessObjectUnavailable();
    return;
  }
  try {
    const response = await fetch(
      `/api/v1/business-objects/${workspaceBusinessObjectKind}/${encodeURIComponent(tenantID)}`,
      {
        credentials: 'same-origin',
        cache: 'no-store',
        headers: { accept: 'application/json' },
      },
    );
    const view = await response.json().catch(() => ({}));
    if (!response.ok || !validWorkspaceBusinessObject(view, tenantID)) {
      throw new Error('Workspace business object unavailable');
    }
    if (load !== workspaceBusinessObjectLoad) return;
    renderWorkspaceBusinessObject(view);
  } catch (_) {
    if (load === workspaceBusinessObjectLoad) renderWorkspaceBusinessObjectUnavailable();
  }
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
  setWorkspaceText('[data-workspace-tenant-name]', tenant.name || 'Unbekanntes Unternehmen');
  setWorkspaceText('[data-workspace-tenant-status]', tenant.status === 'active' ? 'Aktives Unternehmen' : tenant.status || 'Status unbekannt');
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
  const documentsModule = document.querySelector('[data-workspace-capability="documents"]');
  if (documentsModule) documentsModule.hidden = overview.capabilities?.documents !== true;
}

window.addEventListener('werk:workspace-ready', (event) => {
  const overview = event.detail || {};
  renderWorkspace(overview);
  void loadWorkspaceBusinessObject(overview.tenant?.id);
});

window.addEventListener('werk:workspace-error', (event) => {
  workspaceBusinessObjectLoad += 1;
  renderWorkspaceBusinessObjectUnavailable();
  const error = event.detail || new Error('Der Arbeitskontext konnte nicht geladen werden.');
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
