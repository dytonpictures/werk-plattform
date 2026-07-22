const workUserForm = document.querySelector('[data-work-user-form]');
const tenantForm = document.querySelector('[data-tenant-form]');
const unitForm = document.querySelector('[data-unit-form]');
const roleForm = document.querySelector('[data-role-form]');
const tenantEditForm = document.querySelector('[data-tenant-edit-form]');
const unitEditForm = document.querySelector('[data-unit-edit-form]');
const roleEditForm = document.querySelector('[data-role-edit-form]');
const roleAssignmentForm = document.querySelector('[data-role-assignment-form]');
const tenantSelect = document.querySelector('[data-tenant-select]');
const unitFormTenantSelect = document.querySelector('[data-unit-form-tenant-select]');
const parentUnitSelect = document.querySelector('[data-parent-unit-select]');
const workTenantSelect = document.querySelector('[data-work-tenant-select]');
const workFormTenantSelect = document.querySelector('[data-work-form-tenant-select]');
const workUnitSelect = document.querySelector('[data-work-unit-select]');
const roleTenantSelect = document.querySelector('[data-role-tenant-select]');
const roleFormTenantSelect = document.querySelector('[data-role-form-tenant-select]');
const tenantList = document.querySelector('[data-tenant-list]');
const tenantSummary = document.querySelector('[data-tenant-summary]');
const unitList = document.querySelector('[data-unit-list]');
const unitSummary = document.querySelector('[data-unit-summary]');
const unitEmpty = document.querySelector('[data-unit-empty]');
const userList = document.querySelector('[data-user-list]');
const userSummary = document.querySelector('[data-user-summary]');
const userEmpty = document.querySelector('[data-user-empty]');
const userSearch = document.querySelector('[data-user-search]');
const roleList = document.querySelector('[data-role-list]');
const roleEmpty = document.querySelector('[data-role-empty]');
const auditTenantSelect = document.querySelector('[data-audit-tenant-select]');
const auditOutcomeSelect = document.querySelector('[data-audit-outcome-select]');
const auditEventType = document.querySelector('[data-audit-event-type]');
const auditList = document.querySelector('[data-audit-list]');
const auditEmpty = document.querySelector('[data-audit-empty]');
let currentUsers = [];
let currentTenants = [];
let currentUnits = [];
let currentRoles = [];
let currentPermissions = [];
let currentAuditEvents = [];
let nextAuditCursor = '';
let loadedRoleTenantID = '';
let userPage = 1;
let userSort = { key: 'display_name', direction: 1 };
const userPageSize = 25;

function adminCSRFToken() {
  const prefix = 'werk_csrf=';
  const value = document.cookie.split(';').map((part) => part.trim()).find((part) => part.startsWith(prefix));
  return value ? decodeURIComponent(value.slice(prefix.length)) : '';
}

async function adminRequest(path, options = {}) {
  const response = await fetch(path, {
    credentials: 'same-origin',
    ...options,
    headers: {
      accept: 'application/json',
      ...(options.body ? { 'content-type': 'application/json', 'X-CSRF-Token': adminCSRFToken() } : {}),
      ...(options.headers || {}),
    },
  });
  const payload = await response.json().catch(() => ({}));
  if (!response.ok) throw new Error(payload.detail || 'Die Verwaltungsaktion ist fehlgeschlagen.');
  return payload;
}

function replaceOptions(select, items, placeholder, label) {
  if (!select) return;
  const current = select.value;
  select.replaceChildren(new Option(placeholder, ''));
  items.forEach((item) => select.add(new Option(label(item), item.id)));
  if (items.some((item) => item.id === current)) select.value = current;
}

function statusLabel(status) {
  return ({ active: 'Aktiv', suspended: 'Ausgesetzt', archived: 'Archiviert', retired: 'Außer Betrieb', disabled: 'Deaktiviert', locked: 'Gesperrt', pending: 'Ausstehend' })[status] || status;
}

function roleLabel(role) {
  const configuredRole = currentRoles.find((item) => item.role_key === role);
  if (configuredRole) return configuredRole.display_name;
  return ({
    'workspace-member': 'Workspace-Mitglied',
    'workspace-manager': 'Workspace-Verantwortlich',
    'tenant-manager': 'Mandantenverantwortlich',
  })[role] || role;
}

function membershipLabel(membership) {
  return ({ 'team.member': 'Teammitglied', 'team.manager': 'Teamleitung' })[membership] || membership || 'Keine Mitgliedschaft';
}

function unitTypeLabel(type) {
  return ({ company: 'Gesellschaft', location: 'Standort', division: 'Bereich', department: 'Abteilung', team: 'Team' })[type] || type;
}

function initials(name) {
  return String(name || 'W').split(/\s+/).filter(Boolean).slice(0, 2).map((part) => part[0]).join('').toUpperCase() || 'W';
}

function createCell(value, className = '') {
  const cell = document.createElement('td');
  cell.className = className;
  if (value instanceof Node) cell.append(value);
  else cell.textContent = value;
  return cell;
}

