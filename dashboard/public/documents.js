const documentsRoot = document.querySelector('[data-documents-root]');
const documentsList = document.querySelector('[data-documents-list]');
const documentsDetail = document.querySelector('[data-documents-detail]');
const documentsEmpty = document.querySelector('[data-documents-empty]');
const documentsCount = document.querySelector('[data-documents-result-count]');
const documentsMore = document.querySelector('[data-documents-more]');
const documentsFilter = document.querySelector('[data-documents-filter-form]');
const documentsReset = document.querySelector('[data-documents-filter-reset]');

const documentsState = {
  items: [],
  selectedId: '',
  nextCursor: '',
  listRequest: 0,
  detailRequest: 0,
};

class DocumentsRequestError extends Error {
  constructor(status) {
    super(`Dokumentanfrage fehlgeschlagen (${status})`);
    this.status = status;
  }
}

function element(tag, className = '', text = '') {
  const node = document.createElement(tag);
  if (className) node.className = className;
  if (text) node.textContent = text;
  return node;
}

function classificationLabel(value) {
  return { internal: 'Intern', confidential: 'Vertraulich', restricted: 'Streng vertraulich' }[value] || 'Nicht klassifiziert';
}

function documentStatusLabel(value) {
  return value === 'archived' ? 'Archiviert' : 'Aktiv';
}

function sourceLabel(value) {
  return { upload: 'Upload', import: 'Import', collaboration: 'Zusammenarbeit', signature: 'Signatur' }[value] || value || 'Unbekannt';
}

function moduleLabel(value) {
  return value === 'core.documents' ? 'WERK Dokumente' : value || 'Unbekannte Quelle';
}

function formatDate(value, withTime = false) {
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return '—';
  return new Intl.DateTimeFormat('de-DE', withTime
    ? { dateStyle: 'medium', timeStyle: 'short' }
    : { dateStyle: 'medium' }).format(date);
}

async function fetchDocumentsJSON(url) {
  const response = await fetch(url, { headers: { Accept: 'application/json' }, credentials: 'same-origin' });
  if (response.status === 401) {
    redirectToSignIn();
    throw new DocumentsRequestError(response.status);
  }
  if (!response.ok) throw new DocumentsRequestError(response.status);
  return response.json();
}

function currentFilters() {
  const data = new FormData(documentsFilter);
  return {
    q: String(data.get('q') || '').trim(),
    status: String(data.get('status') || ''),
    classification: String(data.get('classification') || ''),
  };
}

function documentListURL(cursor = '') {
  const filters = currentFilters();
  const parameters = new URLSearchParams({ limit: '25' });
  if (filters.q) parameters.set('q', filters.q);
  if (filters.status) parameters.set('status', filters.status);
  if (filters.classification) parameters.set('classification', filters.classification);
  if (cursor) parameters.set('cursor', cursor);
  return `/api/v1/documents?${parameters}`;
}

function setDocumentsBusy(value) {
  documentsRoot?.setAttribute('aria-busy', String(value));
}

function redirectToSignIn() {
  documentsState.listRequest += 1;
  documentsState.detailRequest += 1;
  documentsState.items = [];
  documentsState.selectedId = '';
  documentsState.nextCursor = '';
  documentsList?.replaceChildren();
  documentsEmpty?.setAttribute('hidden', '');
  documentsMore?.setAttribute('hidden', '');
  renderDocumentPlaceholder('Sitzung beendet', 'Sie werden zur Anmeldung weitergeleitet.');
  setDocumentsBusy(false);
  window.location.replace('/');
}

function updateDocumentsCount() {
  if (!documentsCount) return;
  const count = documentsState.items.length;
  documentsCount.textContent = count === 1 ? '1 Dokument sichtbar' : `${count} Dokumente sichtbar`;
}

function createClassificationBadge(level) {
  const badge = element('span', `documents-classification is-${level || 'unknown'}`, classificationLabel(level));
  return badge;
}

