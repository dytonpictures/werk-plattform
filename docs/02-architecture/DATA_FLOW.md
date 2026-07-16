# Datenfluss

Browseranfragen erreichen die Same-Origin-Webschicht und anschließend `/api/v1`. Das Backend validiert Eingaben, Sitzung und Berechtigung serverseitig, führt Geschäftsregeln transaktional aus und schreibt sicherheitsrelevante Ereignisse in das Audit-Log.

Passwörter, rohe Sitzungstoken und Secrets dürfen weder Antworten noch Logs oder Audit-Metadaten erreichen.
