# creationindex

You run creationindex to answer how accounts come into existence on an Antelope chain: who created each one, and what RAM arrived with it. It follows a Roborovski action stream from the first global sequence, records every `eosio::newaccount` together with the RAM contributions in its transaction, and serves the results over HTTP under `/v1/creationindex/`. A configured gift program account adds program views: gifts per program creator, quota use, and returns. The terms are defined in [GLOSSARY.md](GLOSSARY.md).

## Building

Go 1.25 or later.

```
make build     # binary at ./creationindex
make install   # go install into GOPATH/bin
make test
make vet
```

## Configuration

Configuration is INI and command-line flags; flags override the file. Copy `config.ini.example` to `config.ini` and set:

| Key | Meaning | Default |
|-----|---------|---------|
| `db-path` | Pebble database directory | `./creationindex.db` |
| `http-listen` | HTTP API TCP address, or `none` | `:9480` |
| `http-socket` | HTTP API Unix socket path ending in `.sock`, or `none` | `none` |
| `actionstream-source` | Roborovski actionstream URL (`stream://host:port` or `stream:///path.sock`) | `stream://localhost:9081` |
| `program-account` | Gift program account (`gift.vaulta` on Vaulta, `gift.gm` on Jungle 4): its `giftram`s count as program gifts and the gift stream indexes its actions; when unset, no gift counts as a program gift and the gift stream stays off | none |
| `log-filter` | Comma-separated log categories | `startup,sync,status,http` |
| `log-file` | Also write logs to this file | none |
| `debug` | Log every category | `false` |

When a new build changes the storage schema, creationindex refuses to start and logs the database path. Delete that directory and start creationindex again; it replays from seq 1.

## API

You query creationindex with GET query parameters, or with a POST JSON body that uses the same names:

```
curl 'localhost:9480/v1/creationindex/get_creation?account=willsunshine'
```

The network routes are `get_creation`, `get_creations` (by `creator` or `program_creator`, paged with `after` and `limit`), `get_daily` (UTC days `from` and `to`, at most 366), `get_day_creators` (one `day`, ranked, paged with `offset` and `limit`) and `health`. Every field is always present: names are `""` when empty, figures the chain does not state are `null`, and lists are `[]`. Errors return `{"error": "..."}` with status 400 for a bad parameter and 404 for an unknown account.

## Test fixtures

Fixtures in `internal/attribution/testdata/` are real stream actions, captured with `cmd/capture` from a host that serves the actionstream. Start the capture at the transaction's first global sequence and name each transaction to keep:

```
go run ./cmd/capture -source stream:///ramdisk/vaulta/actionstream.sock -start 368360304285 -out internal/attribution/testdata willsunshine=3efce2e5432e2e1ce595615e19fa245118e6766414022bc935a7a8eebfa79258
```

It subscribes with the service's `eosio` filter and writes each named transaction's actions to `<name>.jsonl`, one stream `Action` per line. For the gift stream, add `-stream gift -program-account gift.gm`.

## Licence

BSD-3-Clause; see [LICENSE](LICENSE).