function renderDocumentsList() {
  documentsList?.replaceChildren();
  updateDocumentsCount();
  if (!documentsState.items.length) {
    const filters = currentFilters();
    const filtered = Boolean(filters.q || filters.classification || filters.status === 'archived');
    documentsEmpty?.removeAttribute('hidden');
    const title = documentsEmpty?.querySelector('h3');
    const description = documentsEmpty?.querySelector('p');
    if (title) title.textContent = filtered ? 'Keine passenden Dokumente' : 'Noch keine Dokumente';
    if (description) description.textContent = filtered
      ? 'Passen Sie Suche oder Filter an, um andere sichtbare Dokumente zu finden.'
      : 'Von Ihnen erzeugte, veröffentlichte Dokumente erscheinen automatisch in dieser Ansicht.';
    renderDocumentPlaceholder();
    return;
  }
  documentsEmpty?.setAttribute('hidden', '');
  documentsState.items.forEach((document) => {
    const item = element('li', 'documents-list-item');
    const button = element('button', 'documents-list-button');
    button.type = 'button';
    button.dataset.documentId = document.id;
    if (documentsState.selectedId === document.id) {
      button.classList.add('is-selected');
      button.setAttribute('aria-current', 'true');
    }
    const icon = element('span', 'documents-list-icon', 'D');
    icon.setAttribute('aria-hidden', 'true');
    const content = element('span', 'documents-list-content');
    content.append(element('strong', '', document.title || 'Unbenanntes Dokument'));
    const meta = element('span', 'documents-list-meta');
    meta.append(createClassificationBadge(document.classification?.level));
    meta.append(element('span', '', `Version ${document.latest_version?.version_number || '—'}`));
    meta.append(element('span', '', formatDate(document.updated_at)));
    content.append(meta);
    const status = element('span', `documents-list-status is-${document.status || 'unknown'}`, documentStatusLabel(document.status));
    button.append(icon, content, status);
    item.append(button);
    documentsList?.append(item);
  });
}

function renderDocumentPlaceholder(title = 'Dokument auswählen', description = 'Klassifikation, Aufbewahrung und veröffentlichte Versionen erscheinen hier.') {
  documentsDetail?.replaceChildren();
  const empty = element('div', 'documents-detail-empty');
  const glyph = element('span', 'documents-document-glyph');
  glyph.setAttribute('aria-hidden', 'true');
  glyph.append(element('i'), element('i'), element('i'));
  const heading = element('h2', '', title);
  heading.id = 'documents-detail-title';
  empty.append(glyph, element('p', 'admin-breadcrumb', 'Dokumentansicht'), heading, element('p', '', description));
  documentsDetail?.append(empty);
}

function appendDetailField(container, label, value, className = '') {
  const row = element('div');
  row.append(element('dt', '', label), element('dd', className, value || '—'));
  container.append(row);
}

