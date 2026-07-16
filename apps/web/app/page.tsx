import Link from "next/link";

type SystemInfo = {
  name: string;
  version: string;
  environment: string;
  apiVersion: string;
};

async function getSystemInfo(): Promise<SystemInfo | null> {
  const baseURL = process.env.WERK_INTERNAL_API_URL ?? "http://127.0.0.1:8080";
  try {
    const response = await fetch(`${baseURL}/api/v1/system/info`, { cache: "no-store" });
    if (!response.ok) return null;
    return (await response.json()) as SystemInfo;
  } catch {
    return null;
  }
}

export default async function Home() {
  const system = await getSystemInfo();
  return (
    <main className="shell">
      <aside className="sidebar" aria-label="Hauptnavigation">
        <Link className="brand" href="/" aria-label="WERK Startseite">
          <span className="brandMark">W</span>
          <span>WERK</span>
        </Link>
        <p className="navLabel">Arbeitsbereich</p>
        <nav>
          <Link className="navItem active" href="/">Übersicht <span className="navMeta">01</span></Link>
          <Link className="navItem" href="/users">Benutzer <span className="navMeta">02</span></Link>
        </nav>
        <p className="navLabel navLabelSpaced">System</p>
        <nav>
          <span className="navItem disabled">Organisation <span className="navMeta">bald</span></span>
          <span className="navItem disabled">Einstellungen <span className="navMeta">bald</span></span>
        </nav>
        <p className="phase">Fundament · Phase 1</p>
      </aside>

      <section className="content">
        <header className="topbar">
          <div>
            <p className="eyebrow">Arbeitsbereich / Übersicht</p>
            <h1>Guten Morgen.</h1>
            <p className="pageLead">Der zentrale Überblick über deine WERK-Installation.</p>
          </div>
          <span className={`status ${system ? "online" : "offline"}`}>
            <span aria-hidden="true" /> {system ? "System bereit" : "API nicht erreichbar"}
          </span>
        </header>

        <div className="hero dashboardHero">
          <div><p className="eyebrow">Plattformstatus</p><h2>Alles im grünen Bereich.</h2><p>Die Kernsysteme arbeiten stabil. Von hier aus verwaltest du Identitäten, Zugriffe und die nächsten Plattformbausteine.</p></div>
          <div className="heroSignal"><span className="signalPulse"/><strong>{system ? "Operational" : "Degraded"}</strong><small>Letzte Prüfung gerade eben</small></div>
        </div>

        <div className="grid" aria-label="Systeminformationen">
          <article className="card">
            <span>Plattformversion</span>
            <strong>{system?.version ?? "nicht verfügbar"}</strong>
            <small>Reproduzierbar gepinnt</small>
          </article>
          <article className="card">
            <span>API-Version</span>
            <strong>{system?.apiVersion ?? "offline"}</strong>
            <small>Versionierte Schnittstelle</small>
          </article>
          <article className="card">
            <span>Betriebsumgebung</span>
            <strong>{system?.environment ?? "unbekannt"}</strong>
            <small>Self-hosted first</small>
          </article>
        </div>
        <section className="quickSection"><div className="sectionHeading"><div><p className="eyebrow">Schnellzugriff</p><h2>Arbeitsbereiche</h2></div></div><div className="quickGrid"><Link className="quickCard" href="/users"><span className="quickIcon">ID</span><span><strong>Benutzer & Zugriff</strong><small>Identitäten, Rollen und Sitzungen verwalten</small></span><span className="arrow">↗</span></Link><div className="quickCard disabledCard"><span className="quickIcon">OR</span><span><strong>Organisation</strong><small>Unternehmensprofil und Grundeinstellungen</small></span><span className="soon">Demnächst</span></div><div className="quickCard disabledCard"><span className="quickIcon">AU</span><span><strong>Audit-Protokoll</strong><small>Sicherheitsrelevante Aktivitäten nachvollziehen</small></span><span className="soon">Demnächst</span></div></div></section>
      </section>
    </main>
  );
}