async function copyToClipboard(value) {
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

function showAdminView() {
  const allowed = ['users', 'roles', 'providers', 'organization', 'audit', 'operations'];
  const rawRequested = window.location.hash.slice(1);
  const requested = ['tenants', 'units'].includes(rawRequested) ? 'organization' : rawRequested;
  const view = allowed.includes(requested) ? requested : 'users';
  if (rawRequested !== view) history.replaceState(null, '', `#${view}`);
  document.querySelectorAll('[data-admin-view]').forEach((section) => { section.hidden = section.dataset.adminView !== view; });
  document.querySelectorAll('[data-admin-view-link]').forEach((link) => {
    const active = link.dataset.adminViewLink === view;
    link.classList.toggle('is-active', active);
    if (active) link.setAttribute('aria-current', 'page');
    else link.removeAttribute('aria-current');
  });
  if (view === 'organization' && tenantSelect?.value) loadUnits(tenantSelect.value).catch((error) => showPageNotice(error.message, 'error'));
  if (view === 'roles' && roleTenantSelect?.value) loadRoles(roleTenantSelect.value).catch((error) => showPageNotice(error.message, 'error'));
  if (view === 'audit') loadAuditEvents(true).catch((error) => showPageNotice(error.message, 'error'));
  if (view === 'operations') loadOperationsStatus().catch((error) => showPageNotice(error.message, 'error'));
}

function openDialog(dialog) {
  if (!dialog?.showModal) return;
  dialog.showModal();
}

document.addEventListener('click', (event) => {
  const close = event.target.closest('[data-dialog-close]');
  if (close) close.closest('dialog')?.close();
  const copy = event.target.closest('[data-copy-details]');
  if (copy) {
    const value = document.querySelector(copy.dataset.copyDetails)?.textContent?.trim();
    if (value) copyToClipboard(value).then(() => {
      copy.textContent = 'Kopiert';
      window.setTimeout(() => { copy.textContent = 'Kopieren'; }, 1600);
    }).catch((error) => showPageNotice(error.message, 'error'));
  }
});

document.querySelectorAll('dialog').forEach((dialog) => {
  dialog.addEventListener('click', (event) => { if (event.target === dialog) dialog.close(); });
});

document.querySelector('[data-open-user-dialog]')?.addEventListener('click', async () => {
  if (workTenantSelect.value) workFormTenantSelect.value = workTenantSelect.value;
  if (workFormTenantSelect.value) await loadWorkUnits(workFormTenantSelect.value);
  openDialog(document.querySelector('[data-user-dialog]'));
});

document.querySelector('[data-open-tenant-dialog]')?.addEventListener('click', () => openDialog(document.querySelector('[data-tenant-dialog]')));

document.querySelector('[data-open-tenant-edit-dialog]')?.addEventListener('click', () => {
  const tenant = currentTenants.find((item) => item.id === tenantSelect.value);
  if (!tenant) return;
  tenantEditForm.elements.tenant_id.value = tenant.id;
  tenantEditForm.elements.version.value = tenant.version;
  tenantEditForm.elements.name.value = tenant.name;
  tenantEditForm.elements.status.value = tenant.status;
  tenantEditForm.elements.default_locale.value = tenant.default_locale;
  tenantEditForm.elements.default_timezone.value = tenant.default_timezone;
  openDialog(document.querySelector('[data-tenant-edit-dialog]'));
});

document.querySelector('[data-open-unit-dialog]')?.addEventListener('click', async () => {
  if (tenantSelect.value) unitFormTenantSelect.value = tenantSelect.value;
  if (unitFormTenantSelect.value) await loadParentUnits(unitFormTenantSelect.value);
  openDialog(document.querySelector('[data-unit-dialog]'));
});

document.querySelector('[data-open-role-dialog]')?.addEventListener('click', async () => {
  const tenantID = roleTenantSelect.value || tenantSelect.value;
  if (!tenantID) {
    showPageNotice('Wählen Sie zuerst einen Mandanten aus.', 'error');
    return;
  }
  roleFormTenantSelect.value = tenantID;
  if (loadedRoleTenantID !== tenantID) await loadRoles(tenantID);
  renderPermissionChoices();
  openDialog(document.querySelector('[data-role-dialog]'));
});

document.querySelector('[data-open-selected-users]')?.addEventListener('click', async () => {
  if (!tenantSelect.value) return;
  workTenantSelect.value = tenantSelect.value;
  window.location.hash = 'users';
  await loadUsers(tenantSelect.value);
});

function renderTenants(items) {
  [tenantSelect, unitFormTenantSelect, workTenantSelect, workFormTenantSelect, roleTenantSelect, roleFormTenantSelect].forEach((select) => replaceOptions(select, items, 'Mandant wählen', (tenant) => tenant.name));
  replaceOptions(auditTenantSelect, items, 'Alle Mandanten und Installation', (tenant) => tenant.name);
  if (tenantSummary) tenantSummary.textContent = `${items.length} Mandant${items.length === 1 ? '' : 'en'}`;
  tenantList?.replaceChildren();
  items.forEach((tenant) => {
    const button = document.createElement('button');
    button.type = 'button';
    button.className = 'organization-tenant-item';
    button.dataset.tenantId = tenant.id;
    button.setAttribute('aria-pressed', String(tenant.id === tenantSelect.value));
    const marker = document.createElement('span');
    marker.className = `organization-tenant-marker status-${tenant.status}`;
    marker.setAttribute('aria-hidden', 'true');
    const identity = document.createElement('span');
    identity.className = 'organization-tenant-identity';
    const name = document.createElement('strong');
    name.textContent = tenant.name;
    const meta = document.createElement('span');
    meta.textContent = `${statusLabel(tenant.status)} · ${tenant.default_locale}`;
    identity.append(name, meta);
    const chevron = document.createElement('span');
    chevron.className = 'organization-tenant-chevron';
    chevron.textContent = '›';
    chevron.setAttribute('aria-hidden', 'true');
    button.append(marker, identity, chevron);
    button.addEventListener('click', () => selectOrganizationTenant(tenant.id).catch((error) => showPageNotice(error.message, 'error')));
    tenantList?.append(button);
  });
}

async function selectOrganizationTenant(tenantID) {
  const tenant = currentTenants.find((item) => item.id === tenantID);
  if (!tenant) return;
  [tenantSelect, unitFormTenantSelect, workTenantSelect, workFormTenantSelect, roleTenantSelect, roleFormTenantSelect].forEach((select) => { select.value = tenantID; });
  document.querySelectorAll('[data-tenant-id]').forEach((button) => {
    const active = button.dataset.tenantId === tenantID;
    button.classList.toggle('is-active', active);
    button.setAttribute('aria-pressed', String(active));
  });
  document.querySelector('[data-selected-tenant-name]').textContent = tenant.name;
  const addUnit = document.querySelector('[data-open-unit-dialog]');
  if (addUnit) addUnit.disabled = false;
  const showUsers = document.querySelector('[data-open-selected-users]');
  if (showUsers) showUsers.disabled = false;
  const editTenant = document.querySelector('[data-open-tenant-edit-dialog]');
  if (editTenant) editTenant.disabled = false;
  const loads = [loadUnits(tenantID), loadWorkUnits(tenantID), loadUsers(tenantID)];
  if (window.location.hash === '#roles') loads.push(loadRoles(tenantID));
  await Promise.all(loads);
}

async function loadTenants() {
  const payload = await adminRequest('/admin/v1/tenants');
  const items = payload.items || [];
  currentTenants = items;
  renderTenants(items);
  if (!items.length) {
    const addUnit = document.querySelector('[data-open-unit-dialog]');
    if (addUnit) addUnit.disabled = true;
    const showUsers = document.querySelector('[data-open-selected-users]');
    if (showUsers) showUsers.disabled = true;
    const editTenant = document.querySelector('[data-open-tenant-edit-dialog]');
    if (editTenant) editTenant.disabled = true;
    renderUnits([]);
    return;
  }
  const selected = items.some((item) => item.id === tenantSelect.value) ? tenantSelect.value : items[0].id;
  await selectOrganizationTenant(selected);
}

function renderUnits(items) {
  if (unitSummary) unitSummary.textContent = `${items.length} Organisationseinheit${items.length === 1 ? '' : 'en'}`;
  if (unitEmpty) unitEmpty.hidden = items.length !== 0;
  unitList?.replaceChildren();
  const names = new Map(items.map((item) => [item.id, item.name]));
  items.forEach((unit) => {
    const row = document.createElement('tr');
    const identity = document.createElement('div');
    identity.className = 'table-primary';
    const name = document.createElement('strong');
    name.textContent = unit.name;
    identity.append(name);
    const status = document.createElement('span');
    status.className = `status-badge status-${unit.status}`;
    status.textContent = statusLabel(unit.status);
    const code = document.createElement('code');
    code.className = 'table-code';
    code.textContent = unit.id.slice(0, 8);
    code.title = unit.id;
    const actions = document.createElement('div');
    actions.className = 'row-actions';
    const edit = document.createElement('button');
    edit.type = 'button';
    edit.className = 'button button-compact';
    edit.textContent = 'Bearbeiten';
    edit.addEventListener('click', () => openUnitEdit(unit));
    actions.append(edit);
    row.append(createCell(identity), createCell(unitTypeLabel(unit.unit_type)), createCell(unit.parent_id ? names.get(unit.parent_id) || unit.parent_id : '—'), createCell(status), createCell(code), createCell(actions, 'table-actions'));
    unitList?.append(row);
  });
}

async function fetchUnits(tenantID) {
  if (!tenantID) return [];
  const payload = await adminRequest(`/admin/v1/tenants/${encodeURIComponent(tenantID)}/organizational-units`);
  return payload.items || [];
}

async function loadUnits(tenantID) {
  if (!tenantID) {
    currentUnits = [];
    renderUnits([]);
    if (unitSummary) unitSummary.textContent = 'Mandant auswählen, um Einheiten anzuzeigen.';
    return;
  }
  const items = await fetchUnits(tenantID);
  currentUnits = items;
  renderUnits(items);
  if (unitFormTenantSelect.value === tenantID) replaceOptions(parentUnitSelect, items, 'Keine', (unit) => `${unit.name} (${unit.unit_type})`);
}

function openUnitEdit(unit) {
  unitEditForm.elements.tenant_id.value = unit.tenant_id;
  unitEditForm.elements.unit_id.value = unit.id;
  unitEditForm.elements.version.value = unit.version;
  unitEditForm.elements.name.value = unit.name;
  unitEditForm.elements.unit_type.value = unit.unit_type;
  unitEditForm.elements.status.value = unit.status;
  const parentSelect = unitEditForm.querySelector('[data-unit-edit-parent-select]');
  replaceOptions(parentSelect, currentUnits.filter((candidate) => candidate.id !== unit.id && candidate.status === 'active'), 'Keine', (candidate) => `${candidate.name} (${unitTypeLabel(candidate.unit_type)})`);
  parentSelect.value = unit.parent_id || '';
  openDialog(document.querySelector('[data-unit-edit-dialog]'));
}

async function loadParentUnits(tenantID) {
  const items = await fetchUnits(tenantID);
  replaceOptions(parentUnitSelect, items, 'Keine', (unit) => `${unit.name} (${unit.unit_type})`);
}

async function loadWorkUnits(tenantID) {
  const items = await fetchUnits(tenantID);
  replaceOptions(workUnitSelect, items, 'Einheit wählen', (unit) => `${unit.name} (${unit.unit_type})`);
}

function createChoice(name, value, title, detail, checked = false) {
  const label = document.createElement('label');
  label.className = 'admin-choice-item';
  const input = document.createElement('input');
  input.type = 'checkbox';
  input.name = name;
  input.value = value;
  input.checked = checked;
  const copy = document.createElement('span');
  const strong = document.createElement('strong');
  strong.textContent = title;
  const small = document.createElement('small');
  small.textContent = detail;
  copy.append(strong, small);
  label.append(input, copy);
  return label;
}

function renderPermissionChoices() {
  const container = document.querySelector('[data-role-permission-options]');
  container?.replaceChildren(...currentPermissions.map((permission) => createChoice(
    'permission_keys',
    permission.permission_key,
    permission.display_name,
    `${permission.permission_key} · Risiko ${permission.risk_level}`,
  )));
}

function renderRoles() {
  if (roleEmpty) roleEmpty.hidden = currentRoles.length !== 0;
  document.querySelector('[data-role-total]').textContent = String(currentRoles.length);
  document.querySelector('[data-role-system]').textContent = String(currentRoles.filter((role) => role.system_role).length);
  document.querySelector('[data-role-custom]').textContent = String(currentRoles.filter((role) => !role.system_role).length);
  document.querySelector('[data-role-permission-total]').textContent = `${currentPermissions.length} registrierte Work-Berechtigung${currentPermissions.length === 1 ? '' : 'en'}`;
  const tenant = currentTenants.find((item) => item.id === roleTenantSelect.value);
  document.querySelector('[data-role-tenant-name]').textContent = tenant?.name || 'Kein Mandant ausgewählt';
  roleList?.replaceChildren();
  currentRoles.forEach((role) => {
    const row = document.createElement('tr');
    const identity = document.createElement('div');
    identity.className = 'table-primary';
    const name = document.createElement('strong');
    name.textContent = role.display_name;
    const key = document.createElement('span');
    key.textContent = role.role_key;
    identity.append(name, key);
    const type = document.createElement('span');
    type.className = `status-badge ${role.system_role ? '' : 'status-active'}`;
    type.textContent = role.system_role ? 'Systemrolle · geschützt' : 'Eigene Rolle';
    const permissions = document.createElement('div');
    permissions.className = 'table-tags';
    const visiblePermissions = role.permissions.slice(0, 3);
    visiblePermissions.forEach((permission) => {
      const tag = document.createElement('span');
      tag.textContent = permission.display_name;
      tag.title = permission.permission_key;
      permissions.append(tag);
    });
    if (role.permissions.length > visiblePermissions.length) {
      const remaining = document.createElement('span');
      remaining.textContent = `+${role.permissions.length - visiblePermissions.length}`;
      remaining.title = role.permissions.slice(visiblePermissions.length).map((permission) => permission.permission_key).join(', ');
      permissions.append(remaining);
    }
    if (!role.permissions.length) {
      const none = document.createElement('span');
      none.textContent = 'Keine Berechtigung';
      permissions.append(none);
    }
    const status = document.createElement('span');
    status.className = `status-badge status-${role.status}`;
    status.textContent = statusLabel(role.status);
    const actions = document.createElement('div');
    actions.className = 'row-actions';
    const accounts = document.createElement('button');
    accounts.type = 'button';
    accounts.className = 'button button-compact';
    accounts.textContent = 'Konten';
    accounts.addEventListener('click', async () => {
      userSearch.value = role.role_key;
      window.location.hash = 'users';
      await loadUsers(role.tenant_id);
    });
    if (!role.system_role) {
      const edit = document.createElement('button');
      edit.type = 'button';
      edit.className = 'button button-compact';
      edit.textContent = 'Bearbeiten';
      edit.addEventListener('click', () => openRoleEdit(role));
      actions.append(edit);
    }
    actions.append(accounts);
    row.append(createCell(identity), createCell(type), createCell(permissions), createCell(`${role.assignment_count} Konto${role.assignment_count === 1 ? '' : 'en'}`), createCell(status), createCell(actions, 'table-actions'));
    roleList?.append(row);
  });
}

function openRoleEdit(role) {
  roleEditForm.elements.role_id.value = role.id;
  roleEditForm.elements.tenant_id.value = role.tenant_id;
  roleEditForm.elements.version.value = role.version;
  roleEditForm.elements.display_name.value = role.display_name;
  roleEditForm.elements.status.value = role.status;
  document.querySelector('[data-role-edit-key]').textContent = role.role_key;
  const assigned = new Set(role.permissions.map((permission) => permission.permission_key));
  document.querySelector('[data-role-edit-permission-options]')?.replaceChildren(...currentPermissions.map((permission) => createChoice(
    'permission_keys',
    permission.permission_key,
    permission.display_name,
    `${permission.permission_key} · Risiko ${permission.risk_level}`,
    assigned.has(permission.permission_key),
  )));
  openDialog(document.querySelector('[data-role-edit-dialog]'));
}

async function loadRoles(tenantID) {
  if (!tenantID) {
    currentRoles = [];
    currentPermissions = [];
    loadedRoleTenantID = '';
    renderRoles();
    return;
  }
  const payload = await adminRequest(`/admin/v1/work-roles?tenant_id=${encodeURIComponent(tenantID)}`);
  currentRoles = payload.roles || [];
  currentPermissions = payload.permissions || [];
  loadedRoleTenantID = tenantID;
  roleTenantSelect.value = tenantID;
  roleFormTenantSelect.value = tenantID;
  renderRoles();
}

async function openRoleAssignment(user) {
  if (loadedRoleTenantID !== user.tenant_id) await loadRoles(user.tenant_id);
  const dialog = document.querySelector('[data-role-assignment-dialog]');
  const form = dialog.querySelector('[data-role-assignment-form]');
  form.elements.account_id.value = user.account_id;
  form.elements.tenant_id.value = user.tenant_id;
  dialog.querySelector('[data-role-assignment-name]').textContent = `Rollen für ${user.display_name}`;
  dialog.querySelector('[data-role-assignment-login]').textContent = user.login_name;
  const assigned = new Set(user.roles || []);
  dialog.querySelector('[data-role-assignment-options]').replaceChildren(...currentRoles.filter((role) => role.status === 'active').map((role) => createChoice(
    'role_ids',
    role.id,
    role.display_name,
    `${role.role_key} · ${role.system_role ? 'geschützte Systemrolle' : `${role.permissions.length} Berechtigung${role.permissions.length === 1 ? '' : 'en'}`}`,
    assigned.has(role.role_key),
  )));
  openDialog(dialog);
}

function openUserDetails(user) {
  const dialog = document.querySelector('[data-user-details-dialog]');
  dialog.querySelector('[data-details-name]').textContent = user.display_name;
  dialog.querySelector('[data-details-login]').textContent = user.login_name;
  dialog.querySelector('[data-details-status]').textContent = `${statusLabel(user.status)} · ${user.must_change_password ? 'Passwortwechsel erforderlich' : 'Passwort eingerichtet'}`;
  dialog.querySelector('[data-details-unit]').textContent = user.organizational_unit_name || 'Keine Einheit';
  dialog.querySelector('[data-details-membership]').textContent = membershipLabel(user.membership_type);
  dialog.querySelector('[data-details-roles]').textContent = user.roles.length ? user.roles.map(roleLabel).join(', ') : 'Keine Rollen';
  dialog.querySelector('[data-details-account-id]').textContent = user.account_id;
  dialog.querySelector('[data-details-party-id]').textContent = user.party_id;
  dialog.querySelector('[data-manage-details-roles]').dataset.accountId = user.account_id;
  openDialog(dialog);
}

function renderUsers() {
  const query = userSearch?.value.trim().toLocaleLowerCase('de') || '';
  const filtered = currentUsers.filter((user) => !query || [user.display_name, user.login_name, user.organizational_unit_name, user.membership_type, ...(user.roles || [])].join(' ').toLocaleLowerCase('de').includes(query));
  const collator = new Intl.Collator('de', { sensitivity: 'base', numeric: true });
  filtered.sort((left, right) => userSort.direction * collator.compare(String(left[userSort.key] || ''), String(right[userSort.key] || '')));
  const pageCount = Math.max(1, Math.ceil(filtered.length / userPageSize));
  userPage = Math.min(userPage, pageCount);
  const start = (userPage - 1) * userPageSize;
  const items = filtered.slice(start, start + userPageSize);
  if (userSummary) userSummary.textContent = query ? `${filtered.length} von ${currentUsers.length} Benutzern` : `${filtered.length} Benutzer`;
  if (userEmpty) userEmpty.hidden = filtered.length !== 0;
  document.querySelector('[data-user-total]').textContent = String(currentUsers.length);
  document.querySelector('[data-user-active]').textContent = String(currentUsers.filter((user) => user.status === 'active').length);
  document.querySelector('[data-user-onboarding]').textContent = String(currentUsers.filter((user) => user.must_change_password).length);
  const range = document.querySelector('[data-user-range]');
  if (range) range.textContent = filtered.length ? `${start + 1}–${start + items.length} von ${filtered.length}` : '0 Einträge';
  const previous = document.querySelector('[data-user-previous]');
  const next = document.querySelector('[data-user-next]');
  if (previous) previous.disabled = userPage <= 1;
  if (next) next.disabled = userPage >= pageCount;
  userList?.replaceChildren();
  items.forEach((user) => {
    const row = document.createElement('tr');
    const identity = document.createElement('div');
    identity.className = 'table-user';
    const avatar = document.createElement('span');
    avatar.className = 'table-avatar';
    avatar.textContent = initials(user.display_name);
    const names = document.createElement('div');
    names.className = 'table-primary';
    const name = document.createElement('strong');
    name.textContent = user.display_name;
    const login = document.createElement('span');
    login.textContent = user.login_name;
    names.append(name, login);
    identity.append(avatar, names);
    const roles = document.createElement('div');
    roles.className = 'table-tags';
    (user.roles.length ? user.roles : ['Keine Rolle']).forEach((role) => {
      const tag = document.createElement('span');
      tag.textContent = roleLabel(role);
      if (roleLabel(role) !== role) tag.title = role;
      roles.append(tag);
    });
    const status = document.createElement('span');
    status.className = `status-badge status-${user.status}`;
    status.textContent = statusLabel(user.status);
    const security = document.createElement('span');
    security.className = `status-badge ${user.must_change_password ? 'status-warning' : 'status-active'}`;
    security.textContent = user.must_change_password ? 'Passwortwechsel' : 'Eingerichtet';
    const details = document.createElement('button');
    details.type = 'button';
    details.className = 'button button-compact';
    details.textContent = 'Details';
    details.addEventListener('click', () => openUserDetails(user));
    const manageRoles = document.createElement('button');
    manageRoles.type = 'button';
    manageRoles.className = 'button button-compact';
    manageRoles.textContent = 'Rollen';
    manageRoles.addEventListener('click', () => openRoleAssignment(user).catch((error) => showPageNotice(error.message, 'error')));
    const actions = document.createElement('div');
    actions.className = 'row-actions';
    actions.append(manageRoles, details);
    row.append(createCell(identity), createCell(user.organizational_unit_name || '—'), createCell(roles), createCell(status), createCell(security), createCell(actions, 'table-actions'));
    userList?.append(row);
  });
}

async function loadUsers(tenantID) {
  if (!tenantID) {
    currentUsers = [];
    userPage = 1;
    renderUsers();
    if (userSummary) userSummary.textContent = 'Mandant auswählen, um Konten anzuzeigen.';
    document.querySelector('[data-user-refreshed]').textContent = 'Kein Mandant ausgewählt';
    return;
  }
  const payload = await adminRequest(`/admin/v1/work-users?tenant_id=${encodeURIComponent(tenantID)}`);
  currentUsers = payload.items || [];
  userPage = 1;
  document.querySelector('[data-user-refreshed]').textContent = `Aktualisiert um ${new Intl.DateTimeFormat('de-DE', { hour: '2-digit', minute: '2-digit' }).format(new Date())} Uhr`;
  renderUsers();
}

function auditOutcomeLabel(outcome) {
  return ({ succeeded: 'Erfolgreich', denied: 'Abgelehnt', failed: 'Fehlgeschlagen' })[outcome] || outcome;
}

function accountClassLabel(accountClass) {
  return ({ work: 'Arbeitskonto', admin: 'Administrationskonto', service: 'Servicekonto' })[accountClass] || 'Nicht zugeordnet';
}

function shortIdentifier(value) {
  if (!value) return '—';
  return value.length > 13 ? `${value.slice(0, 8)}…${value.slice(-4)}` : value;
}

function auditTimestamp(value) {
  return new Intl.DateTimeFormat('de-DE', {
    dateStyle: 'medium',
    timeStyle: 'medium',
  }).format(new Date(value));
}

function openAuditDetails(item) {
  document.querySelector('[data-audit-details-event]').textContent = item.event_type;
  document.querySelector('[data-audit-details-time]').textContent = auditTimestamp(item.occurred_at);
  document.querySelector('[data-audit-details-outcome]').textContent = auditOutcomeLabel(item.outcome);
  document.querySelector('[data-audit-details-tenant]').textContent = item.tenant_name || item.tenant_id || 'Installation';
  document.querySelector('[data-audit-details-account-class]').textContent = accountClassLabel(item.actor_account_class);
  document.querySelector('[data-audit-details-account]').textContent = item.actor_account_id || 'System / nicht zugeordnet';
  document.querySelector('[data-audit-details-id]').textContent = item.id;
  document.querySelector('[data-audit-details-request]').textContent = item.request_id;
  document.querySelector('[data-audit-details-correlation]').textContent = item.correlation_id;
  openDialog(document.querySelector('[data-audit-details-dialog]'));
}

function renderAuditEvents() {
  auditList?.replaceChildren();
  if (auditEmpty) auditEmpty.hidden = currentAuditEvents.length !== 0;
  currentAuditEvents.forEach((item) => {
    const row = document.createElement('tr');
    const occurred = document.createElement('div');
    occurred.className = 'table-primary';
    const date = document.createElement('strong');
    date.textContent = auditTimestamp(item.occurred_at);
    const eventID = document.createElement('span');
    eventID.textContent = shortIdentifier(item.id);
    eventID.title = item.id;
    occurred.append(date, eventID);

    const event = document.createElement('div');
    event.className = 'table-primary';
    const eventName = document.createElement('strong');
    eventName.textContent = item.event_type;
    const correlation = document.createElement('span');
    correlation.textContent = `Korrelation ${shortIdentifier(item.correlation_id)}`;
    correlation.title = item.correlation_id;
    event.append(eventName, correlation);

    const outcome = document.createElement('span');
    outcome.className = `status-badge ${item.outcome === 'succeeded' ? 'status-active' : item.outcome === 'denied' ? 'status-warning' : 'status-disabled'}`;
    outcome.textContent = auditOutcomeLabel(item.outcome);

    const tenant = document.createElement('div');
    tenant.className = 'table-primary';
    const tenantName = document.createElement('strong');
    tenantName.textContent = item.tenant_name || 'Installation';
    const tenantID = document.createElement('span');
    tenantID.textContent = item.tenant_id ? shortIdentifier(item.tenant_id) : 'Globaler Kontext';
    tenantID.title = item.tenant_id || '';
    tenant.append(tenantName, tenantID);

    const actor = document.createElement('div');
    actor.className = 'table-primary';
    const actorClass = document.createElement('strong');
    actorClass.textContent = accountClassLabel(item.actor_account_class);
    const actorID = document.createElement('span');
    actorID.textContent = shortIdentifier(item.actor_account_id);
    actorID.title = item.actor_account_id || '';
    actor.append(actorClass, actorID);

    const details = document.createElement('button');
    details.type = 'button';
    details.className = 'button button-compact';
    details.textContent = 'Details';
    details.addEventListener('click', () => openAuditDetails(item));
    row.append(createCell(occurred), createCell(event), createCell(outcome), createCell(tenant), createCell(actor), createCell(details, 'table-actions'));
    auditList?.append(row);
  });

  const denied = currentAuditEvents.filter((item) => item.outcome === 'denied').length;
  const failed = currentAuditEvents.filter((item) => item.outcome === 'failed').length;
  document.querySelector('[data-audit-total]').textContent = String(currentAuditEvents.length);
  document.querySelector('[data-audit-denied]').textContent = String(denied);
  document.querySelector('[data-audit-failed]').textContent = String(failed);
  document.querySelector('[data-audit-range]').textContent = `${currentAuditEvents.length} Ereignis${currentAuditEvents.length === 1 ? '' : 'se'} angezeigt`;
  const more = document.querySelector('[data-load-more-audit]');
  if (more) {
    more.disabled = !nextAuditCursor;
    more.textContent = nextAuditCursor ? 'Weitere laden' : 'Keine weiteren Ereignisse';
  }
}

async function loadAuditEvents(reset) {
  if (!reset && !nextAuditCursor) return;
  const parameters = new URLSearchParams({ limit: '50' });
  if (auditTenantSelect?.value) parameters.set('tenant_id', auditTenantSelect.value);
  if (auditOutcomeSelect?.value) parameters.set('outcome', auditOutcomeSelect.value);
  if (auditEventType?.value.trim()) parameters.set('event_type', auditEventType.value.trim());
  if (!reset) parameters.set('cursor', nextAuditCursor);
  const payload = await adminRequest(`/admin/v1/security-audit?${parameters.toString()}`);
  currentAuditEvents = reset ? (payload.items || []) : currentAuditEvents.concat(payload.items || []);
  nextAuditCursor = payload.next_cursor || '';
  renderAuditEvents();
  document.querySelector('[data-audit-refreshed]').textContent = `Aktualisiert um ${new Intl.DateTimeFormat('de-DE', { hour: '2-digit', minute: '2-digit' }).format(new Date())} Uhr`;
  const scope = auditTenantSelect?.value ? auditTenantSelect.selectedOptions[0]?.textContent : 'Installation und alle Mandanten';
  document.querySelector('[data-audit-summary]').textContent = `${currentAuditEvents.length} Ereignis${currentAuditEvents.length === 1 ? '' : 'se'} · ${scope}`;
}

function setOperationState(kind, ready) {
  const dot = document.querySelector(`[data-${kind}-dot]`);
  const status = document.querySelector(`[data-${kind}-status]`);
  dot?.classList.toggle('is-ready', ready);
  dot?.classList.toggle('is-error', !ready);
  if (status) status.textContent = ready ? 'Bereit' : 'Nicht bereit';
}

async function loadOperationsStatus() {
  const overall = document.querySelector('[data-operations-overall]');
  const checked = document.querySelector('[data-operations-checked]');
  if (!overall || !checked) return;
  overall.className = 'status-badge';
  overall.textContent = 'Wird geprüft';
  const check = async (path) => {
    try {
      const response = await fetch(path, { cache: 'no-store', headers: { accept: 'application/json' } });
      return response.ok;
    } catch {
      return false;
    }
  };
  const [live, ready, metaResponse] = await Promise.all([
    check('/health/live'),
    check('/health/ready'),
    fetch('/meta', { cache: 'no-store', headers: { accept: 'application/json' } }).catch(() => null),
  ]);
  setOperationState('live', live);
  setOperationState('ready', ready);
  const healthy = live && ready;
  overall.classList.add(healthy ? 'status-active' : 'status-disabled');
  overall.textContent = healthy ? 'Betriebsbereit' : 'Prüfung erforderlich';
  checked.textContent = `Zuletzt geprüft um ${new Intl.DateTimeFormat('de-DE', { hour: '2-digit', minute: '2-digit', second: '2-digit' }).format(new Date())} Uhr`;
  if (metaResponse?.ok) {
    const meta = await metaResponse.json();
    document.querySelector('[data-meta-product]').textContent = meta.product || '—';
    document.querySelector('[data-meta-service]').textContent = meta.service || '—';
    document.querySelector('[data-meta-version]').textContent = meta.version || '—';
    document.querySelector('[data-meta-api]').textContent = meta.api_version || '—';
  }
}

userSearch?.addEventListener('input', () => { userPage = 1; renderUsers(); });
document.querySelectorAll('[data-user-sort]').forEach((button) => {
  button.addEventListener('click', () => {
    const key = button.dataset.userSort;
    userSort = userSort.key === key ? { key, direction: userSort.direction * -1 } : { key, direction: 1 };
    document.querySelectorAll('[data-user-sort]').forEach((other) => {
      const active = other === button;
      other.classList.toggle('is-active', active);
      other.querySelector('span').textContent = active ? (userSort.direction === 1 ? '↑' : '↓') : '↕';
    });
    userPage = 1;
    renderUsers();
  });
});
document.querySelector('[data-user-previous]')?.addEventListener('click', () => { if (userPage > 1) { userPage -= 1; renderUsers(); } });
document.querySelector('[data-user-next]')?.addEventListener('click', () => { userPage += 1; renderUsers(); });
document.querySelector('[data-refresh-users]')?.addEventListener('click', async (event) => {
  const button = event.currentTarget;
  button.disabled = true;
  button.classList.add('is-refreshing');
  try {
    await loadUsers(workTenantSelect.value);
  } catch (error) {
    showPageNotice(error.message, 'error');
  } finally {
    button.disabled = false;
    button.classList.remove('is-refreshing');
  }
});
document.querySelector('[data-refresh-operations]')?.addEventListener('click', async (event) => {
  const button = event.currentTarget;
  button.disabled = true;
  button.classList.add('is-refreshing');
  try { await loadOperationsStatus(); } finally {
    button.disabled = false;
    button.classList.remove('is-refreshing');
  }
});
document.querySelector('[data-refresh-audit]')?.addEventListener('click', async (event) => {
  const button = event.currentTarget;
  button.disabled = true;
  button.classList.add('is-refreshing');
  try { await loadAuditEvents(true); } catch (error) { showPageNotice(error.message, 'error'); } finally {
    button.disabled = false;
    button.classList.remove('is-refreshing');
  }
});
document.querySelector('[data-load-more-audit]')?.addEventListener('click', async (event) => {
  const button = event.currentTarget;
  button.disabled = true;
  try { await loadAuditEvents(false); } catch (error) { showPageNotice(error.message, 'error'); } finally { button.disabled = !nextAuditCursor; }
});
auditTenantSelect?.addEventListener('change', () => loadAuditEvents(true).catch((error) => showPageNotice(error.message, 'error')));
auditOutcomeSelect?.addEventListener('change', () => loadAuditEvents(true).catch((error) => showPageNotice(error.message, 'error')));
auditEventType?.addEventListener('change', () => loadAuditEvents(true).catch((error) => showPageNotice(error.message, 'error')));
document.querySelector('[data-refresh-roles]')?.addEventListener('click', async (event) => {
  const button = event.currentTarget;
  button.disabled = true;
  button.classList.add('is-refreshing');
  try { await loadRoles(roleTenantSelect.value); } catch (error) { showPageNotice(error.message, 'error'); } finally {
    button.disabled = false;
    button.classList.remove('is-refreshing');
  }
});
document.querySelector('[data-manage-details-roles]')?.addEventListener('click', (event) => {
  const user = currentUsers.find((item) => item.account_id === event.currentTarget.dataset.accountId);
  if (!user) return;
  event.currentTarget.closest('dialog')?.close();
  openRoleAssignment(user).catch((error) => showPageNotice(error.message, 'error'));
});
tenantSelect?.addEventListener('change', () => {
  unitFormTenantSelect.value = tenantSelect.value;
  loadUnits(tenantSelect.value).catch((error) => showPageNotice(error.message, 'error'));
});
unitFormTenantSelect?.addEventListener('change', () => loadParentUnits(unitFormTenantSelect.value).catch((error) => showPageNotice(error.message, 'error')));
workFormTenantSelect?.addEventListener('change', () => loadWorkUnits(workFormTenantSelect.value).catch((error) => showPageNotice(error.message, 'error')));
workTenantSelect?.addEventListener('change', () => {
  selectOrganizationTenant(workTenantSelect.value).catch((error) => showPageNotice(error.message, 'error'));
});
roleTenantSelect?.addEventListener('change', () => {
  if (!roleTenantSelect.value) {
    loadRoles('');
    return;
  }
  selectOrganizationTenant(roleTenantSelect.value).catch((error) => showPageNotice(error.message, 'error'));
});
roleFormTenantSelect?.addEventListener('change', async () => {
  try {
    await selectOrganizationTenant(roleFormTenantSelect.value);
    renderPermissionChoices();
  } catch (error) {
    showPageNotice(error.message, 'error');
  }
});

tenantForm?.addEventListener('submit', async (event) => {
  event.preventDefault();
  if (!tenantForm.reportValidity()) return;
  const button = tenantForm.querySelector('button[type="submit"]');
  button.disabled = true;
  try {
    const payload = await adminRequest('/admin/v1/tenants', { method: 'POST', body: JSON.stringify(Object.fromEntries(new FormData(tenantForm).entries())) });
    showPageNotice(`Mandant ${payload.name} wurde angelegt.`, 'success');
    tenantForm.reset();
    tenantForm.elements.default_locale.value = 'de-DE';
    tenantForm.elements.default_timezone.value = 'Europe/Berlin';
    tenantForm.closest('dialog')?.close();
    await loadTenants();
    await selectOrganizationTenant(payload.id);
  } catch (error) {
    showPageNotice(error.message || 'Der Mandant konnte nicht angelegt werden.', 'error');
  } finally { button.disabled = false; }
});

tenantEditForm?.addEventListener('submit', async (event) => {
  event.preventDefault();
  if (!tenantEditForm.reportValidity()) return;
  const button = tenantEditForm.querySelector('button[type="submit"]');
  const form = new FormData(tenantEditForm);
  const tenantID = form.get('tenant_id');
  button.disabled = true;
  try {
    const payload = await adminRequest(`/admin/v1/tenants/${encodeURIComponent(tenantID)}`, {
      method: 'PUT',
      headers: { 'If-Match': `"${form.get('version')}"` },
      body: JSON.stringify({
        name: form.get('name'),
        status: form.get('status'),
        default_locale: form.get('default_locale'),
        default_timezone: form.get('default_timezone'),
      }),
    });
    showPageNotice(`Mandant ${payload.name} wurde gespeichert.`, 'success');
    tenantEditForm.closest('dialog')?.close();
    await loadTenants();
  } catch (error) {
    showPageNotice(error.message || 'Der Mandant konnte nicht gespeichert werden.', 'error');
  } finally { button.disabled = false; }
});

unitForm?.addEventListener('submit', async (event) => {
  event.preventDefault();
  if (!unitForm.reportValidity()) return;
  const button = unitForm.querySelector('button[type="submit"]');
  const form = new FormData(unitForm);
  const tenantID = form.get('tenant_id');
  const body = { name: form.get('name'), unit_type: form.get('unit_type') };
  if (form.get('parent_id')) body.parent_id = form.get('parent_id');
  button.disabled = true;
  try {
    const payload = await adminRequest(`/admin/v1/tenants/${encodeURIComponent(tenantID)}/organizational-units`, { method: 'POST', body: JSON.stringify(body) });
    showPageNotice(`Organisationseinheit ${payload.name} wurde angelegt.`, 'success');
    unitForm.elements.name.value = '';
    unitForm.elements.parent_id.value = '';
    unitForm.closest('dialog')?.close();
    tenantSelect.value = tenantID;
    await Promise.all([loadUnits(tenantID), workFormTenantSelect.value === tenantID ? loadWorkUnits(tenantID) : Promise.resolve()]);
  } catch (error) {
    showPageNotice(error.message || 'Die Organisationseinheit konnte nicht angelegt werden.', 'error');
  } finally { button.disabled = false; }
});

unitEditForm?.addEventListener('submit', async (event) => {
  event.preventDefault();
  if (!unitEditForm.reportValidity()) return;
  const button = unitEditForm.querySelector('button[type="submit"]');
  const form = new FormData(unitEditForm);
  const tenantID = form.get('tenant_id');
  const unitID = form.get('unit_id');
  const body = {
    name: form.get('name'),
    unit_type: form.get('unit_type'),
    status: form.get('status'),
  };
  if (form.get('parent_id')) body.parent_id = form.get('parent_id');
  button.disabled = true;
  try {
    const payload = await adminRequest(`/admin/v1/tenants/${encodeURIComponent(tenantID)}/organizational-units/${encodeURIComponent(unitID)}`, {
      method: 'PUT',
      headers: { 'If-Match': `"${form.get('version')}"` },
      body: JSON.stringify(body),
    });
    showPageNotice(`Organisationseinheit ${payload.name} wurde gespeichert.`, 'success');
    unitEditForm.closest('dialog')?.close();
    await Promise.all([loadUnits(tenantID), loadWorkUnits(tenantID)]);
  } catch (error) {
    showPageNotice(error.message || 'Die Organisationseinheit konnte nicht gespeichert werden.', 'error');
  } finally { button.disabled = false; }
});

workUserForm?.addEventListener('submit', async (event) => {
  event.preventDefault();
  if (!workUserForm.reportValidity()) return;
  const button = workUserForm.querySelector('button[type="submit"]');
  const form = new FormData(workUserForm);
  const tenantID = form.get('tenant_id');
  button.disabled = true;
  try {
    const payload = await adminRequest('/admin/v1/work-users', { method: 'POST', body: JSON.stringify(Object.fromEntries(form.entries())) });
    showPageNotice(`Benutzer ${payload.login_name} wurde angelegt. Beim ersten Login ist ein Passwortwechsel erforderlich.`, 'success');
    workUserForm.reset();
    workUserForm.elements.membership_type.value = 'team.member';
    workUserForm.closest('dialog')?.close();
    workFormTenantSelect.value = tenantID;
    workTenantSelect.value = tenantID;
    await Promise.all([loadWorkUnits(tenantID), loadUsers(tenantID)]);
  } catch (error) {
    showPageNotice(error.message || 'Das Arbeitskonto konnte nicht angelegt werden.', 'error');
  } finally { button.disabled = false; }
});

roleForm?.addEventListener('submit', async (event) => {
  event.preventDefault();
  if (!roleForm.reportValidity()) return;
  const button = roleForm.querySelector('button[type="submit"]');
  const form = new FormData(roleForm);
  const permissionKeys = form.getAll('permission_keys');
  if (!permissionKeys.length) {
    showPageNotice('Wählen Sie mindestens eine Work-Berechtigung aus.', 'error');
    return;
  }
  const tenantID = form.get('tenant_id');
  button.disabled = true;
  try {
    const payload = await adminRequest('/admin/v1/work-roles', {
      method: 'POST',
      body: JSON.stringify({ tenant_id: tenantID, role_key: form.get('role_key'), display_name: form.get('display_name'), permission_keys: permissionKeys }),
    });
    showPageNotice(`Rolle ${payload.display_name} wurde angelegt.`, 'success');
    roleForm.elements.role_key.value = '';
    roleForm.elements.display_name.value = '';
    roleForm.closest('dialog')?.close();
    await loadRoles(tenantID);
  } catch (error) {
    showPageNotice(error.message || 'Die Work-Rolle konnte nicht angelegt werden.', 'error');
  } finally { button.disabled = false; }
});

roleEditForm?.addEventListener('submit', async (event) => {
  event.preventDefault();
  if (!roleEditForm.reportValidity()) return;
  const button = roleEditForm.querySelector('button[type="submit"]');
  const form = new FormData(roleEditForm);
  const permissionKeys = form.getAll('permission_keys');
  if (!permissionKeys.length) {
    showPageNotice('Wählen Sie mindestens eine Work-Berechtigung aus.', 'error');
    return;
  }
  const tenantID = form.get('tenant_id');
  const roleID = form.get('role_id');
  button.disabled = true;
  try {
    const payload = await adminRequest(`/admin/v1/work-roles/${encodeURIComponent(roleID)}`, {
      method: 'PUT',
      headers: { 'If-Match': `"${form.get('version')}"` },
      body: JSON.stringify({
        tenant_id: tenantID,
        display_name: form.get('display_name'),
        status: form.get('status'),
        permission_keys: permissionKeys,
      }),
    });
    showPageNotice(`Rolle ${payload.display_name} wurde gespeichert.`, 'success');
    roleEditForm.closest('dialog')?.close();
    await Promise.all([loadRoles(tenantID), loadUsers(tenantID)]);
  } catch (error) {
    showPageNotice(error.message || 'Die Work-Rolle konnte nicht gespeichert werden.', 'error');
  } finally { button.disabled = false; }
});

roleAssignmentForm?.addEventListener('submit', async (event) => {
  event.preventDefault();
  const button = roleAssignmentForm.querySelector('button[type="submit"]');
  const form = new FormData(roleAssignmentForm);
  const accountID = form.get('account_id');
  const tenantID = form.get('tenant_id');
  button.disabled = true;
  try {
    await adminRequest(`/admin/v1/work-users/${encodeURIComponent(accountID)}/roles`, {
      method: 'PUT',
      body: JSON.stringify({ tenant_id: tenantID, role_ids: form.getAll('role_ids') }),
    });
    showPageNotice('Die Work-Rollenzuweisung wurde gespeichert.', 'success');
    roleAssignmentForm.closest('dialog')?.close();
    await Promise.all([loadUsers(tenantID), loadRoles(tenantID)]);
  } catch (error) {
    showPageNotice(error.message || 'Die Rollenzuweisung konnte nicht gespeichert werden.', 'error');
  } finally { button.disabled = false; }
});

window.addEventListener('hashchange', showAdminView);
showAdminView();
loadTenants().catch((error) => {
  if (tenantSummary) tenantSummary.textContent = 'Mandanten konnten nicht geladen werden.';
  showPageNotice(error.message, 'error');
});