function renderDocumentDetail(payload) {
  const document = payload?.document;
  const versions = Array.isArray(payload?.versions) ? payload.versions : [];
  if (!document || document.id !== documentsState.selectedId) return;
  documentsDetail?.replaceChildren();

  const header = element('div', 'documents-detail-header');
  const heading = element('div');
  const title = element('h2', '', document.title || 'Unbenanntes Dokument');
  title.id = 'documents-detail-title';
  heading.append(element('p', 'admin-breadcrumb', 'Dokumentansicht'), title);
  const badges = element('div', 'documents-detail-badges');
  badges.append(createClassificationBadge(document.classification?.level));
  badges.append(element('span', `documents-list-status is-${document.status || 'unknown'}`, documentStatusLabel(document.status)));
  header.append(heading, badges);

  const governance = element('section', 'documents-governance');
  const governanceTitle = element('div', 'documents-section-title');
  governanceTitle.append(element('h3', '', 'Einordnung und Aufbewahrung'), element('span', '', `Revision ${document.classification?.revision || '—'}`));
  const metadata = element('dl', 'documents-detail-grid');
  appendDetailField(metadata, 'Klassifikation', classificationLabel(document.classification?.level));
  appendDetailField(metadata, 'Aufbewahrungsklasse', document.classification?.retention_class || '—', 'mono');
  appendDetailField(metadata, 'Aufbewahrung bis', document.classification?.retention_until ? formatDate(document.classification.retention_until) : 'Nicht festgelegt');
  appendDetailField(metadata, 'Legal Hold', document.classification?.legal_hold ? 'Aktiv – Löschung gesperrt' : 'Nicht aktiv');
  appendDetailField(metadata, 'Erzeuger-Modul', moduleLabel(document.source_module));
  appendDetailField(metadata, 'Zuletzt geändert', formatDate(document.updated_at, true));
  governance.append(governanceTitle, metadata);

  const history = element('section', 'documents-version-section');
  const historyTitle = element('div', 'documents-section-title');
  historyTitle.append(element('h3', '', 'Veröffentlichte Versionen'), element('span', '', `${versions.length} unveränderlich`));
  const list = element('ol', 'documents-version-list');
  versions.forEach((version) => {
    const item = element('li');
    const marker = element('span', 'documents-version-marker', `V${version.version_number}`);
    const content = element('span');
    content.append(element('strong', '', `Version ${version.version_number}`), element('small', '', `${sourceLabel(version.source)} · ${formatDate(version.published_at, true)}`));
    if (version.id === document.latest_version?.id) content.append(element('span', 'documents-current-version', 'Aktueller Stand'));
    item.append(marker, content);
    list.append(item);
  });
  if (!versions.length) list.append(element('li', 'documents-version-empty', 'Keine veröffentlichte Version verfügbar.'));
  history.append(historyTitle, list);

  const boundary = element('div', 'documents-detail-boundary');
  boundary.append(element('strong', '', 'Inhalt getrennt geschützt'), element('span', '', 'Dieser Bereich liest keine Blob-, Provider- oder Transferdaten.'));
  documentsDetail?.append(header, governance, history, boundary);
}

function renderDocumentDetailError(message, retry) {
  documentsDetail?.replaceChildren();
  const panel = element('div', 'documents-detail-empty is-error');
  const heading = element('h2', '', 'Dokument nicht verfügbar');
  heading.id = 'documents-detail-title';
  panel.append(element('span', 'documents-empty-mark', '!'), heading, element('p', '', message));
  if (retry) {
    const button = element('button', 'button button-secondary', 'Erneut versuchen');
    button.type = 'button';
    button.addEventListener('click', retry);
    panel.append(button);
  }
  documentsDetail?.append(panel);
}

async function selectDocument(documentID, moveFocus = false) {
  const requestVersion = ++documentsState.detailRequest;
  documentsState.selectedId = documentID;
  renderDocumentsList();
  documentsDetail?.setAttribute('aria-busy', 'true');
  renderDocumentPlaceholder('Dokument wird geladen', 'Klassifikation und Versionshistorie werden serverseitig geprüft.');
  try {
    const payload = await fetchDocumentsJSON(`/api/v1/documents/${encodeURIComponent(documentID)}`);
    if (requestVersion !== documentsState.detailRequest || documentsState.selectedId !== documentID) return;
    renderDocumentDetail(payload);
    if (moveFocus) documentsDetail?.focus({ preventScroll: true });
  } catch (error) {
    if (requestVersion !== documentsState.detailRequest || error.status === 401) return;
    if (error.status === 404) {
      documentsState.items = documentsState.items.filter((item) => item.id !== documentID);
      documentsState.selectedId = '';
      renderDocumentsList();
      renderDocumentDetailError('Das Dokument existiert nicht mehr oder ist für dieses Konto nicht sichtbar.');
    } else {
      renderDocumentDetailError('Die geprüften Dokumentdetails konnten nicht geladen werden.', () => selectDocument(documentID, false));
    }
  } finally {
    if (requestVersion === documentsState.detailRequest) documentsDetail?.setAttribute('aria-busy', 'false');
  }
}

