const workUserForm = document.querySelector('[data-work-user-form]');
const tenantForm = document.querySelector('[data-tenant-form]');
const unitForm = document.querySelector('[data-unit-form]');
const roleForm = document.querySelector('[data-role-form]');
const tenantEditForm = document.querySelector('[data-tenant-edit-form]');
const unitEditForm = document.querySelector('[data-unit-edit-form]');
const roleEditForm = document.querySelector('[data-role-edit-form]');
const roleAssignmentForm = document.querySelector('[data-role-assignment-form]');
const userStatusForm = document.querySelector('[data-user-status-form]');
const invitationReissueForm = document.querySelector('[data-invitation-reissue-form]');
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
const adminMFARecommendation = document.querySelector('[data-admin-mfa-recommendation]');
const invitationResultDialog = document.querySelector('[data-invitation-result-dialog]');
const tenantContextControls = [tenantSelect, unitFormTenantSelect, workTenantSelect, workFormTenantSelect, roleTenantSelect, roleFormTenantSelect];
let currentUsers = [];
let currentTenants = [];
let currentUnits = [];
let currentRoles = [];
let currentPermissions = [];
let currentAuditEvents = [];
let nextAuditCursor = '';
let loadedUnitTenantID = '';
let loadedUserTenantID = '';
let loadedRoleTenantID = '';
let userPage = 1;
let userSort = { key: 'display_name', direction: 1 };
let adminPlaneReady = false;
let adminInitialized = false;
let adminMFARecommended = false;
let activeInvitationURL = '';
let selectedTenantID = '';
let tenantContextRevision = 0;
let tenantLoadSequence = 0;
let unitLoadSequence = 0;
let userLoadSequence = 0;
let roleLoadSequence = 0;
let auditLoadSequence = 0;
const userPageSize = 25;
const userCollator = new Intl.Collator('de', { sensitivity: 'base', numeric: true });
const dateTimeFormatter = new Intl.DateTimeFormat('de-DE', { dateStyle: 'medium', timeStyle: 'short' });
const timeFormatter = new Intl.DateTimeFormat('de-DE', { hour: '2-digit', minute: '2-digit' });
const latestRequestControllers = new Map();
let userSearchTimer = 0;

function adminCSRFToken() {
  const prefix = 'werk_csrf=';
  const value = document.cookie.split(';').map((part) => part.trim()).find((part) => part.startsWith(prefix));
  return value ? decodeURIComponent(value.slice(prefix.length)) : '';
}

async function adminRequest(path, options = {}, reauthenticationAttempted = false) {
  if (!adminPlaneReady) throw new Error('Die Administrationssitzung wird noch geprüft.');
  const method = String(options.method || 'GET').toUpperCase();
  const mutatesState = !['GET', 'HEAD', 'OPTIONS'].includes(method);
  const response = await fetch(path, {
    credentials: 'same-origin',
    ...options,
    headers: {
      accept: 'application/json',
      ...(options.body ? { 'content-type': 'application/json' } : {}),
      ...(mutatesState ? { 'X-CSRF-Token': adminCSRFToken() } : {}),
      ...(options.headers || {}),
    },
  });
  const payload = await response.json().catch(() => ({}));
  if (!response.ok) {
	if (response.status === 428 && payload.code === 'reauthentication-required' && !reauthenticationAttempted) {
	  const binding = {
		permission_key: response.headers.get('WERK-Reauth-Permission') || '',
		resource_kind: response.headers.get('WERK-Reauth-Resource-Kind') || '',
		resource_id: response.headers.get('WERK-Reauth-Resource-ID') || '',
	  };
	  if (!binding.permission_key || !binding.resource_kind || !binding.resource_id) {
		throw new Error('Die Re-Authentifizierungsbindung des Servers ist unvollständig.');
	  }
	  const credential = window.prompt('Kritische Aktion: Bestätigen Sie sich mit Ihrem aktuellen Admin-Passwort oder einem 6-stelligen Code Ihrer Authenticator-App.');
	  if (credential === null) throw new Error('Die kritische Aktion wurde nicht bestätigt.');
	  const totp = /^\d{6}$/.test(credential);
	  const ticket = await adminRequest('/admin/v1/reauthentication', {
		method: 'POST',
		body: JSON.stringify(totp ? { method: 'totp', totp_code: credential, binding } : { method: 'password', current_password: credential, binding }),
	  }, true);
	  if (!ticket.token) throw new Error('Der Server hat keine gültige Aktionsbestätigung geliefert.');
	  return adminRequest(path, {
		...options,
		headers: { ...(options.headers || {}), 'X-WERK-Reauth-Token': ticket.token },
	  }, true);
	}
    const localizedProblems = {
      'organizational-unit-referenced': 'Die Organisationseinheit besitzt noch aktive oder geplante Organisations-, Zugriffs-, Add-on- oder Rollenverknüpfungen. Beenden Sie diese Verknüpfungen vor der Archivierung.',
      'organizational-unit-inherited-access-conflict': 'Das Verschieben würde die Reichweite einer aktiven oder geplanten, nach unten vererbten Add-on- oder Gruppenzuweisung ändern. Passen Sie zuerst diese Verknüpfung an.',
      'organizational-unit-depth-limit-exceeded': 'Die Änderung würde die unterstützte Organisationstiefe von 64 Ebenen überschreiten. Wählen Sie eine weniger tiefe Struktur.',
      'version-conflict': 'Der Datensatz wurde zwischenzeitlich geändert. Laden Sie die Ansicht neu und versuchen Sie es erneut.',
    };
    throw new Error(localizedProblems[payload.code] || payload.detail || 'Die Verwaltungsaktion ist fehlgeschlagen.');
  }
  return payload;
}

async function latestAdminRequest(key, path) {
  latestRequestControllers.get(key)?.abort();
  const controller = new AbortController();
  latestRequestControllers.set(key, controller);
  try {
    return await adminRequest(path, { signal: controller.signal });
  } finally {
    if (latestRequestControllers.get(key) === controller) latestRequestControllers.delete(key);
  }
}

function requiredArray(payload, key, context) {
  if (!payload || !Array.isArray(payload[key])) {
    throw new Error(`Die Serverantwort für ${context} ist unvollständig.`);
  }
  return payload[key];
}

