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
| `actionstream-source` | Roborovski actionstream URL (`stream://host:port` or `stream:///path.sock`) | `stream://localhost:9081` |
| `log-filter` | Comma-separated log categories | `startup,sync,status` |
| `log-file` | Also write logs to this file | none |
| `debug` | Log every category | `false` |

## Licence

BSD-3-Clause; see [LICENSE](LICENSE).