function renderListFailure(status) {
  documentsState.items = [];
  documentsState.selectedId = '';
  documentsState.nextCursor = '';
  documentsList?.replaceChildren();
  documentsEmpty?.removeAttribute('hidden');
  documentsEmpty?.querySelectorAll('button').forEach((button) => button.remove());
  const title = documentsEmpty?.querySelector('h3');
  const description = documentsEmpty?.querySelector('p');
  if (title) title.textContent = status === 403 ? 'Keine Berechtigung' : 'Dokumente nicht verfügbar';
  if (description) description.textContent = status === 403
    ? 'Für dieses Arbeitskonto wurde die persönliche Dokumentansicht nicht freigeschaltet.'
    : 'Die Dokumentmetadaten konnten nicht geladen werden. Versuchen Sie es erneut.';
  if (status !== 403 && documentsEmpty) {
    const retry = element('button', 'button button-secondary', 'Erneut versuchen');
    retry.type = 'button';
    retry.addEventListener('click', () => loadDocuments());
    documentsEmpty.append(retry);
  }
  if (documentsCount) documentsCount.textContent = status === 403 ? 'Zugriff nicht freigegeben' : 'Laden fehlgeschlagen';
  documentsMore?.setAttribute('hidden', '');
  renderDocumentPlaceholder(status === 403 ? 'Ansicht nicht freigegeben' : 'Dokumente nicht verfügbar', status === 403
    ? 'Die Berechtigung wird ausschließlich serverseitig vergeben.'
    : 'Nach einer erfolgreichen Verbindung erscheint hier wieder die Detailansicht.');
}

async function loadDocuments({ append = false } = {}) {
  if (append && (documentsMore?.disabled || !documentsState.nextCursor)) return;
  const requestVersion = ++documentsState.listRequest;
  const cursor = append ? documentsState.nextCursor : '';
  setDocumentsBusy(true);
  if (documentsMore) documentsMore.disabled = true;
  if (!append) {
    documentsState.detailRequest += 1;
    documentsState.items = [];
    documentsState.nextCursor = '';
    documentsState.selectedId = '';
    documentsList?.replaceChildren(element('li', 'documents-list-loading'));
    documentsEmpty?.setAttribute('hidden', '');
  }
  try {
    const page = await fetchDocumentsJSON(documentListURL(cursor));
    if (requestVersion !== documentsState.listRequest) return;
    if (page.visibility_scope !== 'created-by-me' || !Array.isArray(page.items)) throw new DocumentsRequestError(502);
    if (append) {
      const knownIDs = new Set(documentsState.items.map((item) => item.id));
      documentsState.items = documentsState.items.concat(page.items.filter((item) => !knownIDs.has(item.id)));
    } else {
      documentsState.items = page.items;
    }
    documentsState.nextCursor = page.next_cursor || '';
    renderDocumentsList();
    documentsMore?.toggleAttribute('hidden', !documentsState.nextCursor);
    if (!append && documentsState.items.length) await selectDocument(documentsState.items[0].id, false);
  } catch (error) {
    if (requestVersion !== documentsState.listRequest || error.status === 401) return;
    renderListFailure(error.status || 0);
  } finally {
    if (requestVersion === documentsState.listRequest) {
      setDocumentsBusy(false);
      if (documentsMore) documentsMore.disabled = false;
    }
  }
}

documentsList?.addEventListener('click', (event) => {
  const button = event.target.closest('[data-document-id]');
  if (button) void selectDocument(button.dataset.documentId, false);
});

documentsFilter?.addEventListener('submit', (event) => {
  event.preventDefault();
  void loadDocuments();
});

documentsReset?.addEventListener('click', () => {
  documentsFilter?.reset();
  void loadDocuments();
});

documentsMore?.addEventListener('click', () => void loadDocuments({ append: true }));

if (documentsRoot) void loadDocuments();
