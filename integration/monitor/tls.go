package main

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"math/big"
	"net"
	"net/http"
	"os"
	"time"
)

func certificates(dir string) (tls.Certificate, error) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return tls.Certificate{}, err
	}
	now := time.Now()
	ca := &x509.Certificate{SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: "ephemeral offline acceptance"}, NotBefore: now.Add(-time.Minute), NotAfter: now.Add(2 * time.Hour), IsCA: true, BasicConstraintsValid: true, KeyUsage: x509.KeyUsageCertSign | x509.KeyUsageDigitalSignature}
	caDER, err := x509.CreateCertificate(rand.Reader, ca, ca, &key.PublicKey, key)
	if err != nil {
		return tls.Certificate{}, err
	}
	leafKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return tls.Certificate{}, err
	}
	leaf := &x509.Certificate{SerialNumber: big.NewInt(2), DNSNames: []string{"api.telegram.org"}, NotBefore: ca.NotBefore, NotAfter: ca.NotAfter, KeyUsage: x509.KeyUsageDigitalSignature, ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}}
	der, err := x509.CreateCertificate(rand.Reader, leaf, ca, &leafKey.PublicKey, key)
	if err != nil {
		return tls.Certificate{}, err
	}
	if err = os.MkdirAll(dir, 0755); err != nil {
		return tls.Certificate{}, err
	}
	if err = os.WriteFile(dir+"/ca.pem.tmp", pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: caDER}), 0644); err != nil {
		return tls.Certificate{}, err
	}
	if err = os.Rename(dir+"/ca.pem.tmp", dir+"/ca.pem"); err != nil {
		return tls.Certificate{}, err
	}
	// Private keys stay in fixture memory, never in a mounted volume or logs.
	return tls.Certificate{Certificate: [][]byte{der, caDER}, PrivateKey: leafKey}, nil
}
func checkReady() error {
	ca, err := os.ReadFile("/ca/ca.pem")
	if err != nil {
		return err
	}
	roots := x509.NewCertPool()
	if !roots.AppendCertsFromPEM(ca) {
		return fmt.Errorf("invalid CA")
	}
	conn, err := tls.DialWithDialer(&net.Dialer{Timeout: time.Second}, "tcp", "127.0.0.1:443", &tls.Config{RootCAs: roots, ServerName: "api.telegram.org", MinVersion: tls.VersionTLS12})
	if err != nil {
		return err
	}
	conn.Close()
	plain, err := net.DialTimeout("tcp", "127.0.0.1:8080", time.Second)
	if err == nil {
		plain.Close()
	}
	return err
}
func serve() error {
	cert, err := certificates("/ca")
	if err != nil {
		return err
	}
	f := newFixture()
	// Bind all listeners before the control endpoint can report readiness.
	html, err := net.Listen("tcp", ":8080")
	if err != nil {
		return err
	}
	defer html.Close()
	telegram, err := tls.Listen("tcp", ":443", &tls.Config{Certificates: []tls.Certificate{cert}, MinVersion: tls.VersionTLS12})
	if err != nil {
		return err
	}
	defer telegram.Close()
	control, err := net.Listen("tcp", "127.0.0.1:9090")
	if err != nil {
		return err
	}
	defer control.Close()
	errs := make(chan error, 3)
	for _, srv := range []struct {
		l net.Listener
		h http.HandlerFunc
	}{{html, f.html}, {telegram, f.telegram}, {control, f.control}} {
		go func(l net.Listener, h http.HandlerFunc) {
			s := &http.Server{Handler: h, ReadHeaderTimeout: 3 * time.Second, IdleTimeout: 30 * time.Second}
			errs <- s.Serve(l)
		}(srv.l, srv.h)
	}
	return <-errs
}
