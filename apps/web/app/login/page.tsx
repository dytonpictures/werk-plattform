"use client";

import { FormEvent, useState } from "react";
import { useRouter } from "next/navigation";

export default function LoginPage() {
  const router = useRouter();
  const [email, setEmail] = useState("");
  const [password, setPassword] = useState("");
  const [error, setError] = useState("");
  const [busy, setBusy] = useState(false);

  async function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault(); setBusy(true); setError("");
    try {
      const response = await fetch("http://127.0.0.1:8080/api/v1/auth/login", { method: "POST", credentials: "include", headers: { "Content-Type": "application/json" }, body: JSON.stringify({ email, password }) });
      if (!response.ok) throw new Error("Anmeldung fehlgeschlagen");
      router.push("/users"); router.refresh();
    } catch { setError("E-Mail oder Passwort ist nicht korrekt."); } finally { setBusy(false); }
  }

  return <main className="authPage"><form className="authCard" onSubmit={submit}>
    <p className="eyebrow">WERK · Identität</p><h1>Anmelden</h1><p className="muted">Melde dich an, um die Plattform zu verwalten.</p>
    <label>E-Mail<input type="email" autoComplete="username" required value={email} onChange={(e) => setEmail(e.target.value)} /></label>
    <label>Passwort<input type="password" autoComplete="current-password" required value={password} onChange={(e) => setPassword(e.target.value)} /></label>
    {error && <p className="formError" role="alert">{error}</p>}
    <button type="submit" disabled={busy}>{busy ? "Anmeldung …" : "Anmelden"}</button>
  </form></main>;
}
