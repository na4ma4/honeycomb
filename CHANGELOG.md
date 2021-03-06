# Changelog

## 0.3.10 (2020-08-19)

- **[NEW]** Add `TLS_[MIN|MAX]_VERSION` environment variables for setting allowed TLS version (thanks [@koshatul])
- **[NEW]** Add `TLS_CIPHER_SUITE` environment variable for setting allowed TLS cipher suites (thanks [@koshatul])

## 0.3.9 (2020-01-28)

- **[NEW]** Added support for H2C (non-TLS HTTP/2) upstream endpoints (requires `honeycomb.tls=h2c` label)

## 0.3.8 (2019-09-18)

- **[FIXED]** Allow "overlapping" wildcard domains by always using the longest match
- **[FIXED]** Surpress `Can not route to '<service>'` error message for unexposed services

## 0.3.7 (2019-08-15)

- **[IMPROVED]** Use a more descriptive user agent when contacting the Docker API

## 0.3.6 (2019-07-24)

- **[IMPROVED]** Use a separate timeout for the healthcheck execution and the healtcheck itself

## 0.3.5 (2019-03-28)

- **[IMPROVED]** Don't require a `honeycomb.match` label on every service (`honeycomb.match.<whatever>`) is sufficient
- **[NEW]** Add support for "insecure" upstream hosts by setting `honeycomb.tls` to `insecure`

## 0.3.4 (2019-02-20)

- **[FIX]** Generated certificates now include the issuer CA certificate in the chain (thanks [@koshatul])
- **[NEW]** Add support for parsing the client's remote address from [PROXY protocol](https://www.haproxy.org/download/1.8/doc/proxy-protocol.txt) headers (thanks [@koshatul])
- **[NEW]** Add `CA_PATH` environment variable for specifying the location of CA bundles

## 0.3.3 (2017-03-16)

- **[NEW]** Add Comodo and GoDaddy intermediate certificates to the CA bundle (thanks [@koshatul])

## 0.3.2 (2017-03-13)

- **[NEW]** Allow specification of static routes via environment variables

## 0.3.1 (2017-03-10)

- **[FIX]** Send `X-Forwarded-Proto` and `X-Forwarded-SSL` headers

## 0.3.0 (2017-03-07)

- **[NEW]** Add support for multiple match labels

## 0.2.0 (2017-03-07)

- **[BC]** Remove support for loading certificates from S3, in favour of Docker secrets
- **[NEW]** Add file-based certificate provider
- **[FIXED]** Fix PKCS#8 private key loading
- **[IMPROVED]** Add HSTS headers

## 0.1.0 (2017-03-03)

- Initial release

[@koshatul]: https://github.com/koshatul
