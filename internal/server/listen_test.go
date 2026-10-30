package server

import (
	"errors"
	"net"
	"testing"
)

func TestIsAddrInUse(t *testing.T) {
	ln, err := net.Listen("tcp", "0.0.0.0:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	_, err = net.Listen("tcp", ln.Addr().String())
	if err == nil {
		t.Fatal("second listen on the same port succeeded")
	}
	if !isAddrInUse(err) {
		t.Fatalf("not recognized as address in use: %v", err)
	}
	if isAddrInUse(errors.New("other")) {
		t.Fatal("unrelated error recognized as address in use")
	}
}

func TestListenEngineIPv4Taken(t *testing.T) {
	other, err := net.Listen("tcp4", "0.0.0.0:0")
	if err != nil {
		t.Fatal(err)
	}
	defer other.Close()
	port := other.Addr().(*net.TCPAddr).Port
	if _, err := listenEngine(port); !errors.Is(err, ErrPortInUse) {
		t.Fatalf("want ErrPortInUse, got %v", err)
	}
}

func TestListenEngineFree(t *testing.T) {
	probe, err := net.Listen("tcp4", "0.0.0.0:0")
	if err != nil {
		t.Fatal(err)
	}
	port := probe.Addr().(*net.TCPAddr).Port
	probe.Close()
	lns, err := listenEngine(port)
	if err != nil {
		t.Fatal(err)
	}
	for _, ln := range lns {
		ln.Close()
	}
}
