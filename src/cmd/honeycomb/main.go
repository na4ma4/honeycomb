package main

import (
	"context"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"io/ioutil"
	"log"
	"net"
	"net/http"
	"os"
	"path"

	"github.com/docker/docker/client"
	"github.com/icecave/honeycomb/src/backend"
	"github.com/icecave/honeycomb/src/cmd"
	"github.com/icecave/honeycomb/src/docker"
	"github.com/icecave/honeycomb/src/docker/health"
	"github.com/icecave/honeycomb/src/frontend"
	"github.com/icecave/honeycomb/src/frontend/cert"
	"github.com/icecave/honeycomb/src/frontend/cert/generator"
	"github.com/icecave/honeycomb/src/name"
	"github.com/icecave/honeycomb/src/proxy"
	"github.com/icecave/honeycomb/src/proxyprotocol"
	"github.com/icecave/honeycomb/src/static"
	"go.uber.org/multierr"
	"golang.org/x/crypto/acme/autocert"
)

var version = "notset"

func main() {
	config := cmd.GetConfigFromEnvironment()
	logger := log.New(os.Stdout, "", log.LstdFlags)

	staticLocator, err := static.FromEnv(logger)
	if err != nil {
		logger.Fatalln(err)
	}

	dockerClient, err := client.NewClientWithOpts(dockerClientFromEnvironment)
	if err != nil {
		logger.Fatalln(err)
	}

	cachingLocator := &backend.Cache{}

	dockerLocator := &docker.Locator{
		Loader: &docker.ServiceLoader{
			Client: dockerClient,
			Inspector: &docker.ServiceInspector{
				Client: dockerClient,
			},
			Logger: logger,
		},
		Cache:  cachingLocator,
		Logger: logger,
	}
	go dockerLocator.Run()
	defer dockerLocator.Stop()

	cachingLocator.Next = backend.AggregateLocator{
		staticLocator,
		dockerLocator,
	}

	defaultCertificate, err := loadDefaultCertificate(config)
	if err != nil {
		logger.Fatalln(err)
	}

	rootCACertPool := rootCAPool(config, logger)

	provider, err := certificateProvider(
		config,
		defaultCertificate,
		logger,
	)
	if err != nil {
		logger.Fatalln(err)
	}

	tlsConfig := &tls.Config{
		GetCertificate: provider.GetCertificate,
		Certificates:   []tls.Certificate{*defaultCertificate},
		RootCAs:        rootCACertPool,
	}

	secureTransport := &http.Transport{
		Proxy:                 http.DefaultTransport.(*http.Transport).Proxy,
		DialContext:           http.DefaultTransport.(*http.Transport).DialContext,
		MaxIdleConns:          http.DefaultTransport.(*http.Transport).MaxIdleConns,
		IdleConnTimeout:       http.DefaultTransport.(*http.Transport).IdleConnTimeout,
		TLSHandshakeTimeout:   http.DefaultTransport.(*http.Transport).TLSHandshakeTimeout,
		ExpectContinueTimeout: http.DefaultTransport.(*http.Transport).ExpectContinueTimeout,
		TLSClientConfig: &tls.Config{
			RootCAs: rootCACertPool,
		},
	}

	insecureTransport := &http.Transport{
		Proxy:                 http.DefaultTransport.(*http.Transport).Proxy,
		DialContext:           http.DefaultTransport.(*http.Transport).DialContext,
		MaxIdleConns:          http.DefaultTransport.(*http.Transport).MaxIdleConns,
		IdleConnTimeout:       http.DefaultTransport.(*http.Transport).IdleConnTimeout,
		TLSHandshakeTimeout:   http.DefaultTransport.(*http.Transport).TLSHandshakeTimeout,
		ExpectContinueTimeout: http.DefaultTransport.(*http.Transport).ExpectContinueTimeout,
		TLSClientConfig: &tls.Config{
			RootCAs:            rootCACertPool,
			InsecureSkipVerify: true,
		},
	}

	prepareTLSConfig(tlsConfig)

	server := http.Server{
		Addr:      ":" + config.Port,
		TLSConfig: tlsConfig,
		Handler: &frontend.Handler{
			Proxy: &proxy.Handler{
				Locator: cachingLocator,
				SecureHTTPProxy: &proxy.HTTPProxy{
					Transport: secureTransport,
				},
				InsecureHTTPProxy: &proxy.HTTPProxy{
					Transport: insecureTransport,
				},
				SecureWebSocketProxy: &proxy.WebSocketProxy{
					Dialer: &proxy.BasicWebSocketDialer{
						TLSConfig: secureTransport.TLSClientConfig,
					},
				},
				InsecureWebSocketProxy: &proxy.WebSocketProxy{
					Dialer: &proxy.BasicWebSocketDialer{
						TLSConfig: secureTransport.TLSClientConfig,
					},
				},
				Logger: logger,
			},
			HealthCheck: &health.HTTPHandler{
				Checker: &health.SwarmChecker{
					Client: dockerClient,
				},
				Logger: logger,
			},
			Logger: logger,
		},
		ErrorLog: logger,
	}

	go redirectServer(config, logger)

	listener, err := net.Listen("tcp", ":"+config.Port)
	if err != nil {
		logger.Fatal(err)
	}

	if config.ProxyProtocol {
		listener = proxyprotocol.NewListener(listener)
	}

	logger.Printf("Listening on port %s", config.Port)

	err = server.ServeTLS(listener, "", "")
	if err != nil {
		logger.Fatalln(err)
	}
}

