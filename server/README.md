# go-kit/server

[![Go Reference][ref1]][ref2]
 [![GitHub Release][gr1]][gr2]
 [![GoCard][gc1]][gc2]
 [![GitHub license][gl1]][gl2]

[ref1]: https://pkg.go.dev/badge/github.com/LeKovr/go-kit/server.svg
[ref2]: https://pkg.go.dev/github.com/LeKovr/go-kit/server
[gc1]: https://goreportcard.com/badge/github.com/LeKovr/go-kit/server
[gc2]: https://goreportcard.com/report/github.com/LeKovr/go-kit/server
[gr1]: https://img.shields.io/github/v/tag/Lekovr/go-kit?filter=server/*
[gr2]: https://github.com/LeKovr/go-kit/releases?q=server&expanded=true
[gl1]: https://img.shields.io/github/license/LeKovr/go-kit.svg
[gl2]: https://github.com/LeKovr/go-kit/blob/master/LICENSE

Пакет для сборки сервисов.

## Пример

[embedmd ]: # (example/main.go golang)

## Использование

Использование пакета добавляет в сервис следующие опции:

```
Server Options:
      --srv.listen=              Addr and port which server listens at (default: :8080) [$SRV_LISTEN]
      --srv.maxheader=           MaxHeaderBytes
      --srv.rto=                 HTTP read timeout (default: 10s)
      --srv.wto=                 HTTP write timeout (default: 60s)
      --srv.grace=               Stop grace period (default: 10s)
      --srv.ip_header=           HTTP Request Header for remote IP (default: X-Real-IP) [$SRV_IP_HEADER]
      --srv.user_header=         HTTP Request Header for username (default: X-Username) [$SRV_USER_HEADER]

HTTPS Options:
      --srv.tls.cert=            CertFile for serving HTTPS instead HTTP [$SRV_TLS_CERT]
      --srv.tls.key=             KeyFile for serving HTTPS instead HTTP [$SRV_TLS_KEY]
      --srv.tls.no-check         disable tls certificate validation

Version response Options:
      --srv.vr.prefix=           URL for version response (default: /js/version.js)
      --srv.vr.format=           Format string for version response (default: "document.addEventListener('DOMContentLoaded', () => { appVersion.innerText = '%s'; });\n")
      --srv.vr.ctype=            js code Content-Type header (default: text/javascript)

Help Options:
  -h, --help                     Show this help message


```