function replaceOptions(select, items, placeholder, label, options = {}) {
  if (!select) return;
  const current = select.value;
  const placeholderOption = new Option(placeholder, '');
  placeholderOption.disabled = options.placeholderDisabled === true && items.length > 0;
  select.replaceChildren(placeholderOption);
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
    'tenant-manager': 'Unternehmensverantwortlich',
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

function setWorkUserOnboardingMethod(method) {
  workUserForm?.querySelectorAll('[data-onboarding-panel]').forEach((panel) => {
    const active = panel.dataset.onboardingPanel === method;
    panel.hidden = !active;
    panel.querySelectorAll('input').forEach((input) => {
      input.disabled = !active;
      input.required = active;
      if (!active) {
        if (input.type === 'radio' || input.type === 'checkbox') input.checked = false;
        else input.value = '';
      }
    });
  });
}

function resetWorkUserForm() {
  if (!workUserForm) return;
  workUserForm.reset();
  workUserForm.elements.membership_type.value = 'team.member';
  workUserForm.querySelectorAll('[name="onboarding_method"]').forEach((input) => { input.checked = false; });
  setWorkUserOnboardingMethod('');
}

function closeInvitationResult() {
  activeInvitationURL = '';
  const url = invitationResultDialog?.querySelector('[data-invitation-activation-url]');
  const recipient = invitationResultDialog?.querySelector('[data-invitation-recipient]');
  const expires = invitationResultDialog?.querySelector('[data-invitation-expires]');
  const mailto = invitationResultDialog?.querySelector('[data-invitation-mailto]');
  if (url) url.textContent = '';
  if (recipient) recipient.textContent = '—';
  if (expires) expires.textContent = '—';
  if (mailto) mailto.href = 'mailto:';
}

function showInvitationResult(invitation, loginName) {
  if (!invitationResultDialog || !invitation) return false;
  if (invitation.delivery_method !== 'manual-email-draft') return false;
  let activationURL;
  try {
    activationURL = new URL(String(invitation.activation_path || ''), window.location.origin);
  } catch {
    return false;
  }
  if (
    activationURL.origin !== window.location.origin
    || activationURL.pathname !== '/activate'
    || activationURL.search !== ''
    || !/^#token=[A-Za-z0-9_-]{43}$/.test(activationURL.hash)
  ) return false;
  const recipientEmail = String(invitation.recipient_email || '').trim();
  const expiresAt = new Date(invitation.expires_at);
  if (!recipientEmail || Number.isNaN(expiresAt.getTime())) return false;
  activeInvitationURL = activationURL.href;
  invitationResultDialog.querySelector('[data-invitation-recipient]').textContent = recipientEmail;
  invitationResultDialog.querySelector('[data-invitation-expires]').textContent = dateTimeFormatter.format(expiresAt);
  invitationResultDialog.querySelector('[data-invitation-activation-url]').textContent = activeInvitationURL;
  const subject = 'Ihre Einladung zu WERK';
  const body = `Hallo,\n\nfür Ihr WERK-Arbeitskonto ${loginName} wurde ein einmaliger Aktivierungslink erstellt:\n\n${activeInvitationURL}\n\nLegen Sie darüber Ihr eigenes Passwort fest. WERK meldet Sie danach nicht automatisch an.`;
  invitationResultDialog.querySelector('[data-invitation-mailto]').href = `mailto:${encodeURIComponent(recipientEmail)}?subject=${encodeURIComponent(subject)}&body=${encodeURIComponent(body)}`;
  openDialog(invitationResultDialog);
  return true;
}

function showAdminView() {
  if (!adminPlaneReady) {
    document.querySelectorAll('[data-admin-view]').forEach((section) => { section.hidden = true; });
    return;
  }
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
  if (adminMFARecommendation) adminMFARecommendation.hidden = !(adminMFARecommended && view === 'providers');
  if (view === 'organization' && selectedTenantID && loadedUnitTenantID !== selectedTenantID) loadUnits(selectedTenantID).catch((error) => showPageNotice(error.message, 'error'));
  if (view === 'users' && selectedTenantID) Promise.all([
    loadedUnitTenantID === selectedTenantID ? Promise.resolve() : loadUnits(selectedTenantID),
    loadedUserTenantID === selectedTenantID ? Promise.resolve() : loadUsers(selectedTenantID),
  ]).catch((error) => showPageNotice(error.message, 'error'));
  if (view === 'roles' && selectedTenantID && loadedRoleTenantID !== selectedTenantID) loadRoles(selectedTenantID).catch((error) => showPageNotice(error.message, 'error'));
  if (view === 'audit') loadAuditEvents(true).catch((error) => showPageNotice(error.message, 'error'));
  if (view === 'providers') loadIdentityProviders().catch((error) => showPageNotice(error.message, 'error'));
  if (view === 'operations') loadOperationsStatus().catch((error) => showPageNotice(error.message, 'error'));
}

function renderAdminMFARecommendation(session) {
  if (!adminMFARecommendation) return;
  const multiFactor = session?.authentication_assurance === 'multi-factor';
  adminMFARecommended = !multiFactor && session?.mfa_enrollment_recommended === true;
  adminMFARecommendation.hidden = true;
}

async function unlockAdminPlane(session) {
  adminPlaneReady = true;
  renderAdminMFARecommendation(session);
  showAdminView();
  if (adminInitialized) return;
  adminInitialized = true;
  try {
    await loadTenants();
  } catch (error) {
    if (tenantSummary) tenantSummary.textContent = 'Unternehmen konnten nicht geladen werden.';
    showPageNotice(error.message, 'error');
  }
}

window.addEventListener('werk:session-ready', (event) => {
  const session = event.detail;
  if (session?.account_class !== 'admin' || session?.audience !== 'admin') return;
  unlockAdminPlane(session);
});

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

invitationResultDialog?.addEventListener('close', closeInvitationResult);
document.querySelector('[data-copy-invitation-link]')?.addEventListener('click', async (event) => {
  if (!activeInvitationURL) return;
  const button = event.currentTarget;
  try {
    await copyToClipboard(activeInvitationURL);
    button.textContent = 'Link kopiert';
    window.setTimeout(() => { button.textContent = 'Link kopieren'; }, 1800);
  } catch (error) {
    showPageNotice(error.message, 'error');
  }
});

workUserForm?.addEventListener('change', (event) => {
  if (event.target?.name === 'onboarding_method') setWorkUserOnboardingMethod(event.target.value);
});

document.querySelector('[data-open-user-dialog]')?.addEventListener('click', async () => {
  const tenantID = selectedTenantID;
  const openingRevision = tenantContextRevision;
  if (!tenantID) {
    showPageNotice('Wählen Sie zuerst ein Unternehmen aus.', 'error');
    return;
  }
  try {
    resetWorkUserForm();
    workFormTenantSelect.value = tenantID;
    if (loadedUnitTenantID !== tenantID) await loadUnits(tenantID);
    else renderWorkUnitOptions(currentUnits, tenantID);
    if (selectedTenantID !== tenantID || loadedUnitTenantID !== tenantID || tenantContextRevision !== openingRevision) {
      throw new Error('Der Unternehmenskontext hat sich geändert. Öffnen Sie das Formular erneut.');
    }
    openDialog(document.querySelector('[data-user-dialog]'));
  } catch (error) {
    showPageNotice(error.message, 'error');
  }
});

document.querySelector('[data-open-tenant-dialog]')?.addEventListener('click', () => openDialog(document.querySelector('[data-tenant-dialog]')));

document.querySelector('[data-open-tenant-edit-dialog]')?.addEventListener('click', () => {
  const tenant = currentTenants.find((item) => item.id === selectedTenantID);
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
  const tenantID = selectedTenantID;
  const openingRevision = tenantContextRevision;
  if (!tenantID) {
    showPageNotice('Wählen Sie zuerst ein Unternehmen aus.', 'error');
    return;
  }
  try {
    unitFormTenantSelect.value = tenantID;
    if (loadedUnitTenantID !== tenantID) await loadUnits(tenantID);
    else renderParentUnitOptions(currentUnits, tenantID);
    if (selectedTenantID !== tenantID || loadedUnitTenantID !== tenantID || tenantContextRevision !== openingRevision) {
      throw new Error('Der Unternehmenskontext hat sich geändert. Öffnen Sie das Formular erneut.');
    }
    openDialog(document.querySelector('[data-unit-dialog]'));
  } catch (error) {
    showPageNotice(error.message, 'error');
  }
});

document.querySelector('[data-open-role-dialog]')?.addEventListener('click', async () => {
  const tenantID = selectedTenantID;
  const openingRevision = tenantContextRevision;
  if (!tenantID) {
    showPageNotice('Wählen Sie zuerst ein Unternehmen aus.', 'error');
    return;
  }
  try {
    roleFormTenantSelect.value = tenantID;
    if (loadedRoleTenantID !== tenantID) await loadRoles(tenantID);
    if (selectedTenantID !== tenantID || loadedRoleTenantID !== tenantID || tenantContextRevision !== openingRevision) {
      throw new Error('Der Unternehmenskontext hat sich geändert. Öffnen Sie das Formular erneut.');
    }
    renderPermissionChoices();
    openDialog(document.querySelector('[data-role-dialog]'));
  } catch (error) {
    showPageNotice(error.message, 'error');
  }
});

document.querySelector('[data-open-selected-users]')?.addEventListener('click', () => {
  if (!selectedTenantID) return;
  window.location.hash = 'users';
});

function renderCompanyMode(items) {
  const singleCompany = items.length === 1 && items[0].status === 'active';
  const mode = singleCompany ? 'single' : items.length === 0 ? 'empty' : 'multiple';
  document.documentElement.dataset.companyMode = mode;
  const layout = document.querySelector('[data-company-layout]');
  if (layout) layout.dataset.companyMode = mode;
  document.querySelectorAll('[data-company-context-field], [data-company-picker]').forEach((element) => {
    element.hidden = singleCompany;
  });
  document.querySelectorAll('[data-single-company-context]').forEach((element) => {
    element.hidden = !singleCompany;
  });
  const createCompany = document.querySelector('[data-open-tenant-dialog]');
  if (createCompany) createCompany.hidden = items.length > 0;
}

function renderTenants(items) {
  tenantContextControls.forEach((select) => replaceOptions(select, items, 'Unternehmen wählen', (tenant) => tenant.name, { placeholderDisabled: true }));
  replaceOptions(auditTenantSelect, items, 'Alle Unternehmen und Installation', (tenant) => tenant.name);
  if (tenantSummary) tenantSummary.textContent = `${items.length} Unternehmen`;
  renderCompanyMode(items);
  tenantList?.replaceChildren();
  items.forEach((tenant) => {
    const button = document.createElement('button');
    button.type = 'button';
    button.className = 'organization-tenant-item';
    button.dataset.tenantId = tenant.id;
    button.setAttribute('aria-pressed', String(tenant.id === selectedTenantID));
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

function setCompanyContextPresentation(tenant) {
  const tenantID = tenant?.id || '';
  tenantContextControls.forEach((select) => { if (select) select.value = tenantID; });
  document.querySelectorAll('[data-tenant-id]').forEach((button) => {
    const active = button.dataset.tenantId === tenantID;
    button.classList.toggle('is-active', active);
    button.setAttribute('aria-pressed', String(active));
  });
  document.querySelectorAll('[data-company-context-name]').forEach((element) => {
    element.textContent = tenant?.name || 'Kein Unternehmen ausgewählt';
  });
  const selectedName = document.querySelector('[data-selected-tenant-name]');
  if (selectedName) selectedName.textContent = tenant?.name || 'Organisationseinheiten';
  const activeCompany = tenant?.status === 'active';
  document.querySelectorAll('[data-requires-company-context]').forEach((control) => { control.disabled = !activeCompany; });
  const addUnit = document.querySelector('[data-open-unit-dialog]');
  if (addUnit) addUnit.disabled = !activeCompany;
  const showUsers = document.querySelector('[data-open-selected-users]');
  if (showUsers) showUsers.disabled = !tenant;
  const editTenant = document.querySelector('[data-open-tenant-edit-dialog]');
  if (editTenant) editTenant.disabled = !tenant;
}

function clearTenantBoundViews() {
  unitLoadSequence += 1;
  userLoadSequence += 1;
  roleLoadSequence += 1;
  currentUnits = [];
  currentUsers = [];
  currentRoles = [];
  currentPermissions = [];
  loadedUnitTenantID = '';
  loadedUserTenantID = '';
  loadedRoleTenantID = '';
  userPage = 1;
  renderUnits([]);
  renderUsers();
  renderRoles();
  replaceOptions(parentUnitSelect, [], 'Keine – direkt unter dem Unternehmen', (unit) => unit.name);
  replaceOptions(workUnitSelect, [], 'Einheit wählen', (unit) => unit.name);
  if (unitSummary) unitSummary.textContent = 'Unternehmen auswählen, um Einheiten anzuzeigen.';
  if (userSummary) userSummary.textContent = 'Unternehmen auswählen, um Konten anzuzeigen.';
  const refreshed = document.querySelector('[data-user-refreshed]');
  if (refreshed) refreshed.textContent = 'Kein Unternehmen ausgewählt';
  const permissionTotal = document.querySelector('[data-role-permission-total]');
  if (permissionTotal) permissionTotal.textContent = 'Unternehmen auswählen';
}

function clearOrganizationTenant() {
  selectedTenantID = '';
  tenantContextRevision += 1;
  setCompanyContextPresentation(null);
  clearTenantBoundViews();
}

function resolveInitialTenantID(items, currentTenantID) {
  if (items.some((item) => item.id === currentTenantID)) return currentTenantID;
  const activeItems = items.filter((item) => item.status === 'active');
  return activeItems.length === 1 ? activeItems[0].id : '';
}

async function loadSelectedCompanyView(tenantID, revision) {
  const view = window.location.hash.slice(1) || 'users';
  const loads = [];
  if (view === 'organization' && loadedUnitTenantID !== tenantID) loads.push(loadUnits(tenantID, revision));
  if (view === 'users') {
    if (loadedUnitTenantID !== tenantID) loads.push(loadUnits(tenantID, revision));
    if (loadedUserTenantID !== tenantID) loads.push(loadUsers(tenantID, revision));
  }
  if (view === 'roles' && loadedRoleTenantID !== tenantID) loads.push(loadRoles(tenantID, revision));
  await Promise.all(loads);
}

async function selectOrganizationTenant(tenantID) {
  const tenant = currentTenants.find((item) => item.id === tenantID);
  if (!tenant) {
    clearOrganizationTenant();
    return;
  }
  if (selectedTenantID === tenantID) {
    setCompanyContextPresentation(tenant);
    await loadSelectedCompanyView(tenantID, tenantContextRevision);
    return;
  }
  selectedTenantID = tenantID;
  const revision = ++tenantContextRevision;
  setCompanyContextPresentation(tenant);
  clearTenantBoundViews();
  setCompanyContextPresentation(tenant);
  await loadSelectedCompanyView(tenantID, revision);
}

async function loadTenants() {
  const loadSequence = ++tenantLoadSequence;
  let payload;
  try {
    payload = await adminRequest('/admin/v1/tenants');
  } catch (error) {
    if (loadSequence !== tenantLoadSequence) return;
    throw error;
  }
  if (loadSequence !== tenantLoadSequence) return;
  const items = requiredArray(payload, 'items', 'Unternehmen');
  currentTenants = items;
  renderTenants(items);
  const selected = resolveInitialTenantID(items, selectedTenantID);
  if (!selected) {
    clearOrganizationTenant();
    if (!items.length && ['', 'users', 'roles'].includes(window.location.hash.slice(1))) {
      window.location.hash = 'organization';
    }
    return;
  }
  await selectOrganizationTenant(selected);
}

function renderUnits(items) {
  if (unitSummary) unitSummary.textContent = `${items.length} Organisationseinheit${items.length === 1 ? '' : 'en'}`;
  if (unitEmpty) {
    const contextLoaded = Boolean(selectedTenantID && loadedUnitTenantID === selectedTenantID);
    unitEmpty.hidden = items.length !== 0;
    unitEmpty.querySelector('[data-unit-empty-title]').textContent = contextLoaded
      ? 'Keine Organisationseinheiten vorhanden'
      : selectedTenantID ? 'Unternehmensdaten werden geladen' : 'Kein Unternehmen ausgewählt';
    unitEmpty.querySelector('[data-unit-empty-copy]').textContent = contextLoaded
      ? 'Legen Sie die erste Einheit innerhalb dieses Unternehmens an.'
      : selectedTenantID ? 'Der serverseitig bestätigte Organisationskontext wird abgerufen.' : 'Legen Sie zuerst das Unternehmen an oder wählen Sie einen vorhandenen Kontext.';
  }
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
  const payload = await latestAdminRequest('units', `/admin/v1/tenants/${encodeURIComponent(tenantID)}/organizational-units`);
  return requiredArray(payload, 'items', 'Organisationseinheiten');
}

function renderParentUnitOptions(items, tenantID) {
  if (unitFormTenantSelect?.value !== tenantID) return;
  replaceOptions(parentUnitSelect, items.filter((unit) => unit.status === 'active'), 'Keine – direkt unter dem Unternehmen', (unit) => `${unit.name} (${unitTypeLabel(unit.unit_type)})`);
}

function renderWorkUnitOptions(items, tenantID) {
  if (workFormTenantSelect?.value !== tenantID) return;
  replaceOptions(workUnitSelect, items.filter((unit) => unit.status === 'active'), 'Einheit wählen', (unit) => `${unit.name} (${unitTypeLabel(unit.unit_type)})`);
}

async function loadUnits(tenantID, revision = tenantContextRevision) {
  const loadSequence = ++unitLoadSequence;
  if (!tenantID) {
    currentUnits = [];
    loadedUnitTenantID = '';
    renderUnits([]);
    if (unitSummary) unitSummary.textContent = 'Unternehmen auswählen, um Einheiten anzuzeigen.';
    return;
  }
  loadedUnitTenantID = '';
  let items;
  try {
    items = await fetchUnits(tenantID);
  } catch (error) {
    if (tenantID !== selectedTenantID || revision !== tenantContextRevision || loadSequence !== unitLoadSequence) return;
    throw error;
  }
  if (tenantID !== selectedTenantID || revision !== tenantContextRevision || loadSequence !== unitLoadSequence) return;
  currentUnits = items;
  loadedUnitTenantID = tenantID;
  renderUnits(items);
  renderParentUnitOptions(items, tenantID);
  renderWorkUnitOptions(items, tenantID);
}

function openUnitEdit(unit) {
  unitEditForm.elements.tenant_id.value = unit.tenant_id;
  unitEditForm.elements.unit_id.value = unit.id;
  unitEditForm.elements.version.value = unit.version;
  unitEditForm.elements.name.value = unit.name;
  unitEditForm.elements.unit_type.value = unit.unit_type;
  unitEditForm.elements.status.value = unit.status;
  const parentSelect = unitEditForm.querySelector('[data-unit-edit-parent-select]');
  replaceOptions(parentSelect, currentUnits.filter((candidate) => candidate.id !== unit.id && candidate.status === 'active'), 'Keine – direkt unter dem Unternehmen', (candidate) => `${candidate.name} (${unitTypeLabel(candidate.unit_type)})`);
  parentSelect.value = unit.parent_id || '';
  openDialog(document.querySelector('[data-unit-edit-dialog]'));
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
  if (roleEmpty) {
    const contextLoaded = Boolean(selectedTenantID && loadedRoleTenantID === selectedTenantID);
    roleEmpty.hidden = currentRoles.length !== 0;
    roleEmpty.querySelector('[data-role-empty-title]').textContent = contextLoaded
      ? 'Keine Rollen vorhanden'
      : selectedTenantID ? 'Rollen werden geladen' : 'Kein Unternehmen ausgewählt';
    roleEmpty.querySelector('[data-role-empty-copy]').textContent = contextLoaded
      ? 'Legen Sie bei Bedarf eine eigene Work-Rolle für dieses Unternehmen an.'
      : selectedTenantID ? 'Der serverseitig bestätigte Rollenkontext wird abgerufen.' : 'Wählen Sie ein Unternehmen, um dessen Work-Rollen anzuzeigen.';
  }
  document.querySelector('[data-role-total]').textContent = String(currentRoles.length);
  document.querySelector('[data-role-system]').textContent = String(currentRoles.filter((role) => role.system_role).length);
  document.querySelector('[data-role-custom]').textContent = String(currentRoles.filter((role) => !role.system_role).length);
  document.querySelector('[data-role-permission-total]').textContent = `${currentPermissions.length} registrierte Work-Berechtigung${currentPermissions.length === 1 ? '' : 'en'}`;
  const tenant = currentTenants.find((item) => item.id === roleTenantSelect.value);
  document.querySelector('[data-role-tenant-name]').textContent = tenant?.name || 'Kein Unternehmen ausgewählt';
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
    accounts.addEventListener('click', () => {
      if (selectedTenantID !== role.tenant_id) {
        showPageNotice('Der Unternehmenskontext hat sich geändert. Öffnen Sie die Rollenansicht erneut.', 'error');
        return;
      }
      userSearch.value = role.role_key;
      window.location.hash = 'users';
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

async function loadRoles(tenantID, revision = tenantContextRevision) {
  const loadSequence = ++roleLoadSequence;
  if (!tenantID) {
    currentRoles = [];
    currentPermissions = [];
    loadedRoleTenantID = '';
    renderRoles();
    return;
  }
  loadedRoleTenantID = '';
  let payload;
  try {
    payload = await latestAdminRequest('roles', `/admin/v1/work-roles?tenant_id=${encodeURIComponent(tenantID)}`);
  } catch (error) {
    if (tenantID !== selectedTenantID || revision !== tenantContextRevision || loadSequence !== roleLoadSequence) return;
    throw error;
  }
  if (tenantID !== selectedTenantID || revision !== tenantContextRevision || loadSequence !== roleLoadSequence) return;
  const roles = requiredArray(payload, 'roles', 'Work-Rollen');
  const permissions = requiredArray(payload, 'permissions', 'Work-Berechtigungen');
  currentRoles = roles;
  currentPermissions = permissions;
  loadedRoleTenantID = tenantID;
  roleTenantSelect.value = tenantID;
  roleFormTenantSelect.value = tenantID;
  renderRoles();
}

async function openRoleAssignment(user) {
  const openingRevision = tenantContextRevision;
  if (selectedTenantID !== user.tenant_id) {
    throw new Error('Der Unternehmenskontext hat sich geändert. Öffnen Sie die Rollenzuweisung erneut.');
  }
  if (loadedRoleTenantID !== user.tenant_id) await loadRoles(user.tenant_id);
  if (selectedTenantID !== user.tenant_id || loadedRoleTenantID !== user.tenant_id || tenantContextRevision !== openingRevision) {
    throw new Error('Der Unternehmenskontext hat sich geändert. Öffnen Sie die Rollenzuweisung erneut.');
  }
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
  const tenant = currentTenants.find((item) => item.id === user.tenant_id);
  const invitationExpiry = user.invitation_expires_at ? new Date(user.invitation_expires_at) : null;
  const invitationExpired = user.invitation_pending && invitationExpiry && !Number.isNaN(invitationExpiry.getTime()) && invitationExpiry.getTime() <= Date.now();
  dialog.querySelector('[data-details-name]').textContent = user.display_name;
  dialog.querySelector('[data-details-login]').textContent = user.login_name;
  dialog.querySelector('[data-details-tenant]').textContent = tenant ? `${tenant.name} · ${shortIdentifier(tenant.id)}` : user.tenant_id;
  const accessState = user.invitation_pending
    ? invitationExpired ? 'Einladung abgelaufen' : 'Aktivierung ausstehend'
    : statusLabel(user.status);
  dialog.querySelector('[data-details-status]').textContent = user.invitation_pending
    ? invitationExpired ? 'Die Einladung ist abgelaufen. Vor der Anmeldung muss ein neuer Link ausgegeben werden.' : 'Das Konto wartet auf die erstmalige Aktivierung.'
    : user.status === 'active'
      ? user.must_change_password ? 'Das Konto ist aktiv. Bei der nächsten Anmeldung muss das Passwort geändert werden.' : 'Das Konto ist aktiv und kann sich anmelden.'
      : user.status === 'disabled' ? 'Der Zugang ist deaktiviert. Eine Anmeldung ist derzeit nicht möglich.' : `Kontostatus: ${statusLabel(user.status)}.`;
  const statusBadge = dialog.querySelector('[data-details-status-badge]');
  statusBadge.textContent = accessState;
  statusBadge.className = `status-badge ${user.invitation_pending ? invitationExpired ? 'status-disabled' : 'status-warning' : `status-${user.status}`}`;
  dialog.querySelector('[data-details-onboarding]').textContent = user.invitation_pending
    ? `Einladungslink ${invitationExpired ? 'abgelaufen' : 'offen'}${invitationExpiry && !Number.isNaN(invitationExpiry.getTime()) ? ` bis ${new Intl.DateTimeFormat('de-DE', { dateStyle: 'medium', timeStyle: 'short' }).format(invitationExpiry)}` : ''}`
    : user.must_change_password ? 'Startpasswort · Wechsel erforderlich' : 'Abgeschlossen';
  dialog.querySelector('[data-details-unit]').textContent = user.organizational_unit_name || 'Keine Einheit';
  dialog.querySelector('[data-details-membership]').textContent = membershipLabel(user.membership_type);
  dialog.querySelector('[data-details-roles]').textContent = user.roles.length ? user.roles.map(roleLabel).join(', ') : 'Keine Rollen';
  dialog.querySelector('[data-details-version]').textContent = String(user.version);
  dialog.querySelector('[data-details-account-id]').textContent = user.account_id;
  dialog.querySelector('[data-details-party-id]').textContent = user.party_id;
  for (const selector of ['[data-details-authentication]', '[data-details-sessions]', '[data-details-activity]']) {
    dialog.querySelector(selector).replaceChildren(supportState('Wird geladen …'));
  }
  dialog.querySelector('[data-manage-details-roles]').dataset.accountId = user.account_id;
  const sessionsButton = dialog.querySelector('[data-manage-details-sessions]');
  sessionsButton.dataset.accountId = user.account_id;
  sessionsButton.disabled = true;
  const statusButton = dialog.querySelector('[data-manage-details-status]');
  statusButton.dataset.accountId = user.account_id;
  statusButton.disabled = user.invitation_pending || !['active', 'disabled'].includes(user.status);
  statusButton.textContent = user.invitation_pending ? 'Bis zur Aktivierung gesperrt' : user.status === 'active' ? 'Zugang deaktivieren' : user.status === 'disabled' ? 'Zugang aktivieren' : 'Status durch Identity geschützt';
  const invitationButton = dialog.querySelector('[data-manage-details-invitation]');
  invitationButton.dataset.accountId = user.account_id;
  invitationButton.hidden = !user.invitation_pending;
  openDialog(dialog);
  loadWorkUserIdentity(user, dialog).catch((error) => {
    if (dialog.open && dialog.dataset.identityAccountId === user.account_id) {
      for (const selector of ['[data-details-authentication]', '[data-details-sessions]', '[data-details-activity]']) {
        dialog.querySelector(selector).replaceChildren(supportState(`Supportdaten konnten nicht geladen werden: ${error.message}`, true));
      }
    }
  });
}

function supportState(message, failed = false) {
  const state = document.createElement('p');
  state.className = 'support-state';
  if (failed) state.classList.add('is-error');
  state.textContent = message;
  return state;
}

function supportList(items) {
  const list = document.createElement('ul');
  list.className = 'support-list';
  for (const item of items) {
    const row = document.createElement('li');
    const title = document.createElement('strong');
    title.textContent = item.title;
    const detail = document.createElement('span');
    detail.textContent = item.detail;
    row.append(title, detail);
    list.append(row);
  }
  return list;
}

async function loadWorkUserIdentity(user, dialog) {
  dialog.dataset.identityAccountId = user.account_id;
  const identityView = await adminRequest(`/admin/v1/tenants/${encodeURIComponent(user.tenant_id)}/work-users/${encodeURIComponent(user.account_id)}/identity`);
  if (!dialog.open || dialog.dataset.identityAccountId !== user.account_id) return;
  const authentication = [];
  if (identityView.password) authentication.push({ title: 'Passwort', detail: `Aktiv · geändert ${auditTimestamp(identityView.password.changed_at)}${identityView.password.last_used_at ? ` · zuletzt verwendet ${auditTimestamp(identityView.password.last_used_at)}` : ''}` });
  for (const factor of identityView.factors || []) authentication.push({
    title: factor.kind === 'webauthn' ? `Passkey · ${factor.display_name}` : `Authenticator-App · ${factor.display_name}`,
    detail: `${factor.status === 'active' ? 'Aktiv' : 'Einrichtung offen'}${factor.last_used_at ? ` · zuletzt verwendet ${auditTimestamp(factor.last_used_at)}` : ''}`,
  });
  dialog.querySelector('[data-details-authentication]').replaceChildren(authentication.length ? supportList(authentication) : supportState('Noch kein nutzbares Anmeldeverfahren hinterlegt.'));
  const sessions = (identityView.active_sessions || []).map((session) => ({ title: `${session.assurance === 'multi-factor' ? 'Mehrstufig bestätigt' : 'Einfach bestätigt'} · ${shortIdentifier(session.id)}`, detail: `Begonnen ${auditTimestamp(session.created_at)} · gültig bis ${auditTimestamp(session.expires_at)}` }));
  dialog.querySelector('[data-details-sessions]').replaceChildren(sessions.length ? supportList(sessions) : supportState('Keine aktive Anmeldung.'));
  const sessionsButton = dialog.querySelector('[data-manage-details-sessions]');
  sessionsButton.disabled = sessions.length === 0;
  sessionsButton.textContent = sessions.length === 0 ? 'Keine aktive Sitzung' : sessions.length === 1 ? '1 Sitzung beenden' : `${sessions.length} Sitzungen beenden`;
  const activity = (identityView.recent_activity || []).map((item) => ({ title: `${auditEventLabel(item.event_type)} · ${auditOutcomeLabel(item.outcome)}`, detail: `${auditTimestamp(item.occurred_at)} · ${shortIdentifier(item.id)}` }));
  dialog.querySelector('[data-details-activity]').replaceChildren(activity.length ? supportList(activity) : supportState('Keine kontobezogenen Sicherheitsvorgänge vorhanden.'));
}

function openInvitationReissue(user) {
  if (!invitationReissueForm || !user.invitation_pending) return;
  invitationReissueForm.reset();
  invitationReissueForm.elements.tenant_id.value = user.tenant_id;
  invitationReissueForm.elements.account_id.value = user.account_id;
  invitationReissueForm.elements.login_name.value = user.login_name;
  document.querySelector('[data-invitation-reissue-login]').textContent = `${user.display_name} · ${user.login_name}`;
  openDialog(document.querySelector('[data-invitation-reissue-dialog]'));
}

function openUserStatus(user) {
  if (!userStatusForm || user.invitation_pending || !['active', 'disabled'].includes(user.status)) return;
  const nextStatus = user.status === 'active' ? 'disabled' : 'active';
  userStatusForm.elements.tenant_id.value = user.tenant_id;
  userStatusForm.elements.account_id.value = user.account_id;
  userStatusForm.elements.version.value = user.version;
  userStatusForm.elements.status.value = nextStatus;
  document.querySelector('[data-user-status-title]').textContent = nextStatus === 'disabled' ? 'Zugang deaktivieren' : 'Zugang aktivieren';
  document.querySelector('[data-user-status-login]').textContent = `${user.display_name} · ${user.login_name}`;
  document.querySelector('[data-user-status-confirmation]').textContent = nextStatus === 'disabled'
    ? 'Das Arbeitskonto kann sich danach nicht mehr anmelden'
    : 'Das Arbeitskonto erhält den Anmeldezugang zurück';
  document.querySelector('[data-user-status-submit]').textContent = nextStatus === 'disabled' ? 'Zugang deaktivieren' : 'Zugang aktivieren';
  openDialog(document.querySelector('[data-user-status-dialog]'));
}

function renderUsers() {
  const query = userSearch?.value.trim().toLocaleLowerCase('de') || '';
  const filtered = currentUsers.filter((user) => !query || [user.display_name, user.login_name, user.organizational_unit_name, user.membership_type, ...(user.roles || [])].join(' ').toLocaleLowerCase('de').includes(query));
  filtered.sort((left, right) => userSort.direction * userCollator.compare(String(left[userSort.key] || ''), String(right[userSort.key] || '')));
  const pageCount = Math.max(1, Math.ceil(filtered.length / userPageSize));
  userPage = Math.min(userPage, pageCount);
  const start = (userPage - 1) * userPageSize;
  const items = filtered.slice(start, start + userPageSize);
  if (userSummary) userSummary.textContent = query ? `${filtered.length} von ${currentUsers.length} Benutzern` : `${filtered.length} Benutzer`;
  if (userEmpty) {
    const contextLoaded = Boolean(selectedTenantID && loadedUserTenantID === selectedTenantID);
    userEmpty.hidden = filtered.length !== 0;
    userEmpty.querySelector('[data-user-empty-title]').textContent = contextLoaded
      ? 'Keine Benutzer vorhanden'
      : selectedTenantID ? 'Arbeitskonten werden geladen' : 'Kein Unternehmen ausgewählt';
    userEmpty.querySelector('[data-user-empty-copy]').textContent = contextLoaded
      ? 'Legen Sie das erste Arbeitskonto für dieses Unternehmen an.'
      : selectedTenantID ? 'Der serverseitig bestätigte Kontenkontext wird abgerufen.' : 'Wählen Sie ein Unternehmen, um Arbeitskonten anzuzeigen.';
  }
  document.querySelector('[data-user-total]').textContent = String(currentUsers.length);
  document.querySelector('[data-user-active]').textContent = String(currentUsers.filter((user) => user.status === 'active').length);
  document.querySelector('[data-user-onboarding]').textContent = String(currentUsers.filter((user) => user.must_change_password || user.invitation_pending).length);
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
    const invitationExpiry = user.invitation_expires_at ? new Date(user.invitation_expires_at) : null;
    const invitationExpired = user.invitation_pending && invitationExpiry && !Number.isNaN(invitationExpiry.getTime()) && invitationExpiry.getTime() <= Date.now();
    status.className = `status-badge ${user.invitation_pending ? invitationExpired ? 'status-disabled' : 'status-warning' : `status-${user.status}`}`;
    status.textContent = user.invitation_pending ? invitationExpired ? 'Einladung abgelaufen' : 'Einladung offen' : statusLabel(user.status);
    const security = document.createElement('span');
    security.className = `status-badge ${user.invitation_pending ? invitationExpired ? 'status-disabled' : 'status-warning' : user.must_change_password ? 'status-warning' : 'status-active'}`;
    security.textContent = user.invitation_pending ? invitationExpired ? 'Neu-Ausgabe möglich' : 'Aktivierung ausstehend' : user.must_change_password ? 'Passwortwechsel' : 'Eingerichtet';
    if (user.invitation_pending && invitationExpiry && !Number.isNaN(invitationExpiry.getTime())) {
      security.title = `Einladung gültig bis ${dateTimeFormatter.format(invitationExpiry)}`;
    }
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
    const reissueInvitation = document.createElement('button');
    reissueInvitation.type = 'button';
    reissueInvitation.className = 'button button-compact';
    reissueInvitation.textContent = 'Einladung neu';
    reissueInvitation.hidden = !user.invitation_pending;
    reissueInvitation.addEventListener('click', () => openInvitationReissue(user));
    const actions = document.createElement('div');
    actions.className = 'row-actions';
    actions.append(reissueInvitation, manageRoles, details);
    const organization = document.createElement('div');
    organization.className = 'table-primary';
    const tenant = currentTenants.find((item) => item.id === user.tenant_id);
    const tenantName = document.createElement('strong');
    tenantName.textContent = tenant?.name || shortIdentifier(user.tenant_id);
    tenantName.title = user.tenant_id;
    const unitName = document.createElement('span');
    unitName.textContent = user.organizational_unit_name || 'Keine Organisationseinheit';
    organization.append(tenantName, unitName);
    row.append(createCell(identity), createCell(organization), createCell(roles), createCell(status), createCell(security), createCell(actions, 'table-actions'));
    userList?.append(row);
  });
}

async function loadUsers(tenantID, revision = tenantContextRevision) {
  const loadSequence = ++userLoadSequence;
  if (!tenantID) {
    currentUsers = [];
    loadedUserTenantID = '';
    userPage = 1;
    renderUsers();
    if (userSummary) userSummary.textContent = 'Unternehmen auswählen, um Konten anzuzeigen.';
    document.querySelector('[data-user-refreshed]').textContent = 'Kein Unternehmen ausgewählt';
    return;
  }
  loadedUserTenantID = '';
  let payload;
  try {
    payload = await latestAdminRequest('users', `/admin/v1/work-users?tenant_id=${encodeURIComponent(tenantID)}`);
  } catch (error) {
    if (tenantID !== selectedTenantID || revision !== tenantContextRevision || loadSequence !== userLoadSequence) return;
    throw error;
  }
  if (tenantID !== selectedTenantID || revision !== tenantContextRevision || loadSequence !== userLoadSequence) return;
  currentUsers = requiredArray(payload, 'items', 'Arbeitskonten');
  loadedUserTenantID = tenantID;
  userPage = 1;
  document.querySelector('[data-user-refreshed]').textContent = `Aktualisiert um ${timeFormatter.format(new Date())} Uhr`;
  renderUsers();
}

function auditOutcomeLabel(outcome) {
  return ({ succeeded: 'Erfolgreich', denied: 'Abgelehnt', failed: 'Fehlgeschlagen' })[outcome] || outcome;
}

function accountClassLabel(accountClass) {
  return ({ work: 'Arbeitskonto', admin: 'Administrationskonto', service: 'Dienstkonto' })[accountClass] || 'Nicht zugeordnet';
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

function auditEventLabel(eventType) {
  return ({
    'identity.login.succeeded.v1': 'Anmeldung erfolgreich',
    'identity.login.denied.v1': 'Anmeldung abgelehnt',
    'identity.login.throttled.v1': 'Anmeldung begrenzt',
    'identity.password.changed.v1': 'Passwort geändert',
    'identity.session.rotated.v1': 'Sitzungen erneuert',
    'identity.work-account-sessions-revoked.v1': 'Sitzungen administrativ beendet',
    'identity.logout.succeeded.v1': 'Abmeldung erfolgreich',
    'identity.work-account.created.v1': 'Arbeitskonto angelegt',
    'identity.work-account.listed.v1': 'Arbeitskonten abgerufen',
    'authorization.work-role.catalog-listed.v1': 'Rollenkatalog abgerufen',
    'core.identity.providers-listed.v1': 'Identitätsanbieter abgerufen',
    'core.platform.provider-registry-listed.v1': 'Provider-Registry abgerufen',
    'core.platform.operations-summary-read.v1': 'Plattformstatus abgerufen',
  })[eventType] || 'Technisches Audit-Ereignis';
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
    eventName.textContent = auditEventLabel(item.event_type);
    const correlation = document.createElement('span');
    correlation.textContent = item.event_type;
    correlation.title = `Korrelation ${item.correlation_id}`;
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
  const loadSequence = ++auditLoadSequence;
  const parameters = new URLSearchParams({ limit: '50' });
  if (auditTenantSelect?.value) parameters.set('tenant_id', auditTenantSelect.value);
  if (auditOutcomeSelect?.value) parameters.set('outcome', auditOutcomeSelect.value);
  if (auditEventType?.value.trim()) parameters.set('event_type', auditEventType.value.trim());
  if (!reset) parameters.set('cursor', nextAuditCursor);
  const payload = await adminRequest(`/admin/v1/security-audit?${parameters.toString()}`);
  if (loadSequence !== auditLoadSequence) return;
  const items = requiredArray(payload, 'items', 'Audit-Ereignisse');
  currentAuditEvents = reset ? items : currentAuditEvents.concat(items);
  nextAuditCursor = payload.next_cursor || '';
  renderAuditEvents();
  document.querySelector('[data-audit-refreshed]').textContent = `Aktualisiert um ${timeFormatter.format(new Date())} Uhr`;
  const scope = auditTenantSelect?.value ? auditTenantSelect.selectedOptions[0]?.textContent : 'Installation und alle Unternehmen';
  document.querySelector('[data-audit-summary]').textContent = `${currentAuditEvents.length} Ereignis${currentAuditEvents.length === 1 ? '' : 'se'} · ${scope}`;
}

function operationsStateLabel(state) {
  return ({ ready: 'Bereit', degraded: 'Eingeschränkt', attention: 'Prüfung nötig', disabled: 'Deaktiviert', unknown: 'Unbekannt' })[state] || state;
}

function operationsStateClass(state) {
  return ({ ready: 'is-ready', degraded: 'is-warning', attention: 'is-error', disabled: 'is-disabled', unknown: 'is-unknown', available: 'is-ready', 'protocol-implemented': 'is-warning', 'not-ready': 'is-warning', 'adapter-unavailable': 'is-disabled' })[state] || 'is-unknown';
}

function statusBadgeClass(state) {
  return state === 'ready' || state === 'available' ? 'status-active' : state === 'degraded' || state === 'unknown' ? 'status-warning' : state === 'attention' ? 'status-disabled' : '';
}

function appendOperationsRow(container, state, title, description, value) {
  const row = document.createElement('div');
  const dot = document.createElement('span');
  dot.className = `operations-status-dot ${operationsStateClass(state)}`;
  dot.setAttribute('aria-hidden', 'true');
  const content = document.createElement('div');
  const name = document.createElement('strong');
  name.textContent = title;
  const detail = document.createElement('small');
  detail.textContent = description;
  content.append(name, detail);
  const status = document.createElement('span');
  status.textContent = value;
  row.append(dot, content, status);
  container?.append(row);
}

function formatDuration(seconds) {
  const total = Math.max(0, Number(seconds) || 0);
  const days = Math.floor(total / 86400);
  const hours = Math.floor((total % 86400) / 3600);
  const minutes = Math.floor((total % 3600) / 60);
  if (days) return `${days} T ${hours} Std`;
  if (hours) return `${hours} Std ${minutes} Min`;
  return `${minutes} Min`;
}

function componentPresentation(component) {
  const definitions = {
    api: ['API-Prozess', `Build ${component.build_version || '—'} · aktuelle Anfrage beantwortet`],
    postgresql: ['PostgreSQL', 'Administrative Projektion erfolgreich gelesen'],
    worker: ['Hintergrundverarbeitung', component.last_observed_at ? `Worker zuletzt ${auditTimestamp(component.last_observed_at)}${component.build_version ? ` · Build ${component.build_version}` : ''}` : 'Kein aktuelles Lebenszeichen des Workers'],
    kafka: ['Kafka', component.state === 'disabled' ? 'Im aktuellen Betriebsprofil deaktiviert' : 'Konfiguriert, aber nicht direkt durch diese Projektion geprüft'],
  };
  return definitions[component.key] || [component.key, 'Registrierte Plattformkomponente'];
}

async function loadIdentityProviders() {
  const [payload, registry] = await Promise.all([
    adminRequest('/admin/v1/identity/providers'),
    adminRequest('/admin/v1/provider-registry'),
  ]);
  const providers = requiredArray(payload, 'items', 'Identitätsprovider');
  const providerAdapters = requiredArray(payload, 'adapters', 'Provider-Adapter');
  const services = requiredArray(registry, 'services', 'Service-Registry');
  const registrations = requiredArray(registry, 'providers', 'Provider-Registry');
  const localProvider = providers.find((provider) => provider.provider_key === 'local');
  const localState = localProvider?.sign_in_state || 'unknown';
  const localBadge = document.querySelector('[data-provider-local-state]');
  localBadge.className = `status-badge ${statusBadgeClass(localState)}`;
  localBadge.textContent = localState === 'available' ? 'Aktiv' : localState === 'disabled' ? 'Deaktiviert' : 'Unbekannt';
  document.querySelector('[data-provider-local-methods]').textContent = (payload.local?.methods || []).map((method) => ({ password: 'Passwort', passkey: 'Passkey' })[method] || method).join(', ') || '—';
  document.querySelector('[data-provider-local-mfa]').textContent = payload.local?.mfa_enabled ? 'Aktiv' : 'Deaktiviert';
  document.querySelector('[data-provider-local-rp-id]').textContent = payload.local?.webauthn_rp_id || '—';
  document.querySelector('[data-provider-local-rp-name]').textContent = payload.local?.webauthn_rp_name || '—';
  document.querySelector('[data-provider-local-origins]').textContent = String(payload.local?.allowed_origin_count ?? 0);
  document.querySelector('[data-provider-local-source]').textContent = payload.local?.configuration_source === 'runtime-environment' ? 'Laufzeitkonfiguration' : payload.local?.configuration_source || '—';

  const providerList = document.querySelector('[data-provider-list]');
  providerList?.replaceChildren();
  providers.forEach((provider) => {
    const state = provider.sign_in_state;
    const kind = provider.provider_kind.toUpperCase();
    const issuer = provider.issuer ? ` · ${provider.issuer}` : '';
    const label = state === 'available' ? 'Anmeldebereit' : state === 'disabled' ? 'Deaktiviert' : state === 'not-ready' ? 'Nicht anmeldebereit' : 'Unbekannt';
    appendOperationsRow(providerList, state, provider.display_name, `${kind} · ${provider.configuration_source}${issuer}`, label);
  });
  document.querySelector('[data-provider-count]').textContent = `${providers.length}${payload.truncated ? '+' : ''} registriert`;

  const serviceRegistry = document.querySelector('[data-service-registry-list]');
  serviceRegistry?.replaceChildren();
  services.forEach((service) => {
    const capabilities = Array.isArray(service.capabilities) ? service.capabilities : [];
    const providerCount = registrations.filter((provider) => (
      provider.service_key === service.service_key
      && provider.service_contract_version === service.contract_version
    )).length;
    const state = service.lifecycle === 'active' ? 'available' : 'disabled';
    const serviceName = ({
      'core.identity.service.directory': 'Identitätsverzeichnis',
      'core.identity.service.login-federation': 'Externe Anmeldung',
    })[service.service_key] || 'Technischer Dienst';
    appendOperationsRow(
      serviceRegistry,
      state,
      serviceName,
      `${service.service_key} · Vertrag v${service.contract_version} · ${capabilities.length} Funktionen · ${providerCount} Provider`,
      statusLabel(service.lifecycle),
    );
  });
  if (!services.length) appendOperationsRow(serviceRegistry, 'unknown', 'Keine Service-Verträge', 'Die Registry enthält noch keine Dienste.', 'Leer');
  document.querySelector('[data-service-registry-count]').textContent = `${services.length}${registry.truncated ? '+' : ''} Dienste`;

  const adapters = document.querySelector('[data-provider-adapters]');
  adapters?.replaceChildren();
  providerAdapters.forEach((adapter) => appendOperationsRow(
    adapters,
    adapter.state,
    adapter.provider_kind.toUpperCase(),
    adapter.implemented ? 'Protokollprüfung implementiert; Laufzeitverbindung separat prüfen' : 'Noch kein ausführbarer Prüfvertrag',
    adapter.state === 'protocol-implemented' ? 'Protokoll vorhanden' : 'Nicht verfügbar',
  ));
  const management = document.querySelector('[data-provider-management-state]');
  management.className = `status-badge ${payload.management?.external_configuration_available ? 'status-active' : 'status-warning'}`;
  management.textContent = payload.management?.external_configuration_available ? 'Verfügbar' : 'Nicht verfügbar';
  document.querySelector('[data-provider-management-message]').textContent = payload.management?.external_configuration_available
    ? 'Externe Provider können über den versionierten Administrationsvertrag konfiguriert werden.'
    : 'Externe Identitätsanbieter bleiben schreibgeschützt, bis Konfiguration, Secret-Port, Kontobindung und Laufzeitverbindung vollständig sind.';
}

async function loadOperationsStatus() {
  const overall = document.querySelector('[data-operations-overall]');
  const checked = document.querySelector('[data-operations-checked]');
  if (!overall || !checked) return;
  overall.className = 'status-badge';
  overall.textContent = 'Wird geprüft';
  const payload = await adminRequest('/admin/v1/operations/summary');
  const operationComponents = requiredArray(payload, 'components', 'Plattformkomponenten');
  const operationQueues = requiredArray(payload, 'queues', 'Betriebswarteschlangen');
  overall.className = `status-badge ${statusBadgeClass(payload.overall)}`;
  overall.textContent = operationsStateLabel(payload.overall);
  checked.textContent = `Beobachtet ${auditTimestamp(payload.observed_at)}`;

  const components = document.querySelector('[data-operations-components]');
  components?.replaceChildren();
  operationComponents.forEach((component) => {
    const [title, description] = componentPresentation(component);
    appendOperationsRow(components, component.state, title, description, operationsStateLabel(component.state));
  });

  const installation = payload.installation || {};
  document.querySelector('[data-meta-environment]').textContent = installation.environment || '—';
  document.querySelector('[data-meta-version]').textContent = installation.build_version || '—';
  document.querySelector('[data-meta-api]').textContent = installation.api_version || '—';
  document.querySelector('[data-meta-uptime]').textContent = formatDuration(installation.api_uptime_seconds);
  document.querySelector('[data-meta-transport]').textContent = ({ disabled: 'Loopback / ohne TLS', tls: 'TLS', mtls: 'mTLS' })[installation.transport_security] || installation.transport_security || '—';
  document.querySelector('[data-meta-events]').textContent = installation.domain_event_transport === 'kafka' ? 'Kafka' : 'Nur dauerhafte PostgreSQL-Queue';

  const queues = document.querySelector('[data-operations-queues]');
  queues?.replaceChildren();
  operationQueues.forEach((queue) => {
    const title = queue.key === 'domain-events' ? 'Domain-Ereignisse' : queue.key === 'security-audit-export' ? 'Security-Audit-Export' : queue.key;
    const counts = `Ausstehend ${queue.pending} · Verarbeitung ${queue.processing} · Wiederholung ${queue.retry} · Dead ${queue.dead}`;
    appendOperationsRow(queues, queue.state, title, counts, operationsStateLabel(queue.state));
  });
  const deliveryState = operationQueues.some((queue) => queue.state === 'attention') ? 'attention'
    : operationQueues.some((queue) => queue.state === 'degraded') ? 'degraded'
      : operationQueues.every((queue) => queue.state === 'disabled') ? 'disabled' : 'ready';
  const delivery = document.querySelector('[data-operations-delivery]');
  delivery.className = `status-badge ${statusBadgeClass(deliveryState)}`;
  delivery.textContent = operationsStateLabel(deliveryState);

  document.querySelector('[data-migration-count]').textContent = String(payload.migrations?.applied_count ?? '—');
  document.querySelector('[data-migration-name]').textContent = payload.migrations?.latest_name || '—';
  document.querySelector('[data-migration-applied]').textContent = payload.migrations?.latest_applied_at ? auditTimestamp(payload.migrations.latest_applied_at) : '—';

  const execution = payload.execution || {};
  document.querySelector('[data-operation-update]').disabled = !execution.update_available;
  document.querySelector('[data-operation-restart]').disabled = !execution.restart_available;
  document.querySelector('[data-operations-executor]').textContent = execution.executor_connected ? 'Verbunden' : 'Getrennt';
  document.querySelector('[data-operations-execution-reason]').textContent = execution.executor_connected
    ? 'Ein getrennter Ops-Executor ist verbunden.'
    : 'Kein unabhängiger Ops-Executor konfiguriert; die Admin-API bleibt rein beobachtend.';
}

userSearch?.addEventListener('input', () => {
  window.clearTimeout(userSearchTimer);
  userSearchTimer = window.setTimeout(() => {
    userPage = 1;
    renderUsers();
  }, 120);
});
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
    await loadUsers(selectedTenantID);
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
  try { await loadOperationsStatus(); } catch (error) { showPageNotice(error.message, 'error'); } finally {
    button.disabled = false;
    button.classList.remove('is-refreshing');
  }
});
document.querySelector('[data-refresh-providers]')?.addEventListener('click', async (event) => {
  const button = event.currentTarget;
  button.disabled = true;
  button.classList.add('is-refreshing');
  try { await loadIdentityProviders(); } catch (error) { showPageNotice(error.message, 'error'); } finally {
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
  try { await loadRoles(selectedTenantID); } catch (error) { showPageNotice(error.message, 'error'); } finally {
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
document.querySelector('[data-manage-details-sessions]')?.addEventListener('click', async (event) => {
  const button = event.currentTarget;
  const user = currentUsers.find((item) => item.account_id === button.dataset.accountId);
  if (!user || button.disabled) return;
  if (!window.confirm(`Alle aktiven Sitzungen von ${user.display_name} beenden? Die Person muss sich anschließend erneut anmelden.`)) return;
  button.disabled = true;
  try {
    const result = await adminRequest(`/admin/v1/tenants/${encodeURIComponent(user.tenant_id)}/work-users/${encodeURIComponent(user.account_id)}/sessions/revoke`, {
      method: 'POST',
      headers: { 'If-Match': `"${user.version}"` },
    });
    user.version = result.version;
    await loadWorkUserIdentity(user, button.closest('dialog'));
    showPageNotice('Alle Sitzungen des Arbeitskontos wurden beendet.', 'success');
  } catch (error) {
    button.disabled = false;
    showPageNotice(error.message, 'error');
  }
});
document.querySelector('[data-manage-details-status]')?.addEventListener('click', (event) => {
  const user = currentUsers.find((item) => item.account_id === event.currentTarget.dataset.accountId);
  if (!user) return;
  event.currentTarget.closest('dialog')?.close();
  openUserStatus(user);
});
document.querySelector('[data-manage-details-invitation]')?.addEventListener('click', (event) => {
  const user = currentUsers.find((item) => item.account_id === event.currentTarget.dataset.accountId);
  if (!user || !user.invitation_pending) return;
  event.currentTarget.closest('dialog')?.close();
  openInvitationReissue(user);
});
tenantContextControls.forEach((select) => {
  select?.addEventListener('change', async () => {
    try {
      await selectOrganizationTenant(select.value);
      if (select === roleFormTenantSelect) renderPermissionChoices();
    } catch (error) {
      showPageNotice(error.message, 'error');
    }
  });
});

tenantForm?.addEventListener('submit', async (event) => {
  event.preventDefault();
  if (!tenantForm.reportValidity()) return;
  const button = tenantForm.querySelector('button[type="submit"]');
  const createCompanyButton = document.querySelector('[data-open-tenant-dialog]');
  if (createCompanyButton) createCompanyButton.hidden = true;
  button.disabled = true;
  try {
    const payload = await adminRequest('/admin/v1/tenants', { method: 'POST', body: JSON.stringify(Object.fromEntries(new FormData(tenantForm).entries())) });
    showPageNotice(`Unternehmen ${payload.name} wurde angelegt.`, 'success');
    tenantForm.reset();
    tenantForm.elements.default_locale.value = 'de-DE';
    tenantForm.elements.default_timezone.value = 'Europe/Berlin';
    tenantForm.closest('dialog')?.close();
    await loadTenants();
    if (selectedTenantID !== payload.id) await selectOrganizationTenant(payload.id);
  } catch (error) {
    showPageNotice(error.message || 'Das Unternehmen konnte nicht angelegt werden.', 'error');
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
    showPageNotice(`Unternehmen ${payload.name} wurde gespeichert.`, 'success');
    tenantEditForm.closest('dialog')?.close();
    await loadTenants();
  } catch (error) {
    showPageNotice(error.message || 'Das Unternehmen konnte nicht gespeichert werden.', 'error');
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
    unitForm.elements.unit_type.value = '';
    unitForm.elements.parent_id.value = '';
    unitForm.closest('dialog')?.close();
    await loadUnits(tenantID);
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
    await loadUnits(tenantID);
  } catch (error) {
    showPageNotice(error.message || 'Die Organisationseinheit konnte nicht gespeichert werden.', 'error');
  } finally { button.disabled = false; }
});

workUserForm?.addEventListener('submit', async (event) => {
  event.preventDefault();
  if (!workUserForm.reportValidity()) return;
  const button = workUserForm.querySelector('button[type="submit"]');
  const form = new FormData(workUserForm);
  const tenantID = String(form.get('tenant_id') || '');
  const onboardingMethod = String(form.get('onboarding_method') || '');
  const body = {
    tenant_id: tenantID,
    organizational_unit_id: String(form.get('organizational_unit_id') || ''),
    given_name: String(form.get('given_name') || ''),
    family_name: String(form.get('family_name') || ''),
    login_name: String(form.get('login_name') || ''),
    membership_type: String(form.get('membership_type') || ''),
    onboarding_method: onboardingMethod,
  };
  if (onboardingMethod === 'initial-password') {
    const requirePasswordChange = String(form.get('require_password_change') || '');
    if (!['true', 'false'].includes(requirePasswordChange)) {
      showPageNotice('Wählen Sie ausdrücklich, ob beim ersten Login ein Passwortwechsel erforderlich ist.', 'error');
      return;
    }
    body.initial_password = String(form.get('initial_password') || '');
    body.require_password_change = requirePasswordChange === 'true';
  } else if (onboardingMethod === 'invitation-link') {
    const expiresInHours = Number(form.get('invitation_expires_in_hours'));
    if (!Number.isInteger(expiresInHours) || expiresInHours < 1 || expiresInHours > 168) {
      showPageNotice('Die Einladungsgültigkeit muss eine ganze Stundenzahl zwischen 1 und 168 sein.', 'error');
      return;
    }
    body.invitation_email = String(form.get('invitation_email') || '').trim();
    body.invitation_expires_in_hours = expiresInHours;
  } else {
    showPageNotice('Wählen Sie eine Bereitstellungsart.', 'error');
    return;
  }
  button.disabled = true;
  try {
    const payload = await adminRequest('/admin/v1/work-users', { method: 'POST', body: JSON.stringify(body) });
    workUserForm.closest('dialog')?.close();
    resetWorkUserForm();
    if (onboardingMethod === 'invitation-link') {
      const shown = showInvitationResult(payload.invitation, payload.login_name);
      showPageNotice(shown
        ? `Benutzer ${payload.login_name} wurde angelegt. WERK hat keine E-Mail versendet.`
        : `Benutzer ${payload.login_name} wurde angelegt, aber der einmalige Einladungslink fehlt in der Antwort.`, shown ? 'success' : 'error');
    } else {
      showPageNotice(payload.must_change_password
        ? `Benutzer ${payload.login_name} wurde angelegt. Beim ersten Login ist der bestätigte Passwortwechsel erforderlich.`
        : `Benutzer ${payload.login_name} wurde mit dem festgelegten Startpasswort angelegt.`, 'success');
    }
    workFormTenantSelect.value = tenantID;
    await Promise.all([loadUnits(tenantID), loadUsers(tenantID)]);
  } catch (error) {
    showPageNotice(error.message || 'Das Arbeitskonto konnte nicht angelegt werden.', 'error');
  } finally { button.disabled = false; }
});

userStatusForm?.addEventListener('submit', async (event) => {
  event.preventDefault();
  const button = userStatusForm.querySelector('button[type="submit"]');
  const form = new FormData(userStatusForm);
  const tenantID = form.get('tenant_id');
  const accountID = form.get('account_id');
  const status = form.get('status');
  button.disabled = true;
  try {
    await adminRequest(`/admin/v1/tenants/${encodeURIComponent(tenantID)}/work-users/${encodeURIComponent(accountID)}`, {
      method: 'PUT',
      headers: { 'If-Match': `"${form.get('version')}"` },
      body: JSON.stringify({ status }),
    });
    showPageNotice(status === 'disabled' ? 'Der Arbeitszugang wurde deaktiviert.' : 'Der Arbeitszugang wurde aktiviert.', 'success');
    userStatusForm.closest('dialog')?.close();
    await loadUsers(tenantID);
  } catch (error) {
    showPageNotice(error.message || 'Der Arbeitszugang konnte nicht geändert werden.', 'error');
  } finally {
    button.disabled = false;
  }
});

invitationReissueForm?.addEventListener('submit', async (event) => {
  event.preventDefault();
  if (!invitationReissueForm.reportValidity()) return;
  const button = invitationReissueForm.querySelector('button[type="submit"]');
  const form = new FormData(invitationReissueForm);
  const tenantID = String(form.get('tenant_id') || '');
  const accountID = String(form.get('account_id') || '');
  const loginName = String(form.get('login_name') || '');
  const recipientEmail = String(form.get('recipient_email') || '').trim();
  const expiresInHours = Number(form.get('expires_in_hours'));
  if (!Number.isInteger(expiresInHours) || expiresInHours < 1 || expiresInHours > 168) {
    showPageNotice('Die Einladungsgültigkeit muss eine ganze Stundenzahl zwischen 1 und 168 sein.', 'error');
    return;
  }
  button.disabled = true;
  try {
    const invitation = await adminRequest(`/admin/v1/tenants/${encodeURIComponent(tenantID)}/work-users/${encodeURIComponent(accountID)}/invitation`, {
      method: 'POST',
      body: JSON.stringify({ recipient_email: recipientEmail, expires_in_hours: expiresInHours }),
    });
    invitationReissueForm.closest('dialog')?.close();
    invitationReissueForm.reset();
    const shown = showInvitationResult(invitation, loginName);
    showPageNotice(shown
      ? `Die Einladung für ${loginName} wurde neu ausgegeben. WERK hat keine E-Mail versendet.`
      : `Die Einladung für ${loginName} wurde ersetzt, aber der einmalige Link fehlt in der Antwort.`, shown ? 'success' : 'error');
    await loadUsers(tenantID);
  } catch (error) {
    showPageNotice(error.message || 'Die Einladung konnte nicht neu ausgegeben werden.', 'error');
  } finally {
    button.disabled = false;
  }
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
  if (!tenantID || tenantID !== selectedTenantID || loadedRoleTenantID !== tenantID) {
    roleAssignmentForm.closest('dialog')?.close();
    showPageNotice('Der Unternehmenskontext hat sich geändert. Öffnen Sie die Rollenzuweisung erneut.', 'error');
    return;
  }
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
