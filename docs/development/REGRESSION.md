# Dauerhafte Entwickler-Regressionen

WERK hält die dauerhaften Blackbox-Regressionen zentral unter
`cmd/werkctl/regression`. Dadurch müssen Produktionspakete nicht mit
verteilten `*_test.go`-Dateien belastet werden.

Die Prüfung ist read-only und arbeitet ausschließlich gegen die öffentliche
HTTP-Oberfläche:

```bash
go run ./cmd/werkctl regression --url http://127.0.0.1:3000
go run ./cmd/werkctl regression --url http://127.0.0.1:3000 --json
```

Geprüft werden Metadaten, Liveness, Readiness sowie die eingebettete Startseite,
Admin-Oberfläche und deren JavaScript. Mutierende und authentifizierte Flows
gehören in einen separat provisionierten Wegwerf-Stack; der zentrale Runner
verändert kein Zielsystem.

Zusätzlich bleiben `go test ./...` und `go vet ./...` als Build-/Vet-Prüfungen
verfügbar. Neue temporäre Tests dürfen für eine lokale Diagnose angelegt und
müssen vor dem Commit wieder entfernt werden.

Für den echten Browser-/DOM-Vertrag muss der separate Dev-Chrome mit
Remote-Debugging auf Port `9224` laufen:

```bash
./scripts/frontend-regression.sh
```

Der Kafka-Nachweis ist ebenfalls ein eigener, nicht mutierender Vertrag:

```bash
go run ./cmd/werkctl kafka-regression --env .env
```

Bei aktiviertem Kafka werden Broker-Ping und Metadaten für alle drei
konfigurierten WERK-Topics geprüft. Es werden keine Nachrichten produziert und
keine Topics automatisch angelegt.
