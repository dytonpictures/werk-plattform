# Modulgrenzen

Startmodule sind System, Identity, Authorization und Audit. Ein Modul greift nicht direkt auf private Tabellen oder interne Pakete eines anderen Moduls zu. Erlaubt sind veröffentlichte Services, Repositories und bewusst definierte interne Events.

Gemeinsame technische Hilfen liegen in kleinen Plattformpaketen und dürfen keine fachlichen Rückkopplungen erzeugen.