func dockerClientFromEnvironment(c *client.Client) error {
	return multierr.Append(
		client.FromEnv(c),
		client.WithHTTPHeaders(
			map[string]string{
				"User-Agent": fmt.Sprintf("Honeycomb/%s", version),
			},
		)(c),
	)
}

func loadDefaultCertificate(config *cmd.Config) (*tls.Certificate, error) {
	cert, err := tls.LoadX509KeyPair(
		path.Join(config.Certificates.BasePath, config.Certificates.ServerCertificate),
		path.Join(config.Certificates.BasePath, config.Certificates.ServerKey),
	)
	if err != nil {
		return nil, err
	}
	issuer, err := tls.LoadX509KeyPair(
		path.Join(config.Certificates.BasePath, config.Certificates.IssuerCertificate),
		path.Join(config.Certificates.BasePath, config.Certificates.IssuerKey),
	)
	if err != nil {
		return nil, err
	}
	cert.Certificate = append(cert.Certificate, issuer.Certificate...)
	return &cert, err
}

func certificateProvider(
	config *cmd.Config,
	defaultCertificate *tls.Certificate,
	logger *log.Logger,
) (cert.Provider, error) {
	providers := cert.AggregateProvider{
		fileCertificateProvider(config, logger),
	}

	acme, ok, err := acmeCertificateProvider(config)
	if err != nil {
		return nil, err
	}
	if ok {
		providers = append(providers, acme)
	}

	adhoc, err := adhocCertificateProvider(
		config,
		defaultCertificate.PrivateKey.(*rsa.PrivateKey),
		logger,
	)
	if err != nil {
		return nil, err
	}

	providers = append(providers, adhoc)

	return providers, nil
}

func fileCertificateProvider(
	config *cmd.Config,
	logger *log.Logger,
) cert.Provider {
	return &cert.FileProvider{
		BasePath: config.Certificates.BasePath,
		Logger:   logger,
	}
}

func acmeCertificateProvider(
	config *cmd.Config,
) (cert.Provider, bool, error) {
	if config.Certificates.ACME.Email == "" {
		return nil, false, nil
	}

	var matchers []*name.Matcher
	for _, d := range config.Certificates.ACME.Domains {
		m, err := name.NewMatcher(d)
		if err != nil {
			return nil, false, err
		}

		matchers = append(matchers, m)
	}

	m := &autocert.Manager{
		Prompt: autocert.AcceptTOS,
		HostPolicy: func(_ context.Context, host string) error {
			sn, err := name.TryParse(host)
			if err != nil {
				return err
			}

			for _, m := range matchers {
				if m.Match(sn) > 0 {
					return nil
				}
			}

			return errors.New("host not allowed")
		},
	}

	if config.Certificates.ACME.CachePath != "" {
		m.Cache = autocert.DirCache(config.Certificates.ACME.CachePath)
	}

	return m, true, nil
}

func adhocCertificateProvider(
	config *cmd.Config,
	serverKey *rsa.PrivateKey,
	logger *log.Logger,
) (cert.Provider, error) {
	issuer, err := tls.LoadX509KeyPair(
		path.Join(config.Certificates.BasePath, config.Certificates.IssuerCertificate),
		path.Join(config.Certificates.BasePath, config.Certificates.IssuerKey),
	)
	if err != nil {
		return nil, err
	}

	x509Cert, err := x509.ParseCertificate(issuer.Certificate[0])
	if err != nil {
		return nil, err
	}

	issuer.Leaf = x509Cert

	return &cert.AdhocProvider{
		Generator: &generator.IssuerSignedGenerator{
			IssuerCertificate: issuer.Leaf,
			IssuerKey:         issuer.PrivateKey.(*rsa.PrivateKey),
			ServerKey:         serverKey,
		},
		Logger: logger,
	}, nil
}

func prepareTLSConfig(config *tls.Config) {
	config.NextProtos = []string{"h2"}
	config.MinVersion = tls.VersionTLS10
	config.PreferServerCipherSuites = true
	config.CurvePreferences = []tls.CurveID{tls.CurveP256, tls.CurveP384, tls.CurveP521}
}

func rootCAPool(
	config *cmd.Config,
	logger *log.Logger,
) *x509.CertPool {
	pool := x509.NewCertPool()
	count := len(pool.Subjects())

	for _, filename := range config.Certificates.CABundles {
		buf, err := ioutil.ReadFile(filename)
		if err == nil {
			pool.AppendCertsFromPEM(buf)
			c := len(pool.Subjects())
			logger.Printf("Loaded %d certificate(s) from CA bundle at %s", c-count, filename)
			count = c
		} else if !os.IsNotExist(err) {
			logger.Fatalln(err)
		}
	}

	return pool
}

func redirectServer(config *cmd.Config, logger *log.Logger) {
	listener, err := net.Listen("tcp", ":"+config.InsecurePort)
	if err != nil {
		logger.Fatal(err)
	}

	if config.ProxyProtocol {
		listener = proxyprotocol.NewListener(listener)
	}

	http.Serve(
		listener,
		http.HandlerFunc(redirectHandler),
	)
}

func redirectHandler(w http.ResponseWriter, req *http.Request) {
	target := "https://" + req.Host + req.URL.Path
	if len(req.URL.RawQuery) > 0 {
		target += "?" + req.URL.RawQuery
	}
	http.Redirect(w, req, target, http.StatusTemporaryRedirect)
}
